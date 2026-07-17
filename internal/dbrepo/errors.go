package dbrepo

import "errors"

// ErrConflict is returned by *UpdateCAS methods when the row's updated_at no
// longer matches the value the caller read — someone else wrote to the row
// in between. Callers map this to an HTTP 409.
var ErrConflict = errors.New("dbrepo: row was modified since it was read")
