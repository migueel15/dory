// Package apperr is the one error model of Dory: a stable code, a technical
// message in English and optional parameters. The transports translate it in
// one place each (WriteHTTP for the API, Format for MCP); the texts the user
// reads are chosen by the web from the code.
package apperr

import (
	"errors"
	"fmt"
	"maps"
)

// Code is the stable identifier of a kind of failure. Clients branch on it.
type Code string

const (
	NotFound         Code = "not_found"
	Invalid          Code = "invalid_argument"
	Conflict         Code = "conflict"
	Busy             Code = "thread_busy"
	Remote           Code = "remote"
	Unreachable      Code = "unreachable"
	Unauthorized     Code = "unauthorized"
	Forbidden        Code = "forbidden"
	Unavailable      Code = "unavailable"
	RevisionConflict Code = "revision_conflict"
	Internal         Code = "internal"
)

// Specific codes: each one names a failure the web explains with its own text.
const (
	Upstream           Code = "upstream"
	TooManyRequests    Code = "too_many_requests"
	PayloadTooLarge    Code = "payload_too_large"
	NodeOffline        Code = "node_offline"
	RepositoryNotClone Code = "repository_not_cloned"
	UnknownWorktree    Code = "unknown_worktree"
	BranchSwitchFailed Code = "branch_switch_failed"
	DirtyWorktree      Code = "dirty_worktree"
	SharedProcessGroup Code = "shared_process_group"
	NoRequest          Code = "no_request"
	JobInputClosed     Code = "job_input_closed"
	InvalidCode        Code = "invalid_code"
	InvalidAccount     Code = "invalid_account"
	InvalidIcon        Code = "invalid_icon"
	InvalidRange       Code = "invalid_range"
	PlanNotAccepted    Code = "plan_not_accepted"
	PlanConflict       Code = "plan_conflict"
	NoRepository       Code = "no_repository"
	NoThread           Code = "no_thread"
	InProgress         Code = "in_progress"
	ConfirmRequired    Code = "confirm_required"
	DeviceError        Code = "device_error"
	NoADB              Code = "no_adb"
	UpdateRefused      Code = "update_refused"
	ModelMissing       Code = "model_missing"
	NoChromium         Code = "no_chromium"
	UserControl        Code = "user_control"
)

// Error is a domain error. Message is technical and in English; Params carry
// the values a client needs to build its own text. Err is the cause, kept for
// logs and errors.Is, and never sent to a client.
type Error struct {
	Code    Code
	Message string
	Params  map[string]any
	Err     error
}

func (e *Error) Error() string {
	switch {
	case e.Err == nil:
		return e.Message
	case e.Message == "":
		return e.Err.Error()
	}
	return e.Message + ": " + e.Err.Error()
}

func (e *Error) Unwrap() error { return e.Err }

// Is matches an *Error with the same code and, when the target names a message,
// the same message. That keeps errors.Is working on copies made by With.
func (e *Error) Is(target error) bool {
	t, ok := target.(*Error)
	if !ok {
		return false
	}
	return e.Code == t.Code && (t.Message == "" || e.Message == t.Message)
}

func New(code Code, msg string) *Error { return &Error{Code: code, Message: msg} }

func Newf(code Code, format string, a ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, a...)}
}

// Wrap keeps err as the cause of a domain error with the given message.
func Wrap(code Code, err error, msg string) *Error {
	return &Error{Code: code, Message: msg, Err: err}
}

// With returns a copy that also carries the parameter.
func (e *Error) With(key string, v any) *Error {
	c := *e
	c.Params = make(map[string]any, len(e.Params)+1)
	maps.Copy(c.Params, e.Params)
	c.Params[key] = v
	return &c
}

// Msg returns a copy with another message, for a sentinel that needs context.
func (e *Error) Msg(msg string) *Error {
	c := *e
	c.Message = msg
	return &c
}

// CodeOf is the code of the first domain error in the chain, or Internal.
func CodeOf(err error) Code {
	if e := As(err); e != nil {
		return e.Code
	}
	return Internal
}

// As returns the first domain error in the chain, or nil.
func As(err error) *Error {
	var e *Error
	if errors.As(err, &e) {
		return e
	}
	return nil
}
