package api

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/migueel15/dory/internal/dictation"
	"github.com/migueel15/dory/internal/service"
	"github.com/migueel15/dory/internal/t3"
)

// maxChunkBytes fits ~30 s of 16 kHz mono 16-bit WAV with room to spare.
const maxChunkBytes = 2 << 20

type dictationTarget struct {
	// Kind is capture (create an inbox item) or define (write in the item's definition thread).
	Kind      string `json:"kind"`
	ProjectID string `json:"project_id"`
	ItemID    string `json:"item_id"`
}

func mountDictation(mux *http.ServeMux, svc service.Service, monitor *t3.Monitor, dict *dictation.Manager) {
	mux.HandleFunc("POST /api/dictation", func(w http.ResponseWriter, r *http.Request) {
		st, err := dict.Create(actor(r))
		if err != nil {
			writeDictationError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, st)
	})
	mux.HandleFunc("GET /api/dictation/{id}", func(w http.ResponseWriter, r *http.Request) {
		st, err := dict.Get(actor(r), r.PathValue("id"))
		if err != nil {
			writeDictationError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, st)
	})
	mux.HandleFunc("POST /api/dictation/{id}/chunks", func(w http.ResponseWriter, r *http.Request) {
		seq, err := strconv.Atoi(r.URL.Query().Get("seq"))
		if err != nil || seq < 0 {
			writeError(w, http.StatusBadRequest, "invalid_argument", "seq must be a non-negative integer")
			return
		}
		wav, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxChunkBytes))
		if err != nil || len(wav) == 0 {
			writeError(w, http.StatusBadRequest, "invalid_argument", "chunk must be a WAV body up to 2 MiB")
			return
		}
		st, err := dict.Add(actor(r), r.PathValue("id"), seq, wav)
		if err != nil {
			writeDictationError(w, err)
			return
		}
		writeJSON(w, http.StatusAccepted, st)
	})
	mux.HandleFunc("POST /api/dictation/{id}/finish", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Mode   string           `json:"mode"`
			Target *dictationTarget `json:"target"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_argument", "invalid body")
			return
		}
		var deliver dictation.Deliver
		switch body.Mode {
		case "insert":
		case "send":
			if body.Target == nil {
				writeError(w, http.StatusBadRequest, "invalid_argument", "target is required in send mode")
				return
			}
			var ok bool
			if deliver, ok = dictationDeliver(w, r, svc, monitor, *body.Target); !ok {
				return
			}
		default:
			writeError(w, http.StatusBadRequest, "invalid_argument", "mode must be insert or send")
			return
		}
		st, err := dict.Finish(r.Context(), actor(r), r.PathValue("id"), deliver)
		if err != nil {
			writeDictationError(w, err)
			return
		}
		status := http.StatusOK
		if deliver != nil {
			status = http.StatusAccepted
		}
		writeJSON(w, status, st)
	})
}

// dictationDeliver checks the target now and returns how to deliver the text
// later, when the request context is already gone.
func dictationDeliver(w http.ResponseWriter, r *http.Request, svc service.Service, monitor *t3.Monitor, target dictationTarget) (dictation.Deliver, bool) {
	who := actor(r)
	switch target.Kind {
	case "capture":
		var project *string
		if target.ProjectID != "" {
			p := target.ProjectID
			project = &p
		}
		return func(ctx context.Context, text string) error {
			_, err := svc.Items().Put(ctx, who, captureItem(text, project))
			return err
		}, true
	case "define":
		item, err := svc.Items().Get(r.Context(), target.ItemID)
		if err != nil {
			writeServiceError(w, err)
			return nil, false
		}
		link, err := refineThread(r, svc, item.ID)
		if err != nil {
			writeServiceError(w, err)
			return nil, false
		}
		if link == nil {
			writeError(w, http.StatusConflict, "conflict", "the item has no definition thread yet")
			return nil, false
		}
		return func(ctx context.Context, text string) error {
			c, err := monitor.ThreadClient(ctx, link.ThreadID)
			if err != nil {
				if c, err = monitor.InstanceClient(ctx, link.InstanceID); err != nil {
					return err
				}
			}
			if err = c.SendMessage(ctx, link.ThreadID, text); err != nil {
				return err
			}
			monitor.Refresh(link.InstanceID)
			if err := svc.RecordEvent(ctx, who, "t3_thread", link.ThreadID, map[string]any{"operation": "t3.message", "item_id": item.ID, "text": text, "source": "dictation"}); err != nil {
				logError(err)
			}
			return nil
		}, true
	}
	writeError(w, http.StatusBadRequest, "invalid_argument", "target.kind must be capture or define")
	return nil, false
}

// captureItem mirrors the web capture: raw text in the inbox, title pending.
func captureItem(text string, project *string) service.Item {
	title := strings.TrimSpace(strings.SplitN(strings.TrimSpace(text), "\n", 2)[0])
	if r := []rune(title); len(r) > 80 {
		title = string(r[:80])
	}
	var item service.Item
	item.ID = newUUID()
	item.Title, item.RawText, item.Status, item.Source, item.TitleSource = title, text, "inbox", "web", "pending"
	item.Tags, item.Refs, item.ProjectID = []string{}, []string{}, project
	return item
}

func newUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

func writeDictationError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, dictation.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", err.Error())
	case errors.Is(err, dictation.ErrUnavailable):
		writeError(w, http.StatusServiceUnavailable, "unavailable", err.Error())
	case errors.Is(err, dictation.ErrFinished), errors.Is(err, dictation.ErrOutOfOrder):
		writeError(w, http.StatusConflict, "conflict", err.Error())
	case errors.Is(err, dictation.ErrQueueFull):
		writeError(w, http.StatusTooManyRequests, "too_many_requests", err.Error())
	default:
		writeError(w, http.StatusBadGateway, "upstream", err.Error())
	}
}
