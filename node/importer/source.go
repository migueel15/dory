package importer

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// source reads the REST API of the running Dory. It only issues GET requests,
// so a token with the read scope is enough.
type source struct {
	base   string
	token  string
	client *http.Client
	idle   time.Duration
}

const pageSize = 500

func (s *source) get(ctx context.Context, path string, q url.Values) (*http.Response, error) {
	u := strings.TrimRight(s.base, "/") + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+s.token)
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

// statusError is a non-200 answer of the source.
type statusError struct {
	path   string
	status int
}

func (e *statusError) Error() string {
	return fmt.Sprintf("importer: GET %s: status %d", e.path, e.status)
}

// list reads every record of a collection, archived ones included, page by page.
func (s *source) list(ctx context.Context, path string, archived bool) ([]map[string]any, error) {
	var out []map[string]any
	for offset := 0; ; offset += pageSize {
		q := url.Values{"limit": {fmt.Sprint(pageSize)}, "offset": {fmt.Sprint(offset)}}
		if archived {
			q.Set("include_archived", "true")
		}
		resp, err := s.get(ctx, path, q)
		if err != nil {
			return nil, err
		}
		var page []map[string]any
		err = decodeBody(resp, path, &page)
		if err != nil {
			return nil, err
		}
		out = append(out, page...)
		if len(page) < pageSize {
			return out, nil
		}
	}
}

func decodeBody(resp *http.Response, path string, v any) error {
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		io.Copy(io.Discard, resp.Body)
		return &statusError{path, resp.StatusCode}
	}
	return json.NewDecoder(resp.Body).Decode(v)
}

// sourceEvent is one audit event of the source.
type sourceEvent struct {
	ID         int64           `json:"id"`
	EntityType string          `json:"entity_type"`
	EntityID   string          `json:"entity_id"`
	Actor      string          `json:"actor"`
	Change     json.RawMessage `json:"change"`
	At         time.Time       `json:"at"`
}

// events reads the audit events with id greater than after. The API only
// exposes them as a server-sent stream that never ends, so the stream is read
// until it has been quiet for s.idle: the server sends all the pending events
// at once, in batches, before it starts to wait.
func (s *source) events(ctx context.Context, after int64) ([]sourceEvent, error) {
	parent := ctx
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	resp, err := s.get(ctx, "/api/events/stream", url.Values{"after": {fmt.Sprint(after)}})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, &statusError{"/api/events/stream", resp.StatusCode}
	}
	timer := time.AfterFunc(s.idle, cancel)
	defer timer.Stop()
	var out []sourceEvent
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 64*1024), 64<<20)
	var data string
	for sc.Scan() {
		timer.Reset(s.idle)
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, "data: "):
			data = line[len("data: "):]
		case line == "" && data != "":
			var e sourceEvent
			if err := json.Unmarshal([]byte(data), &e); err != nil {
				return nil, fmt.Errorf("importer: invalid event: %w", err)
			}
			data = ""
			if e.ID > after { // monitor pushes carry no id
				out = append(out, e)
				after = e.ID
			}
		}
	}
	if err := parent.Err(); err != nil {
		return nil, err
	}
	if err := sc.Err(); err != nil && !errors.Is(err, context.Canceled) {
		return nil, err
	}
	return out, nil
}
