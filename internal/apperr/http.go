package apperr

import (
	"encoding/json"
	"log"
	"net/http"
)

var statuses = map[Code]int{
	NotFound:         http.StatusNotFound,
	Invalid:          http.StatusBadRequest,
	Conflict:         http.StatusConflict,
	Busy:             http.StatusConflict,
	Remote:           http.StatusConflict,
	Unreachable:      http.StatusBadGateway,
	Unauthorized:     http.StatusUnauthorized,
	Forbidden:        http.StatusForbidden,
	Unavailable:      http.StatusServiceUnavailable,
	RevisionConflict: http.StatusConflict,
	Internal:         http.StatusInternalServerError,

	Upstream:           http.StatusBadGateway,
	TooManyRequests:    http.StatusTooManyRequests,
	PayloadTooLarge:    http.StatusRequestEntityTooLarge,
	NodeOffline:        http.StatusServiceUnavailable,
	RepositoryNotClone: http.StatusBadRequest,
	UnknownWorktree:    http.StatusBadRequest,
	BranchSwitchFailed: http.StatusConflict,
	DirtyWorktree:      http.StatusConflict,
	SharedProcessGroup: http.StatusConflict,
	NoRequest:          http.StatusConflict,
	JobInputClosed:     http.StatusConflict,
	InvalidCode:        http.StatusBadRequest,
	InvalidAccount:     http.StatusBadRequest,
	InvalidIcon:        http.StatusBadRequest,
	InvalidRange:       http.StatusBadRequest,
	PlanNotAccepted:    http.StatusConflict,
	PlanConflict:       http.StatusConflict,
	NoRepository:       http.StatusConflict,
	NoThread:           http.StatusConflict,
	InProgress:         http.StatusConflict,
	ConfirmRequired:    http.StatusBadRequest,
	DeviceError:        http.StatusBadRequest,
	NoADB:              http.StatusFailedDependency,
	UpdateRefused:      http.StatusConflict,
	ModelMissing:       http.StatusConflict,
	NoChromium:         http.StatusServiceUnavailable,
	UserControl:        http.StatusConflict,
}

// HTTPStatus is the response status of a code; unknown codes are 500.
func HTTPStatus(c Code) int {
	if s, ok := statuses[c]; ok {
		return s
	}
	return http.StatusInternalServerError
}

// ForStatus is the code of an HTTP status, for the transports that decide the
// status first (the auth and mesh endpoints).
func ForStatus(status int) Code {
	for code, s := range map[Code]int{Invalid: 400, Unauthorized: 401, Forbidden: 403, NotFound: 404, Conflict: 409, TooManyRequests: 429} {
		if s == status {
			return code
		}
	}
	return Internal
}

// Body is the JSON envelope of an error response.
type Body struct {
	Error struct {
		Code    Code           `json:"code"`
		Message string         `json:"message"`
		Params  map[string]any `json:"params,omitempty"`
	} `json:"error"`
}

// WriteHTTP answers with the status and envelope of err. A domain error sends
// its own message; anything else is an internal error whose text only goes to
// the log, because it may carry command output or paths.
// r may be nil.
func WriteHTTP(w http.ResponseWriter, r *http.Request, err error) {
	var b Body
	e := As(err)
	switch {
	case e == nil:
		b.Error.Code, b.Error.Message = Internal, "internal error"
	default:
		b.Error.Code, b.Error.Message, b.Error.Params = e.Code, e.Message, e.Params
	}
	status := HTTPStatus(b.Error.Code)
	if status >= 500 {
		if r != nil {
			log.Printf("api: %s %s: %v", r.Method, r.URL.Path, err)
		} else {
			log.Printf("api: %v", err)
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(b); err != nil {
		log.Printf("api: encode error response: %v", err)
	}
}

// Format is the text an MCP tool error carries: "<code>: <message>" and the
// parameters as JSON when there are any. Unlike HTTP, the agent gets the text
// of a plain error too, because the agent has to act on it.
func Format(err error) string {
	e := As(err)
	if e == nil {
		return string(Internal) + ": " + err.Error()
	}
	out := string(e.Code) + ": " + e.Message
	if len(e.Params) > 0 {
		if raw, err := json.Marshal(e.Params); err == nil {
			out += " " + string(raw)
		}
	}
	return out
}
