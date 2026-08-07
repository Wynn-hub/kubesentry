package console

import "github.com/Wynn-hub/kubesentry/internal/dbrepo"

// isUniqueViolation delegates to dbrepo.IsUniqueViolation. The implementation
// moved down to dbrepo so internal/console/service can share it: console root
// imports service (ogen server assembly in Task 7), so service importing
// console back for this helper would be an import cycle. dbrepo is the
// downstream dependency both packages already have.
//
// Kept as an unexported alias rather than rewriting the five call sites in the
// legacy handlers — those files are deleted wholesale in Task 9.
func isUniqueViolation(err error) bool { return dbrepo.IsUniqueViolation(err) }
