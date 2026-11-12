package localstore

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/migueel15/dory/internal/service"
)

const timeLayout = "2006-01-02T15:04:05.000000Z"

var uuidRe = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// arrayFields marshal as null when unset; they are stored as [].
var arrayFields = []string{"tags", "refs", "stack", "agents_available", "code_refs", "permissions", "open_questions"}

// columns managed by the store; they are not part of data.
var columnFields = []string{"id", "created_at", "updated_at", "archived_at"}

func newUUID() string {
	var u [16]byte
	rand.Read(u[:])
	u[6] = u[6]&0x0f | 0x40
	u[8] = u[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", u[0:4], u[4:6], u[6:8], u[8:10], u[10:])
}

type resource[T any] struct {
	b    *backend
	spec *kindSpec
}

type row struct {
	data, fieldClock, owner, created, updated string
	archived                                  sql.NullString
}

func (r resource[T]) load(ctx context.Context, q execer, id string) (*row, error) {
	var x row
	err := q.QueryRowContext(ctx, `SELECT data, field_clock, owner_device, created_at, updated_at, archived_at FROM records WHERE kind = ? AND id = ?`, r.spec.name, id).
		Scan(&x.data, &x.fieldClock, &x.owner, &x.created, &x.updated, &x.archived)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &x, err
}

// compose rebuilds the full document of a row: data plus its columns.
func compose(id string, x *row) ([]byte, error) {
	doc := map[string]any{}
	if err := json.Unmarshal([]byte(x.data), &doc); err != nil {
		return nil, err
	}
	doc["id"], doc["created_at"], doc["updated_at"] = id, x.created, x.updated
	if x.archived.Valid {
		doc["archived_at"] = x.archived.String
	} else {
		doc["archived_at"] = nil
	}
	return json.Marshal(doc)
}

func decode[T any](raw []byte, err error) (v T, e error) {
	if err != nil {
		return v, err
	}
	return v, json.Unmarshal(raw, &v)
}

func parseID(id string) (string, error) {
	if !uuidRe.MatchString(id) {
		return "", invalid("invalid uuid %q", id)
	}
	return strings.ToLower(id), nil
}

func (r resource[T]) Get(ctx context.Context, id string) (T, error) {
	var zero T
	id, err := parseID(id)
	if err != nil {
		return zero, err
	}
	x, err := r.load(ctx, r.b.db, id)
	if err != nil {
		return zero, err
	}
	if x == nil || x.archived.Valid {
		return zero, service.ErrNotFound
	}
	return decode[T](compose(id, x))
}

func (r resource[T]) now() string { return r.b.opts.Clock().UTC().Format(timeLayout) }

// scalar converts a filter value to something SQLite can bind.
func scalar(v any) (any, error) {
	switch t := v.(type) {
	case bool:
		if t {
			return 1, nil
		}
		return 0, nil
	case string, float64:
		return t, nil
	}
	return nil, invalid("unsupported filter value %v", v)
}

func (r resource[T]) List(ctx context.Context, f service.Filter) ([]T, error) {
	if f.Offset < 0 || f.Limit < 0 || f.Limit > 1000 {
		return nil, service.ErrInvalid
	}
	if f.Limit == 0 {
		f.Limit = 100
	}
	// Normalise through JSON so every number, bool and array has one Go type.
	var fields map[string]any
	if raw, err := json.Marshal(f.Fields); err != nil {
		return nil, err
	} else if err = json.Unmarshal(raw, &fields); err != nil {
		return nil, err
	}
	where, args := []string{"kind = ?"}, []any{r.spec.name}
	if !f.IncludeArchived {
		where = append(where, "archived_at IS NULL")
	}
	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		v := fields[k]
		if strings.ContainsAny(k, `"'`) {
			return nil, invalid("invalid filter key %q", k)
		}
		expr, path := "json_extract(data, ?)", []any{`$."` + k + `"`}
		if slices.Contains(columnFields, k) {
			expr, path = k, nil
		}
		switch t := v.(type) {
		case nil:
			where = append(where, expr+" IS NULL")
			args = append(args, path...)
		case []any:
			if path == nil {
				return nil, invalid("unsupported filter on %s", k)
			}
			where = append(where, "json_type(data, ?) = 'array'")
			args = append(args, path[0])
			for _, e := range t {
				s, err := scalar(e)
				if err != nil {
					return nil, err
				}
				where = append(where, "EXISTS (SELECT 1 FROM json_each(data, ?) WHERE value = ?)")
				args = append(args, path[0], s)
			}
		default:
			s, err := scalar(v)
			if err != nil {
				return nil, err
			}
			where = append(where, expr+" = ?")
			args = append(append(args, path...), s)
		}
	}
	anyKeys := make([]string, 0, len(f.AnyOf))
	for k := range f.AnyOf {
		anyKeys = append(anyKeys, k)
	}
	sort.Strings(anyKeys)
	for _, k := range anyKeys {
		if strings.ContainsAny(k, `"'`) {
			return nil, invalid("invalid filter key %q", k)
		}
		vals := f.AnyOf[k]
		if len(vals) == 0 {
			where = append(where, "0")
			continue
		}
		where = append(where, "json_extract(data, ?) IN ("+strings.TrimSuffix(strings.Repeat("?,", len(vals)), ",")+")")
		args = append(args, `$."`+k+`"`)
		for _, v := range vals {
			args = append(args, v)
		}
	}
	args = append(args, f.Limit, f.Offset)
	rows, err := r.b.db.QueryContext(ctx, `SELECT id, data, field_clock, owner_device, created_at, updated_at, archived_at FROM records WHERE `+
		strings.Join(where, " AND ")+` ORDER BY `+r.spec.order+` LIMIT ? OFFSET ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []T{}
	for rows.Next() {
		var id string
		var x row
		if err = rows.Scan(&id, &x.data, &x.fieldClock, &x.owner, &x.created, &x.updated, &x.archived); err != nil {
			return nil, err
		}
		v, err := decode[T](compose(id, &x))
		if err != nil {
			return nil, err
		}
		result = append(result, v)
	}
	return result, rows.Err()
}

func (r resource[T]) exists(ctx context.Context, q execer, kind, id string) (bool, error) {
	var n int
	err := q.QueryRowContext(ctx, `SELECT count(*) FROM records WHERE kind = ? AND id = ?`, kind, id).Scan(&n)
	return n > 0, err
}

// validate normalises doc in place and checks what Postgres constraints did.
func (r resource[T]) validate(ctx context.Context, q execer, id string, doc map[string]any) error {
	s := r.spec
	for _, key := range arrayFields {
		if v, ok := doc[key]; ok && v == nil {
			doc[key] = []any{}
		}
	}
	for key, def := range s.defaults {
		if v, ok := doc[key]; !ok || v == nil || v == "" {
			doc[key] = def
		}
	}
	for field, allowed := range s.enums {
		if v, ok := doc[field].(string); ok && !slices.Contains(allowed, v) {
			return invalid("%s %q is not allowed", field, v)
		}
	}
	uuids := slices.Clone(s.uuids)
	for _, f := range s.fks {
		uuids = append(uuids, f.field)
	}
	for _, field := range uuids {
		v, _ := doc[field].(string)
		if v == "" {
			doc[field] = nil
			continue
		}
		norm, err := parseID(v)
		if err != nil {
			return invalid("%s: invalid uuid %q", field, v)
		}
		doc[field] = norm
	}
	for _, f := range s.fks {
		if doc[f.field] == nil {
			if f.required {
				return invalid("%s is required", f.field)
			}
			continue
		}
		ok, err := r.exists(ctx, q, f.kind, doc[f.field].(string))
		if err != nil {
			return err
		}
		if !ok {
			return invalid("%s references a missing %s", f.field, f.kind)
		}
	}
	if s.check != nil {
		if err := s.check(doc); err != nil {
			return err
		}
	}
	for _, u := range s.uniques {
		query := `SELECT count(*) FROM records WHERE kind = ? AND id <> ?`
		args := []any{s.name, id}
		if u.live {
			query += ` AND archived_at IS NULL`
		}
		skip := false
		for _, f := range u.fields {
			if doc[f] == nil {
				skip = true // NULLs are never equal in a unique constraint
				break
			}
			query += ` AND json_extract(data, ?) = ?`
			val, err := scalar(doc[f])
			if err != nil {
				return err
			}
			args = append(args, `$."`+f+`"`, val)
		}
		if skip {
			continue
		}
		var n int
		if err := q.QueryRowContext(ctx, query, args...).Scan(&n); err != nil {
			return err
		}
		if n > 0 {
			return invalid("%s must be unique", strings.Join(u.fields, ", "))
		}
	}
	return nil
}

// stamp merges the clock of every field that differs between old and new.
func (r resource[T]) stamp(clock string, oldDoc, newDoc map[string]any) (string, error) {
	if !r.b.opts.Local {
		return clock, nil
	}
	m := map[string]string{}
	if err := json.Unmarshal([]byte(clock), &m); err != nil {
		m = map[string]string{}
	}
	at := r.b.opts.Clock().UTC().Format(timeLayout) + "@" + r.b.opts.Device
	for k, v := range newDoc {
		if !reflect.DeepEqual(oldDoc[k], v) {
			m[k] = at
		}
	}
	out, err := json.Marshal(m)
	return string(out), err
}

func (r resource[T]) Put(ctx context.Context, actor string, v T) (T, error) {
	var zero T
	if actor == "" {
		return zero, fmt.Errorf("%w: actor required", service.ErrInvalid)
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return zero, err
	}
	doc := map[string]any{}
	if err = json.Unmarshal(raw, &doc); err != nil {
		return zero, err
	}
	id, _ := doc["id"].(string)
	if id == "" {
		id = newUUID()
	} else if id, err = parseID(id); err != nil {
		return zero, err
	}
	for _, c := range columnFields {
		delete(doc, c)
	}
	var out []byte
	err = r.b.write(ctx, func(q execer) error {
		cur, err := r.load(ctx, q, id)
		if err != nil {
			return err
		}
		if cur != nil && cur.archived.Valid {
			return service.ErrNotFound
		}
		if err = r.validate(ctx, q, id, doc); err != nil {
			return err
		}
		oldDoc := map[string]any{}
		now := r.now()
		if cur != nil {
			json.Unmarshal([]byte(cur.data), &oldDoc)
		}
		if r.spec.name == "itemplan" {
			if cur != nil {
				// A stored version is immutable; only accepted_at changes.
				next := maps(oldDoc)
				next["accepted_at"] = doc["accepted_at"]
				doc = next
			} else {
				var max sql.NullInt64
				if err = q.QueryRowContext(ctx, `SELECT max(json_extract(data, '$.version')) FROM records WHERE kind = 'itemplan' AND json_extract(data, '$.item_id') = ?`, doc["item_id"]).Scan(&max); err != nil {
					return err
				}
				doc["version"] = max.Int64 + 1
			}
		}
		data, err := json.Marshal(doc)
		if err != nil {
			return err
		}
		if cur == nil {
			clock, err := r.stamp("{}", nil, doc)
			if err != nil {
				return err
			}
			_, err = q.ExecContext(ctx, `INSERT INTO records (kind, id, data, field_clock, owner_device, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
				r.spec.name, id, string(data), clock, r.b.opts.Device, now, now)
			if err != nil {
				return err
			}
			cur = &row{data: string(data), created: now, updated: now}
		} else {
			clock, err := r.stamp(cur.fieldClock, oldDoc, doc)
			if err != nil {
				return err
			}
			if _, err = q.ExecContext(ctx, `UPDATE records SET data = ?, field_clock = ?, updated_at = ? WHERE kind = ? AND id = ?`,
				string(data), clock, now, r.spec.name, id); err != nil {
				return err
			}
			cur.data, cur.updated = string(data), now
		}
		out, err = compose(id, cur)
		return err
	})
	if err != nil {
		return zero, err
	}
	return decode[T](out, nil)
}

func maps(m map[string]any) map[string]any {
	c := make(map[string]any, len(m))
	for k, v := range m {
		c[k] = v
	}
	return c
}

func (r resource[T]) Archive(ctx context.Context, actor, id string) error {
	if actor == "" {
		return fmt.Errorf("%w: actor required", service.ErrInvalid)
	}
	id, err := parseID(id)
	if err != nil {
		return err
	}
	return r.b.write(ctx, func(q execer) error {
		cur, err := r.load(ctx, q, id)
		if err != nil {
			return err
		}
		if cur == nil || cur.archived.Valid {
			return service.ErrNotFound
		}
		now := r.now()
		clock := cur.fieldClock
		if r.b.opts.Local {
			clock, err = r.stamp(clock, nil, map[string]any{"archived_at": now})
			if err != nil {
				return err
			}
		}
		if _, err = q.ExecContext(ctx, `UPDATE records SET archived_at = ?, updated_at = ?, field_clock = ? WHERE kind = ? AND id = ?`, now, now, clock, r.spec.name, id); err != nil {
			return err
		}
		if r.spec.name != "project" {
			return nil
		}
		// Archiving a project archives its repositories, items and features with it.
		_, err = q.ExecContext(ctx, `UPDATE records SET archived_at = ?, updated_at = ? WHERE kind IN ('repository','item','feature') AND archived_at IS NULL AND json_extract(data, '$.project_id') = ?`, now, now, id)
		return err
	})
}
