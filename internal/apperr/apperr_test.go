package apperr

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestIsMatchesCodeAndMessage(t *testing.T) {
	busy := New(Busy, "wait")
	copyOf := busy.With("thread", "t1")
	if !errors.Is(copyOf, busy) {
		t.Fatal("a copy with params must match its sentinel")
	}
	if m := busy.Msgf("wait %d", 2); !errors.Is(m, busy) || m.Message != "wait: wait 2" {
		t.Fatalf("Msgf copy must match its sentinel and extend its message: %q", m.Message)
	}
	if errors.Is(New(Busy, "other"), busy) {
		t.Fatal("different message must not match a sentinel with message")
	}
	if !errors.Is(New(Busy, "other"), New(Busy, "")) {
		t.Fatal("an empty target message matches any message of the code")
	}
	if errors.Is(New(NotFound, "wait"), busy) {
		t.Fatal("different code must not match")
	}
	if errors.Is(errors.New("x"), busy) {
		t.Fatal("plain errors never match")
	}
}

func TestWrapUnwrapAndCodeOf(t *testing.T) {
	cause := errors.New("git: exit 128")
	err := fmt.Errorf("context: %w", Wrap(Conflict, cause, "branch switch failed"))
	if !errors.Is(err, cause) {
		t.Fatal("Unwrap must reach the cause")
	}
	if CodeOf(err) != Conflict {
		t.Fatalf("code = %q", CodeOf(err))
	}
	if CodeOf(cause) != Internal {
		t.Fatal("a plain error is internal")
	}
	if got := Wrap(Conflict, cause, "m").Error(); got != "m: git: exit 128" {
		t.Fatalf("Error() = %q", got)
	}
}

func TestWithDoesNotMutate(t *testing.T) {
	base := New(Invalid, "bad").With("a", 1)
	next := base.With("b", 2)
	if len(base.Params) != 1 || len(next.Params) != 2 {
		t.Fatalf("params leaked: %v %v", base.Params, next.Params)
	}
}

func TestHTTPStatus(t *testing.T) {
	if HTTPStatus(NotFound) != 404 || HTTPStatus(Busy) != 409 || HTTPStatus("nope") != 500 {
		t.Fatal("unexpected statuses")
	}
}

func TestWriteHTTPDomainError(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/x", nil)
	WriteHTTP(rec, req, fmt.Errorf("ctx: %w", Wrap(NotFound, errors.New("secret path /home/x"), "thread not found").With("id", "t1")))
	if rec.Code != 404 {
		t.Fatalf("status = %d", rec.Code)
	}
	var b Body
	if err := json.Unmarshal(rec.Body.Bytes(), &b); err != nil {
		t.Fatal(err)
	}
	if b.Error.Code != NotFound || b.Error.Message != "thread not found" || b.Error.Params["id"] != "t1" {
		t.Fatalf("body = %+v", b)
	}
	if strings.Contains(rec.Body.String(), "secret") {
		t.Fatal("the cause must not reach the client")
	}
}

func TestWriteHTTPUnknownErrorIsGeneric(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/x", nil), errors.New("git: fatal: /home/miguel/repo"))
	if rec.Code != 500 {
		t.Fatalf("status = %d", rec.Code)
	}
	var b Body
	json.Unmarshal(rec.Body.Bytes(), &b)
	if b.Error.Code != Internal || b.Error.Message != "internal error" || strings.Contains(rec.Body.String(), "miguel") {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestFormat(t *testing.T) {
	if got := Format(New(Invalid, "needs text").With("field", "text")); got != `invalid_argument: needs text {"field":"text"}` {
		t.Fatalf("Format = %q", got)
	}
	if got := Format(errors.New("boom")); got != "internal: boom" {
		t.Fatalf("Format = %q", got)
	}
}
