package localstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/migueel15/dory/internal/service"
	"github.com/migueel15/dory/node/syncx"
)

// audit appends an event for a write, in the caller's transaction. Password and
// credential hashes never reach the log.
func (b *backend) audit(ctx context.Context, q execer, actor, kind, id, operation string, record []byte) error {
	var snapshot map[string]any
	if err := json.Unmarshal(record, &snapshot); err != nil {
		return err
	}
	delete(snapshot, "password_hash")
	delete(snapshot, "secret_hash")
	change, err := json.Marshal(map[string]any{"operation": operation, "record": snapshot})
	if err != nil {
		return err
	}
	return b.appendEvent(ctx, q, actor, kind, id, change)
}

func (b *backend) appendEvent(ctx context.Context, q execer, actor, kind, id string, change []byte) error {
	if b.log == nil || !syncx.EventReplicated(kind) {
		_, err := q.ExecContext(ctx, `INSERT INTO events (device, seq, entity_type, entity_id, actor, change, at)
			VALUES (?, (SELECT coalesce(max(seq), 0) + 1 FROM events WHERE device = ?), ?, ?, ?, ?, ?)`,
			b.opts.Device, b.opts.Device, kind, id, actor, string(change), b.opts.Clock().UTC().Format(timeLayout))
		return err
	}
	// A replicated event is stored in canonical JSON and logged as an op, so
	// every replica ends with the same bytes.
	var parsed any
	if err := json.Unmarshal(change, &parsed); err != nil {
		return err
	}
	canonical, err := json.Marshal(parsed)
	if err != nil {
		return err
	}
	h := b.log.Now()
	at := syncx.FormatTime(h)
	var seq int64
	err = q.QueryRowContext(ctx, `INSERT INTO events (device, seq, entity_type, entity_id, actor, change, at)
		VALUES (?, (SELECT coalesce(max(seq), 0) + 1 FROM events WHERE device = ?), ?, ?, ?, ?, ?) RETURNING seq`,
		b.opts.Device, b.opts.Device, kind, id, actor, string(canonical), at).Scan(&seq)
	if err != nil {
		return err
	}
	_, _, err = b.log.Commit(ctx, q, h, syncx.KindEvent, "event", fmt.Sprintf("%s:%d", b.opts.Device, seq), syncx.Fields{Event: map[string]any{
		"device": b.opts.Device, "seq": seq, "entity_type": kind, "entity_id": id, "actor": actor, "change": parsed, "at": at,
	}})
	return err
}

func (b *backend) RecordEvent(ctx context.Context, actor, kind, id string, change any) error {
	raw, err := json.Marshal(change)
	if err != nil {
		return err
	}
	return b.write(ctx, func(q execer) error { return b.appendEvent(ctx, q, actor, kind, id, raw) })
}

func (b *backend) ListEvents(ctx context.Context, after int64, kind string, limit int32) ([]service.Event, error) {
	if limit < 0 || limit > 1000 || after < 0 {
		return nil, service.ErrInvalid
	}
	if limit == 0 {
		limit = 100
	}
	rows, err := b.db.QueryContext(ctx, `SELECT id, entity_type, entity_id, actor, change, at FROM events
		WHERE id > ? AND (? = '' OR entity_type = ?) ORDER BY id LIMIT ?`, after, kind, kind, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []service.Event{}
	for rows.Next() {
		var e service.Event
		var change, at string
		if err = rows.Scan(&e.ID, &e.EntityType, &e.EntityID, &e.Actor, &change, &at); err != nil {
			return nil, err
		}
		e.Change = json.RawMessage(change)
		if e.At, err = time.Parse(timeLayout, at); err != nil {
			return nil, err
		}
		result = append(result, e)
	}
	return result, rows.Err()
}

func (b *backend) LatestEventID(ctx context.Context) (int64, error) {
	var id int64
	err := b.db.QueryRowContext(ctx, `SELECT coalesce(max(id), 0) FROM events`).Scan(&id)
	return id, err
}

// searchText is what each kind contributes to full text search, as in Postgres.
var searchFields = map[string][]string{
	"item":    {"title", "raw_text", "body_md"},
	"feature": {"title", "body_md"},
	"memory":  {"title", "content"},
	"project": {"name", "slug", "description"},
}

func (b *backend) unindexSearch(ctx context.Context, q execer, kind, id string) error {
	if _, ok := searchFields[kind]; !ok {
		return nil
	}
	_, err := q.ExecContext(ctx, `DELETE FROM search WHERE entity_type = ? AND entity_id = ?`, kind, id)
	return err
}

func (b *backend) indexSearch(ctx context.Context, q execer, kind, id string, doc map[string]any) error {
	fields, ok := searchFields[kind]
	if !ok {
		return nil
	}
	parts := make([]string, 0, len(fields))
	for _, f := range fields {
		if s := str(doc, f); s != "" {
			parts = append(parts, s)
		}
	}
	if err := b.unindexSearch(ctx, q, kind, id); err != nil {
		return err
	}
	_, err := q.ExecContext(ctx, `INSERT INTO search (entity_type, entity_id, body) VALUES (?, ?, ?)`, kind, id, strings.Join(parts, " "))
	return err
}

// matchQuery turns web-search syntax (words, "phrases", -excluded, OR) into an
// FTS5 expression. Words match as prefixes. ok is false when nothing can match.
func matchQuery(query string) (expr string, ok bool) {
	type term struct {
		text   string
		neg    bool
		phrase bool
	}
	var groups [][]term
	cur := []term{}
	flush := func() {
		if len(cur) > 0 {
			groups = append(groups, cur)
		}
		cur = []term{}
	}
	runes := []rune(query)
	isWord := func(r rune) bool { return unicodeLetterOrDigit(r) }
	for i := 0; i < len(runes); {
		r := runes[i]
		neg := false
		if r == '-' && i+1 < len(runes) && (runes[i+1] == '"' || isWord(runes[i+1])) && (i == 0 || !isWord(runes[i-1])) {
			neg = true
			i++
			r = runes[i]
		}
		switch {
		case r == '"':
			j := i + 1
			for j < len(runes) && runes[j] != '"' {
				j++
			}
			words := splitWords(string(runes[i+1 : j]))
			if len(words) > 0 {
				cur = append(cur, term{text: `"` + strings.Join(words, " ") + `"`, neg: neg, phrase: true})
			}
			i = j + 1
		case isWord(r):
			j := i
			for j < len(runes) && isWord(runes[j]) {
				j++
			}
			w := string(runes[i:j])
			i = j
			if strings.EqualFold(w, "or") && !neg {
				flush()
				continue
			}
			cur = append(cur, term{text: `"` + w + `"*`, neg: neg})
		default:
			i++
		}
	}
	flush()
	var alts []string
	for _, g := range groups {
		var pos, neg []string
		for _, t := range g {
			if t.neg {
				neg = append(neg, t.text)
			} else {
				pos = append(pos, t.text)
			}
		}
		if len(pos) == 0 {
			continue
		}
		e := strings.Join(pos, " AND ")
		if len(neg) > 0 {
			e = "(" + e + ") NOT (" + strings.Join(neg, " OR ") + ")"
		}
		alts = append(alts, "("+e+")")
	}
	if len(alts) == 0 {
		return "", false
	}
	return strings.Join(alts, " OR "), true
}

func (b *backend) Search(ctx context.Context, query string, limit, offset int32) ([]service.SearchHit, error) {
	if offset < 0 || limit < 0 || limit > 1000 {
		return nil, service.ErrInvalid
	}
	if limit == 0 {
		limit = 100
	}
	expr, ok := matchQuery(query)
	if !ok {
		return []service.SearchHit{}, nil
	}
	// bm25 is negative and lower is better; negating it gives a ts_rank-like score.
	rows, err := b.db.QueryContext(ctx, `SELECT search.entity_type, search.entity_id, -bm25(search) AS rank,
		r.data, r.field_clock, r.owner_device, r.created_at, r.updated_at, r.archived_at
		FROM search JOIN records r ON r.kind = search.entity_type AND r.id = search.entity_id
		WHERE search MATCH ? AND r.archived_at IS NULL
		ORDER BY rank DESC, search.entity_type, search.entity_id LIMIT ? OFFSET ?`, expr, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("%w: search: %v", service.ErrInvalid, err)
	}
	defer rows.Close()
	result := []service.SearchHit{}
	for rows.Next() {
		var h service.SearchHit
		var x row
		var rank float64
		if err = rows.Scan(&h.EntityType, &h.EntityID, &rank, &x.data, &x.fieldClock, &x.owner, &x.created, &x.updated, &x.archived); err != nil {
			return nil, err
		}
		h.Rank = float32(rank)
		content, err := compose(h.EntityID, &x)
		if err != nil {
			return nil, err
		}
		h.Content = content
		result = append(result, h)
	}
	return result, rows.Err()
}

func (b *backend) findAuth(ctx context.Context, kind, field, value string) (id string, x *row, err error) {
	x = &row{}
	err = b.db.QueryRowContext(ctx, `SELECT id, data, field_clock, owner_device, created_at, updated_at, archived_at FROM auth_records
		WHERE kind = ? AND json_extract(data, ?) = ? AND archived_at IS NULL`, kind, `$."`+field+`"`, value).
		Scan(&id, &x.data, &x.fieldClock, &x.owner, &x.created, &x.updated, &x.archived)
	if err == sql.ErrNoRows {
		err = service.ErrNotFound
	}
	return
}

// userActive reports whether the user exists and is not archived.
func (b *backend) userActive(ctx context.Context, id string) (bool, error) {
	var n int
	err := b.db.QueryRowContext(ctx, `SELECT count(*) FROM auth_records WHERE kind = 'user' AND id = ? AND archived_at IS NULL`, id).Scan(&n)
	return n > 0, err
}

func (b *backend) FindUser(ctx context.Context, username string) (service.User, error) {
	id, x, err := b.findAuth(ctx, "user", "username", username)
	if err != nil {
		return service.User{}, err
	}
	return decode[service.User](compose(id, x))
}

// live checks the owner and the optional expiry, as the Postgres queries do.
func (b *backend) live(ctx context.Context, userID string, expires *time.Time) error {
	if expires != nil && !expires.After(b.opts.Clock()) {
		return service.ErrNotFound
	}
	if ok, err := b.userActive(ctx, userID); err != nil {
		return err
	} else if !ok {
		return service.ErrNotFound
	}
	return nil
}

func (b *backend) FindSession(ctx context.Context, hash string) (service.Session, error) {
	id, x, err := b.findAuth(ctx, "session", "secret_hash", hash)
	if err != nil {
		return service.Session{}, err
	}
	s, err := decode[service.Session](compose(id, x))
	if err != nil {
		return s, err
	}
	if err = b.live(ctx, s.UserID, &s.ExpiresAt); err != nil {
		return service.Session{}, err
	}
	return s, nil
}

func (b *backend) FindToken(ctx context.Context, hash string) (service.Token, error) {
	id, x, err := b.findAuth(ctx, "token", "secret_hash", hash)
	if err != nil {
		return service.Token{}, err
	}
	t, err := decode[service.Token](compose(id, x))
	if err != nil {
		return t, err
	}
	if err = b.live(ctx, t.UserID, t.ExpiresAt); err != nil {
		return service.Token{}, err
	}
	return t, nil
}

func unicodeLetterOrDigit(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }

func splitWords(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool { return !unicodeLetterOrDigit(r) })
}
