package data

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/migueel15/dory/internal/apperr"
	"github.com/migueel15/dory/internal/oplog"
	"github.com/migueel15/dory/internal/schema"
	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

var ErrConflict = apperr.New(apperr.Conflict, "resource already exists")
var ErrRevisionConflict = apperr.New(apperr.RevisionConflict, "resource changed since it was read")

// Fields contains only the columns to change. Nil clears a nullable column.
type Fields map[string]any

// Create inserts a new row, filling in the schema defaults. A key already
// present, including an archived row, is a conflict.
func (r Resource[T]) Create(ctx context.Context, v T) (T, error) {
	return r.save(ctx, v, false)
}

// Upsert creates or updates a row with a natural identity: repositories,
// repository locations, skill states, settings and task dependencies.
func (r Resource[T]) Upsert(ctx context.Context, v T) (T, error) {
	switch r.entity {
	case "repository", "repository_location", "skill_state", "setting", "task_dependency":
		return r.save(ctx, v, true)
	default:
		var zero T
		return zero, ErrInvalid.Msgf("%s does not support upsert", r.entity)
	}
}

func (r Resource[T]) save(ctx context.Context, v T, upsert bool) (T, error) {
	var out T
	fields, err := jsonFields(v)
	if err != nil {
		return out, err
	}
	spec, _ := schema.Lookup(r.entity)
	id, err := r.identity(fields, spec.Key)
	if err != nil {
		return out, err
	}
	input := Fields{}
	for _, col := range spec.Columns {
		if col.Name != "created_at" && col.Name != "updated_at" {
			input[col.Name] = fields[col.Name]
		}
	}
	err = r.s.log.Tx(ctx, func(tx *oplog.Tx) error {
		raw, err := r.row(ctx, tx, id, true)
		switch {
		case errors.Is(err, ErrNotFound):
			out, err = r.insert(ctx, tx, spec, id, input)
		case err == nil && !upsert:
			return ErrConflict.Msgf("%s %q", r.entity, id)
		case err == nil:
			out, err = r.update(ctx, tx, spec, id, raw, nil, input)
		}
		return err
	})
	return writeResult(out, err)
}

// Patch updates only the supplied columns of an existing row.
func (r Resource[T]) Patch(ctx context.Context, id string, f Fields) (T, error) {
	return r.patch(ctx, id, nil, f)
}

// PatchIf applies a patch only if updated_at still matches the caller's version.
func (r Resource[T]) PatchIf(ctx context.Context, id, expectedUpdatedAt string, f Fields) (T, error) {
	return r.patch(ctx, id, &expectedUpdatedAt, f)
}

func (r Resource[T]) patch(ctx context.Context, id string, expected *string, f Fields) (T, error) {
	var out T
	fields, err := jsonFields(f)
	if err != nil {
		return out, err
	}
	spec, _ := schema.Lookup(r.entity)
	err = r.s.log.Tx(ctx, func(tx *oplog.Tx) error {
		out, err = r.patchTx(ctx, tx, spec, id, expected, fields)
		return err
	})
	return writeResult(out, err)
}

func (r Resource[T]) patchTx(ctx context.Context, tx *oplog.Tx, spec schema.Entity, id string, expected *string, fields Fields) (T, error) {
	// An explicit archival edit may reopen or modify an archived row.
	archivedAt, archivalEdit := fields["archived_at"]
	raw, err := r.row(ctx, tx, id, archivalEdit && (archivedAt == nil || expected != nil))
	if err != nil {
		var zero T
		return zero, err
	}
	return r.update(ctx, tx, spec, id, raw, expected, fields)
}

// Archive hides a row. It replicates like any other change.
func (r Resource[T]) Archive(ctx context.Context, id string) error {
	_, err := r.Patch(ctx, id, Fields{"archived_at": r.s.now().UTC().Format(time.RFC3339Nano)})
	return err
}

func (r Resource[T]) insert(ctx context.Context, tx *oplog.Tx, spec schema.Entity, id string, fields Fields) (T, error) {
	for col, def := range spec.Defaults() {
		if empty(fields[col]) {
			fields[col] = def
		}
	}
	fields["created_at"] = r.s.now().UTC().Format(time.RFC3339Nano)
	if _, ok := spec.Column("updated_at"); ok {
		fields["updated_at"] = fields["created_at"]
	}
	return r.write(ctx, tx, id, fields)
}

func (r Resource[T]) update(ctx context.Context, tx *oplog.Tx, spec schema.Entity, id string, raw json.RawMessage, expected *string, fields Fields) (T, error) {
	var zero T
	current, err := readFields(raw)
	if err != nil {
		return zero, err
	}
	updatedAt, hasVersion := current["updated_at"].(string)
	if expected != nil && (!hasVersion || *expected != updatedAt) {
		return zero, ErrRevisionConflict.Msgf("%s %q", r.entity, id)
	}
	changed := Fields{}
	for name, value := range fields {
		col, ok := spec.Column(name)
		if !ok || name == "created_at" || name == "updated_at" {
			return zero, ErrInvalid.Msgf("%s has no writable column %q", r.entity, name)
		}
		if !validValue(col, value) {
			return zero, ErrInvalid.Msgf("incompatible value for %s.%s", r.entity, name)
		}
		if !same(current[name], value) {
			changed[name] = value
		}
	}
	if len(changed) == 0 {
		return decode[T](raw)
	}
	if hasVersion {
		now := r.s.now().UTC()
		if previous, err := time.Parse(time.RFC3339Nano, updatedAt); err == nil && !now.After(previous) {
			now = previous.Add(time.Nanosecond)
		}
		changed["updated_at"] = now.Format(time.RFC3339Nano)
	}
	return r.write(ctx, tx, id, changed)
}

func (r Resource[T]) write(ctx context.Context, tx *oplog.Tx, id string, f Fields) (T, error) {
	var zero T
	if remote, ok := f["remote_url"].(string); ok && r.entity == "repository" && oplog.RepositoryID(remote) != id {
		return zero, ErrInvalid.Msgf("remote_url changes the repository identity")
	}
	if _, err := tx.Write(r.entity, id, f); err != nil {
		return zero, err
	}
	out, err := r.get(ctx, tx, id, true)
	if _, ok := errors.AsType[*json.UnmarshalTypeError](err); ok {
		return zero, ErrInvalid.Msgf("%v", err)
	}
	return out, err
}

func writeResult[T any](out T, err error) (T, error) {
	if err == nil {
		return out, nil
	}
	if sqlErr, ok := errors.AsType[*sqlite.Error](err); ok && sqlErr.Code()&0xff == sqlite3.SQLITE_CONSTRAINT {
		err = ErrInvalid.Msgf("%v", err)
	}
	var zero T
	return zero, err
}

func jsonFields(v any) (Fields, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, ErrInvalid.Msgf("%v", err)
	}
	return readFields(raw)
}

func readFields(raw []byte) (Fields, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var fields Fields
	err := dec.Decode(&fields)
	return fields, err
}

func validValue(col schema.Column, value any) bool {
	if value == nil {
		return col.Nullable
	}
	switch col.Kind {
	case schema.Text:
		_, ok := value.(string)
		return ok
	case schema.Time:
		s, ok := value.(string)
		_, err := time.Parse(time.RFC3339Nano, s)
		return ok && err == nil
	case schema.Int:
		n, ok := value.(json.Number)
		_, err := n.Int64()
		return ok && err == nil
	case schema.Bool:
		_, ok := value.(bool)
		return ok
	case schema.JSON:
		// Materialization and decoding validate the column's JSON shape.
		// SQLite stores JSON text, so prevent quoted JSON from being unwrapped.
		switch value.(type) {
		case map[string]any, []any:
			return true
		}
	}
	return false
}
