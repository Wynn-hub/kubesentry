package service

import (
	"errors"
	"strings"
)

// ErrNotFound marks a missing resource; the handler maps it to the 404 variant.
var ErrNotFound = errors.New("not found")

// ErrGone marks a rollback target whose snapshot has been pruned; maps to 410.
var ErrGone = errors.New("gone")

// ValidationError is a client-input problem (400). The message is shown to the user.
type ValidationError struct{ Msg string }

func (e *ValidationError) Error() string { return e.Msg }

// ReferencedByError means the delete was blocked because groups still
// reference the policy (409, with data.referencedBy listing them).
type ReferencedByError struct{ Groups []string }

func (e *ReferencedByError) Error() string {
	return "policy is referenced by policy groups: " + strings.Join(e.Groups, ", ")
}
