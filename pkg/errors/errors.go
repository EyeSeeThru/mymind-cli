package errors

import (
	"fmt"
	"net/url"
	"os"
)

// Exit codes
const (
	ExitOK         = 0
	ExitUser       = 1
	ExitAuth       = 2
	ExitServer     = 3
	ExitNetwork    = 4
	ExitRateLimit  = 5
)

// ExhaustedPolicy is a rate-limit policy that has been exhausted.
type ExhaustedPolicy struct {
	Name string `json:"name"`
	T    int    `json:"t"`
}

// ApiError is a structured error from the MyMind API.
type ApiError struct {
	Type              string
	Status            int
	Detail            string
	RequestID         string
	RetryAfter        *int
	ExhaustedPolicies []ExhaustedPolicy
}

func (e *ApiError) Error() string {
	return fmt.Sprintf("[%d %s] %s", e.Status, e.Type, e.Detail)
}

// UserError is a CLI usage error (bad args, missing inputs, etc.)
type UserError struct {
	Message string
}

func (e *UserError) Error() string {
	return e.Message
}

// AuthError is a credentials / authentication error.
type AuthError struct {
	Message string
}

func (e *AuthError) Error() string {
	return e.Message
}

// ErrNetwork is a network-level error (DNS, connection refused, timeout).
type ErrNetwork struct {
	Err error
}

func (e *ErrNetwork) Error() string {
	return e.Err.Error()
}

func (e *ErrNetwork) Unwrap() error { return e.Err }

// MapExitCode maps any error to the appropriate CLI exit code.
func MapExitCode(err error) int {
	switch err.(type) {
	case *AuthError:
		return ExitAuth
	case *ApiError:
		ae := err.(*ApiError)
		if ae.Status == 401 || ae.Status == 403 {
			return ExitAuth
		}
		if ae.Status == 429 {
			return ExitRateLimit
		}
		if ae.Status >= 500 {
			return ExitServer
		}
		return ExitUser
	case *UserError:
		return ExitUser
	case *url.Error:
		return ExitNetwork
	}
	if os.IsNotExist(err) || os.IsTimeout(err) {
		return ExitNetwork
	}
	return ExitUser
}

// ToJSON returns a JSON-serializable map for --json error output.
func ToJSON(err error) map[string]interface{} {
	out := map[string]interface{}{}

	switch e := err.(type) {
	case *ApiError:
		m := map[string]interface{}{
			"type":   e.Type,
			"status": e.Status,
			"detail": e.Detail,
		}
		if e.RequestID != "" {
			m["requestId"] = e.RequestID
		}
		if e.RetryAfter != nil {
			m["retryAfter"] = *e.RetryAfter
		}
		if len(e.ExhaustedPolicies) > 0 {
			pols := make([]map[string]interface{}, len(e.ExhaustedPolicies))
			for i, p := range e.ExhaustedPolicies {
				pols[i] = map[string]interface{}{"name": p.Name, "t": p.T}
			}
			m["exhaustedPolicies"] = pols
		}
		out["error"] = m
	case *AuthError:
		out["error"] = map[string]interface{}{
			"type":   "AuthRequired",
			"status": 0,
			"detail": e.Message,
		}
	case *UserError:
		out["error"] = map[string]interface{}{
			"type":   "UserError",
			"status": 0,
			"detail": e.Message,
		}
	default:
		out["error"] = map[string]interface{}{
			"type":   "Error",
			"status": 0,
			"detail": e.Error(),
		}
	}
	return out
}
