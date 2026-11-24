package acp

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/migueel15/dory/internal/harness"
	"github.com/migueel15/dory/internal/workspace"
)

func (d *driver) list(ctx context.Context, limit int) ([]harness.External, error) {
	c, err := d.open(ctx, "", false, "")
	if err != nil {
		return nil, err
	}
	defer c.close()
	if _, err := c.initialize(ctx, &d.p); err != nil {
		return nil, err
	}
	var result []harness.External
	p := params{}
	for page := 0; page < 10; page++ {
		var response struct {
			Sessions   []struct{ SessionID, Cwd, Title, UpdatedAt string }
			NextCursor string
		}
		if err := c.call(ctx, "session/list", p, &response, nil); err != nil {
			return result, err
		}
		for _, session := range response.Sessions {
			base := filepath.Base(session.Cwd)
			if session.SessionID == "" || !filepath.IsAbs(session.Cwd) || strings.HasPrefix(base, probePrefix) || strings.HasPrefix(base, legacyProbePrefix) {
				continue
			}
			title := session.Title
			if title == "" {
				title = d.p.Name + " · " + base
			}
			result = append(result, harness.External{Provider: d.p.ID, ID: session.SessionID, Title: title, Dir: session.Cwd, UpdatedAt: session.UpdatedAt})
			if len(result) == limit {
				return result, nil
			}
		}
		if response.NextCursor == "" || response.NextCursor == p["cursor"] {
			break
		}
		p["cursor"] = response.NextCursor
	}
	return result, nil
}

func (r rich) List(ctx context.Context, _ harness.Paths) []harness.External {
	if !r.p.Sessions {
		return nil
	}
	probe, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	list, _ := r.list(probe, sessionListLimit)
	for i := range list {
		if branch, err := workspace.CurrentBranch(probe, list[i].Dir); err == nil {
			list[i].Branch = branch
		}
	}
	return list
}

func (rich) SessionFile(harness.Paths, string) string { return "" }

// Read replays a saved session and returns its messages.
func (r rich) Read(ctx context.Context, e harness.External) ([]harness.Message, error) {
	if !r.p.Sessions {
		return nil, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	dir := e.Dir
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		// Pass the original working directory to ACP. Some releases can still
		// replay sessions whose directory was removed.
		dir = ""
	}
	c, err := r.open(ctx, dir, false, "")
	if err != nil {
		return nil, err
	}
	defer c.close()
	if _, err := c.initialize(ctx, &r.p); err != nil {
		return nil, err
	}
	s := newSession(ctx, c, harness.Turn{RunID: e.ID, Dir: e.Dir, Session: e.ID}, r.p.DefaultModel.Slug)
	s.id = e.ID
	var result []harness.Message
	done := make(chan struct{})
	go func() {
		defer close(done)
		index := map[string]int{}
		for event := range s.events {
			if msg := event.Message; msg != nil {
				if i, ok := index[msg.ID]; ok {
					result[i] = *msg
				} else {
					index[msg.ID] = len(result)
					result = append(result, *msg)
				}
			}
		}
	}()
	err = c.call(ctx, "session/load", params{"sessionId": e.ID, "cwd": e.Dir, "mcpServers": []any{}}, nil, func(f rpcFrame) error {
		if f.Method == "session/update" {
			s.update(f.Params)
			return nil
		}
		return s.handle(f)
	})
	s.finishChunk()
	close(s.events)
	<-done
	return result, err
}
