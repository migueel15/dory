package runtime

import (
	"context"
	"github.com/migueel15/dory/internal/workspace"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func (a Antigravity) list(ctx context.Context, limit int) ([]External, error) {
	c, err := openACP(ctx, a.bin(), "", false)
	if err != nil {
		return nil, err
	}
	defer c.close()
	if _, err := c.initialize(ctx); err != nil {
		return nil, err
	}
	var result []External
	params := acpParams{}
	for page := 0; page < 10; page++ {
		var response struct {
			Sessions   []struct{ SessionID, Cwd, Title, UpdatedAt string }
			NextCursor string
		}
		if err := c.call(ctx, "session/list", params, &response, nil); err != nil {
			return result, err
		}
		for _, session := range response.Sessions {
			if session.SessionID == "" || !filepath.IsAbs(session.Cwd) || strings.HasPrefix(filepath.Base(session.Cwd), "dory-agy-models-") {
				continue
			}
			title := session.Title
			if title == "" {
				title = "Antigravity · " + filepath.Base(session.Cwd)
			}
			result = append(result, External{Provider: antigravityID, ID: session.SessionID, Title: title, Dir: session.Cwd, UpdatedAt: session.UpdatedAt})
			if len(result) == limit {
				return result, nil
			}
		}
		if response.NextCursor == "" || response.NextCursor == params["cursor"] {
			break
		}
		params["cursor"] = response.NextCursor
	}
	return result, nil
}

// ListExternal includes the ACP sessions of the local Antigravity CLI.
func (m *Manager) ListExternal(ctx context.Context, p Paths) []External {
	out := ExternalSessions(p)
	if a, ok := m.provider(antigravityID); ok {
		if a, ok := a.(Antigravity); ok {
			probe, cancel := context.WithTimeout(ctx, 8*time.Second)
			list, _ := a.list(probe, externalLimit)
			cancel()
			out = append(out, list...)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].UpdatedAt > out[j].UpdatedAt })
	return out
}

func (m *Manager) FindExternal(ctx context.Context, p Paths, provider, id string) (External, error) {
	if provider != antigravityID {
		return ExternalSession(p, provider, id)
	}
	for _, session := range m.ListExternal(ctx, p) {
		if session.Provider == provider && session.ID == id {
			if branch, err := workspace.CurrentBranch(ctx, session.Dir); err == nil {
				session.Branch = branch
			}
			return session, nil
		}
	}
	return External{}, ErrNotFound
}

func (m *Manager) ReadExternal(ctx context.Context, e External) ([]Message, error) {
	if e.Provider != antigravityID {
		return ExternalMessages(e)
	}
	p, ok := m.provider(antigravityID)
	if !ok {
		return nil, ErrProvider
	}
	a, ok := p.(Antigravity)
	if !ok {
		return nil, ErrProvider
	}
	return a.history(ctx, e)
}

func (a Antigravity) history(ctx context.Context, e External) ([]Message, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	dir := e.Dir
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		// Pass the original working directory to ACP. Some releases can still
		// replay sessions whose directory was removed.
		dir = ""
	}
	c, err := openACP(ctx, a.bin(), dir, false)
	if err != nil {
		return nil, err
	}
	defer c.close()
	if _, err := c.initialize(ctx); err != nil {
		return nil, err
	}
	s := newAntigravitySession(ctx, c, Turn{RunID: e.ID, Dir: e.Dir, Session: e.ID})
	s.id = e.ID
	var result []Message
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
	err = c.call(ctx, "session/load", acpParams{"sessionId": e.ID, "cwd": e.Dir, "mcpServers": []any{}}, nil, func(f acpFrame) error {
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
