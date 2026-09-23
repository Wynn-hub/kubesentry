package service

import (
	"errors"
	"fmt"
	"strings"

	dbrepo "github.com/Wynn-hub/kubesentry/internal/database/repository"
)

// ErrNotFound marks a missing resource; the handler maps it to the 404 variant.
var ErrNotFound = errors.New("not found")

// ErrGone marks a rollback target whose snapshot has been pruned; maps to 410.
var ErrGone = errors.New("gone")

// ErrUnavailable marks an optional repository that is not configured.
var ErrUnavailable = errors.New("service unavailable")

// ValidationError is a client-input problem (400). The message is shown to the user.
type ValidationError struct{ Msg string }

func (e *ValidationError) Error() string { return e.Msg }

// ReferencedByError means the delete was blocked because groups still
// reference the policy (409, with data.referencedBy listing them).
type ReferencedByError struct{ Groups []string }

func (e *ReferencedByError) Error() string {
	return "policy is referenced by policy groups: " + strings.Join(e.Groups, ", ")
}

// conflictError carries a caller-facing message while still matching
// errors.Is(err, dbrepo.ErrConflict) — the handler layer maps on the sentinel,
// the user reads the message. Wrapping the sentinel with %w instead would
// append its internal text ("dbrepo: row was modified since it was read") to
// every conflict message the API returns.
type conflictError struct{ msg string }

func (e *conflictError) Error() string        { return e.msg }
func (e *conflictError) Is(target error) bool { return target == dbrepo.ErrConflict }
func (e *conflictError) Unwrap() error        { return dbrepo.ErrConflict }

func newConflict(format string, args ...any) error {
	return &conflictError{msg: fmt.Sprintf(format, args...)}
}

// IsConflict keeps storage sentinels behind the service boundary.
func IsConflict(err error) bool { return errors.Is(err, dbrepo.ErrConflict) }

// IsUniqueViolation keeps database-specific uniqueness errors behind the service boundary.
func IsUniqueViolation(err error) bool { return dbrepo.IsUniqueViolation(err) }
