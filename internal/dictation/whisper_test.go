package dictation

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWhisperClient(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/inference" || r.FormValue("language") != "es" || r.FormValue("prompt") != "antes" {
			http.Error(w, "bad", http.StatusBadRequest)
			return
		}
		if _, _, err := r.FormFile("file"); err != nil {
			http.Error(w, "no file", http.StatusBadRequest)
			return
		}
		w.Write([]byte(`{"text":" hola mundo\n"}`))
	}))
	defer srv.Close()
	got, err := Whisper{URL: srv.URL}.Transcribe(context.Background(), []byte("RIFF"), "antes")
	if err != nil || got != "hola mundo" {
		t.Fatalf("got %q %v", got, err)
	}
	if _, err := (Whisper{URL: srv.URL + "/x"}).Transcribe(context.Background(), nil, ""); err == nil {
		t.Fatal("want error on non-200")
	}
}
