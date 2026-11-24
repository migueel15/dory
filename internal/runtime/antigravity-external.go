package runtime

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/migueel15/dory/internal/workspace"
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

func (a Antigravity) List(ctx context.Context, _ Paths) []External {
	probe, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	list, _ := a.list(probe, externalLimit)
	for i := range list {
		if branch, err := workspace.CurrentBranch(probe, list[i].Dir); err == nil {
			list[i].Branch = branch
		}
	}
	return list
}

func (Antigravity) SessionFile(Paths, string) string { return "" }

func (a Antigravity) Read(ctx context.Context, e External) ([]Message, error) {
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
