package dictation

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
)

// Whisper calls the /inference endpoint of a local whisper.cpp whisper-server.
type Whisper struct {
	URL      string
	Language string
	Client   *http.Client
}

func (w Whisper) Transcribe(ctx context.Context, wav []byte) (string, error) {
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	part, err := mw.CreateFormFile("file", "chunk.wav")
	if err != nil {
		return "", err
	}
	if _, err = part.Write(wav); err != nil {
		return "", err
	}
	lang := w.Language
	if lang == "" {
		lang = "es"
	}
	for k, v := range map[string]string{"response_format": "json", "language": lang, "temperature": "0"} {
		if err = mw.WriteField(k, v); err != nil {
			return "", err
		}
	}
	if err = mw.Close(); err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(w.URL, "/")+"/inference", &body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	client := w.Client
	if client == nil {
		client = http.DefaultClient
	}
	res, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return "", err
	}
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("whisper-server: %s: %s", res.Status, strings.TrimSpace(string(raw)))
	}
	var out struct {
		Text  string `json:"text"`
		Error string `json:"error"`
	}
	if err = json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("whisper-server: %w", err)
	}
	if out.Error != "" {
		return "", fmt.Errorf("whisper-server: %s", out.Error)
	}
	return strings.TrimSpace(out.Text), nil
}
