package dictation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/migueel15/dory/internal/apperr"
	"github.com/migueel15/dory/internal/i18n"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/migueel15/dory/internal/cmdexec"
)

var ErrModelMissing = apperr.New(apperr.ModelMissing, "download the dictation model before using it")

type localModel struct {
	ID, Name, Revision, SHA string
	Bytes                   int64
	Streaming               bool
}

var localCatalog = []localModel{
	{"parakeet-tdt-0.6b-v3", "Parakeet TDT 0.6B v3", "541d1f99c6b0c3cd0b11a95167540bb8edefd82b", "e3880d0aaaaf2c308ea2c35016b2b895c423eb3fda924c1b463d1c19b7f4d32e", 713975456, false},
	{"nemotron-3.5-asr-streaming-0.6b", "Nemotron Streaming 3.5 0.6B", "ea30d66debe3740a08b573244286791d423d6b3e", "3fc991d3badad7277c11030a7519832cddaf2057aafed6d4b25147e953a070b1", 742090464, true},
}

type ModelStatus struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Bytes     int64  `json:"bytes"`
	Streaming bool   `json:"streaming"`
	Status    string `json:"status"`
	Error     string `json:"error,omitempty"`
}

type ModelCatalog struct {
	Runtime bool          `json:"runtime"`
	Models  []ModelStatus `json:"models"`
}

type nemoWorker struct {
	ready, exited chan struct{}
	address       *url.URL
	token         string
	err           error
}

// LocalModels downloads pinned model artifacts and starts NeMo on loopback
// on first use. The processes belong to the lifetime of the Dory node.
type LocalModels struct {
	ctx context.Context
	dir string
	bin string

	engineMu  sync.Mutex
	mu        sync.Mutex
	downloads map[string]string
	workers   map[string]*nemoWorker
}

// NewLocalModels takes the NeMo binary configured for the node; when empty,
// the downloaded engine or the one on PATH is used.
func NewLocalModels(ctx context.Context, dir, bin string) *LocalModels {
	return &LocalModels{ctx: ctx, dir: dir, bin: bin, downloads: map[string]string{}, workers: map[string]*nemoWorker{}}
}

func findLocalModel(id string) (localModel, error) {
	for _, model := range localCatalog {
		if model.ID == id {
			return model, nil
		}
	}
	return localModel{}, ErrNotFound
}

func (m *LocalModels) path(model localModel) string {
	return filepath.Join(m.dir, model.ID+".q8_0.gguf")
}

func (m *LocalModels) installed(model localModel) bool {
	info, err := os.Stat(m.path(model))
	return err == nil && info.Mode().IsRegular() && info.Size() == model.Bytes
}

func (m *LocalModels) Catalog() ModelCatalog {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, supported := platformAsset()
	engine := m.binary() != ""
	out := ModelCatalog{Runtime: supported || engine, Models: make([]ModelStatus, 0, len(localCatalog))}
	for _, model := range localCatalog {
		status := ModelStatus{ID: model.ID, Name: model.Name, Bytes: model.Bytes, Streaming: model.Streaming, Status: "available"}
		if failure, ok := m.downloads[model.ID]; ok {
			status.Status, status.Error = "downloading", failure
			if failure != "" {
				status.Status = "failed"
			}
		} else if m.installed(model) && engine {
			status.Status = "installed"
		}
		out.Models = append(out.Models, status)
	}
	return out
}

func (m *LocalModels) Download(id string) error {
	model, err := findLocalModel(id)
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if failure, pending := m.downloads[id]; (pending && failure == "") || (m.installed(model) && m.binary() != "") {
		return nil
	}
	m.downloads[id] = ""
	go func() {
		err := m.ensureEngine()
		if err == nil && !m.installed(model) {
			err = m.download(model)
		}
		m.mu.Lock()
		defer m.mu.Unlock()
		if err != nil {
			m.downloads[id] = err.Error()
		} else {
			delete(m.downloads, id)
		}
	}()
	return nil
}

func (m *LocalModels) download(model localModel) error {
	if err := os.MkdirAll(m.dir, 0700); err != nil {
		return err
	}
	file, err := os.CreateTemp(m.dir, model.ID+"-*.part")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	defer file.Close()
	ctx, cancel := context.WithTimeout(m.ctx, 30*time.Minute)
	defer cancel()
	address := "https://huggingface.co/nvidia/" + model.ID + "/resolve/" + model.Revision + "/" + filepath.Base(m.path(model))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return errors.New(i18n.Text("dictation.no_pudo_descargar_modelo"))
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf(i18n.Text("dictation.hugging_face_respondio_http"), resp.StatusCode)
	}
	hash := sha256.New()
	size, err := io.Copy(io.MultiWriter(file, hash), io.LimitReader(resp.Body, model.Bytes+1))
	if err != nil {
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	if size != model.Bytes || hex.EncodeToString(hash.Sum(nil)) != model.SHA {
		return errors.New(i18n.Text("dictation.archivo_descargado_no_coincide"))
	}
	return os.Rename(file.Name(), m.path(model))
}

func (m *LocalModels) worker(ctx context.Context, model localModel) (*nemoWorker, error) {
	if !m.installed(model) {
		return nil, ErrModelMissing
	}
	bin := m.binary()
	if bin == "" {
		return nil, ErrModelMissing
	}
	m.mu.Lock()
	worker := m.workers[model.ID]
	if worker != nil {
		select {
		case <-worker.exited:
			worker = nil
		default:
		}
	}
	if worker == nil {
		worker = &nemoWorker{ready: make(chan struct{}), exited: make(chan struct{}), token: newID()}
		m.workers[model.ID] = worker
		go m.startWorker(bin, model, worker)
	}
	m.mu.Unlock()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-worker.ready:
		return worker, worker.err
	}
}

func workerArgs(modelPath, port, token string, streaming bool) []string {
	args := []string{"serve", "--host", "127.0.0.1", "--port", port,
		"--asr-model", modelPath, "--no-ui", "--api-key", token}
	if !streaming {
		args = append(args, "--max-upload-mb", "12")
	}
	return args
}

func (m *LocalModels) startWorker(bin string, model localModel, worker *nemoWorker) {
	defer close(worker.ready)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		worker.err = err
		close(worker.exited)
		return
	}
	port := fmt.Sprint(listener.Addr().(*net.TCPAddr).Port)
	listener.Close()
	worker.address = &url.URL{Scheme: "http", Host: "127.0.0.1:" + port}
	cmd := cmdexec.CommandContext(m.ctx, bin, workerArgs(m.path(model), port, worker.token, model.Streaming)...)
	cmd.Stderr = os.Stderr
	if err = cmd.Start(); err != nil {
		worker.err = err
		close(worker.exited)
		return
	}
	go func() { _ = cmd.Wait(); close(worker.exited) }()
	ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
	defer cancel()
	tick := time.NewTicker(150 * time.Millisecond)
	defer tick.Stop()
	for {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, worker.address.String()+"/v1/models", nil)
		req.Header.Set("Authorization", "Bearer "+worker.token)
		if res, err := http.DefaultClient.Do(req); err == nil {
			res.Body.Close()
			if res.StatusCode == http.StatusOK {
				return
			}
		}
		select {
		case <-worker.exited:
			worker.err = errors.New(i18n.Text("dictation.nemo_speech_termino_antes"))
			return
		case <-ctx.Done():
			_ = cmd.Process.Kill()
			worker.err = errors.New(i18n.Text("dictation.nemo_speech_tardo_demasiado"))
			return
		case <-tick.C:
		}
	}
}

func (m *LocalModels) Transcribe(w http.ResponseWriter, r *http.Request, id string, streaming bool) error {
	model, err := findLocalModel(id)
	if err != nil {
		return err
	}
	if model.Streaming != streaming {
		return ErrUnavailable
	}
	worker, err := m.worker(r.Context(), model)
	if err != nil {
		return err
	}
	endpoint := "/v1/audio/transcriptions"
	if streaming {
		endpoint += "/realtime"
	}
	proxy := httputil.ReverseProxy{Rewrite: func(req *httputil.ProxyRequest) {
		req.SetURL(worker.address)
		req.Out.URL.Path, req.Out.URL.RawPath, req.Out.URL.RawQuery = endpoint, "", ""
		req.Out.Header.Del("Cookie")
		req.Out.Header.Del("Origin")
		req.Out.Header.Set("Authorization", "Bearer "+worker.token)
	}}
	proxy.ServeHTTP(w, r)
	return nil
}
