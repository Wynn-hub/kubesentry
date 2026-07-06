package console

import (
	"encoding/json"

	"github.com/Wynn-hub/kubesentry/internal/api/v1alpha1"
)

// logicalCursor is persisted as JSON in the kubesentry.io/logical-cursor
// annotation. Cursor is the original-timeline version whose content is now
// live; AtVersion is status.currentVersion at the moment the rollback was
// requested; Head is the original newest version before the cursor first
// left the head.
type logicalCursor struct {
	Cursor    int64 `json:"cursor"`
	AtVersion int64 `json:"atVersion"`
	Head      int64 `json:"head"`
}

// resolveCursor computes the logical position of a policy on its version
// timeline. Rollbacks are roll-forward (a new snapshot is created), so
// currentVersion alone cannot express "where we are" — see the spec §3.4.
func resolveCursor(p *v1alpha1.Policy) (cursor, head int64, inFlight bool) {
	cur := p.Status.CurrentVersion
	raw := ""
	if p.Annotations != nil {
		raw = p.Annotations[cursorAnnotation]
	}
	if raw == "" {
		return cur, cur, false
	}
	var c logicalCursor
	if err := json.Unmarshal([]byte(raw), &c); err != nil {
		return cur, cur, false
	}
	switch cur {
	case c.AtVersion:
		return c.Cursor, c.Head, true // operator has not settled the rollback yet
	case c.AtVersion + 1:
		return c.Cursor, c.Head, false // rollback settled
	default:
		return cur, cur, false // spec changed outside console — cursor is stale
	}
}
