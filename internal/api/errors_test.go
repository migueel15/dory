package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/migueel15/dory/internal/apperr"
	"github.com/migueel15/dory/internal/data"
	"github.com/migueel15/dory/internal/mesh"
	"github.com/migueel15/dory/internal/runtime"
)

func errorResponse(t *testing.T, err error) (int, apperr.Body, string) {
	t.Helper()
	rec := httptest.NewRecorder()
	writeErr(rec, httptest.NewRequest(http.MethodGet, "/api/x", nil), err)
	var b apperr.Body
	if jerr := json.Unmarshal(rec.Body.Bytes(), &b); jerr != nil {
		t.Fatalf("body is not the error envelope: %v", jerr)
	}
	return rec.Code, b, rec.Body.String()
}

func TestWriteErrDomainErrorsKeepStatusCodeAndParams(t *testing.T) {
	for _, c := range []struct {
		err    error
		status int
		code   apperr.Code
	}{
		{data.ErrNotFound.Msgf("task %s", "t1"), 404, apperr.NotFound},
		{fmt.Errorf("ctx: %w", data.ErrInvalid.Msgf("title is required")), 400, apperr.Invalid},
		{runtime.ErrBusy, 409, apperr.Busy},
		{runtime.ErrRemote, 409, apperr.Remote},
		{runtime.ErrProvider.Msgf("nope"), 400, apperr.Invalid},
		{runtime.ErrDirtyWorktree, 409, apperr.DirtyWorktree},
		{mesh.ErrUnreachable, 502, apperr.Unreachable},
		{errNotCloned, 400, apperr.RepositoryNotClone},
		{ErrPlanConflict.Msgf("now at revision %d", 4).With("revision", 4), 409, apperr.RevisionConflict},
	} {
		status, b, _ := errorResponse(t, c.err)
		if status != c.status || b.Error.Code != c.code || b.Error.Message == "" {
			t.Errorf("%v: status %d code %q message %q", c.err, status, b.Error.Code, b.Error.Message)
		}
		if !errors.Is(c.err, c.err) {
			t.Errorf("%v does not match itself", c.err)
		}
	}
	_, b, _ := errorResponse(t, ErrPlanConflict.With("revision", 4))
	if b.Error.Params["revision"] != float64(4) {
		t.Fatalf("params = %v", b.Error.Params)
	}
}

func TestWriteErrNeverSendsTheTextOfAnUnknownError(t *testing.T) {
	status, b, raw := errorResponse(t, errors.New("git: fatal: not a repository: /home/miguel/secret"))
	if status != 500 || b.Error.Code != apperr.Internal || b.Error.Message != "internal error" {
		t.Fatalf("status %d body %+v", status, b)
	}
	if strings.Contains(raw, "secret") {
		t.Fatalf("leaked: %s", raw)
	}
	// A domain error that wraps a cause keeps the cause out of the response too.
	_, _, raw = errorResponse(t, apperr.Wrap(apperr.Conflict, errors.New("/home/miguel/secret"), "branch busy"))
	if strings.Contains(raw, "secret") {
		t.Fatalf("leaked cause: %s", raw)
	}
}

func TestDecodeRejectsBadJSONAsInvalid(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/x", strings.NewReader("{"))
	var v map[string]any
	if decode(rec, req, &v) {
		t.Fatal("decode accepted bad JSON")
	}
	var b apperr.Body
	json.Unmarshal(rec.Body.Bytes(), &b)
	if rec.Code != 400 || b.Error.Code != apperr.Invalid {
		t.Fatalf("status %d body %+v", rec.Code, b)
	}
}
