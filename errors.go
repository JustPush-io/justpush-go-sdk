package justpush

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// Sentinel errors. Every error the client returns matches one of these with errors.Is,
// so callers can branch on the kind of failure without type assertions.
var (
	// ErrValidation: the message or topic is invalid, either caught before sending
	// (a *ValidationError) or rejected by the API with a 422 (an *APIError).
	ErrValidation = errors.New("justpush: validation failed")
	// ErrUnauthorized: 401, the token is missing, unknown or revoked.
	ErrUnauthorized = errors.New("justpush: unauthorized")
	// ErrForbidden: 403, the token may not do this, or the plan doesn't allow it.
	ErrForbidden = errors.New("justpush: forbidden")
	// ErrNotFound: 404, no message or topic with that key or UUID for this account.
	ErrNotFound = errors.New("justpush: not found")
	// ErrSubscriptionExpired: 410, the account's subscription has expired.
	ErrSubscriptionExpired = errors.New("justpush: subscription expired")
	// ErrRateLimited: 429, too many requests. See APIError.RetryAfter.
	ErrRateLimited = errors.New("justpush: rate limited")
	// ErrConnection: the API couldn't be reached, or didn't answer in time.
	ErrConnection = errors.New("justpush: connection failed")
)

// ValidationError is returned before any request is made when the SDK can tell on its own
// that the API would reject the message or topic (for example, more than 10 buttons).
type ValidationError struct {
	Message string
}

func (e *ValidationError) Error() string { return "justpush: " + e.Message }

// Is makes errors.Is(err, ErrValidation) true.
func (e *ValidationError) Is(target error) bool { return target == ErrValidation }

func invalid(format string, args ...any) error {
	return &ValidationError{Message: fmt.Sprintf(format, args...)}
}

// APIError is returned when the API answers with an error status.
type APIError struct {
	// StatusCode is the HTTP status, e.g. 401 or 422.
	StatusCode int
	// Message is the most readable message the API gave, or a generic one.
	Message string
	// Errors maps each field to its messages for a 422, when the API sends them.
	Errors map[string][]string
	// RetryAfter is set for a 429 when the API sends a Retry-After header.
	RetryAfter time.Duration
	// Body is the raw response body.
	Body []byte
}

func (e *APIError) Error() string {
	msg := fmt.Sprintf("justpush: %s (HTTP %d)", e.Message, e.StatusCode)
	if len(e.Errors) > 0 {
		fields := make([]string, 0, len(e.Errors))
		for field := range e.Errors {
			fields = append(fields, field)
		}
		sort.Strings(fields)
		msg += ": invalid " + strings.Join(fields, ", ")
	}
	return msg
}

// Unwrap returns the sentinel matching the status code, so errors.Is(err, ErrNotFound) works.
func (e *APIError) Unwrap() error {
	switch e.StatusCode {
	case 401:
		return ErrUnauthorized
	case 403:
		return ErrForbidden
	case 404:
		return ErrNotFound
	case 410:
		return ErrSubscriptionExpired
	case 422:
		return ErrValidation
	case 429:
		return ErrRateLimited
	}
	return nil
}

// ConnectionError wraps a network failure or timeout. It matches ErrConnection, and the
// underlying error too (so errors.Is(err, context.DeadlineExceeded) still works).
type ConnectionError struct {
	Err error
}

func (e *ConnectionError) Error() string {
	return "justpush: could not reach JustPush: " + e.Err.Error()
}

func (e *ConnectionError) Unwrap() []error { return []error{ErrConnection, e.Err} }
