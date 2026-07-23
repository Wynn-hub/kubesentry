// internal/console/handlers_exception.go
package console

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"

	"github.com/Wynn-hub/kubesentry/internal/api/v1alpha1"
	"github.com/Wynn-hub/kubesentry/internal/dbrepo"
)

type exceptionListItem struct {
	Name          string     `json:"name"`
	Phase         string     `json:"phase"`
	Reason        string     `json:"reason"`
	Duration      string     `json:"duration"`
	TargetSummary string     `json:"targetSummary"`
	ExpiresAt     *time.Time `json:"expiresAt,omitempty"`
}

// exceptionStatus mirrors the CRD-era status subresource — computed at read
// time from ExpiresAt/CreatedAt, never stored, same rationale as groupStatus.
type exceptionStatus struct {
	Phase       string     `json:"phase"`
	EffectiveAt time.Time  `json:"effectiveAt"`
	ExpiresAt   *time.Time `json:"expiresAt,omitempty"`
}

type exceptionDetail struct {
	Name            string                       `json:"name"`
	ResourceVersion string                       `json:"resourceVersion"`
	Spec            v1alpha1.PolicyExceptionSpec `json:"spec"`
	Status          exceptionStatus              `json:"status"`
	Scope           clusterScope                 `json:"scope"`
}

// exceptionPhase derives Active/Expired from ExpiresAt at read time — the
// GORM-era equivalent of the old ExceptionReconciler's async Status.Phase
// computation, which no longer runs against console-created rows.
func exceptionPhase(expiresAt *time.Time) string {
	if expiresAt != nil && expiresAt.Before(time.Now()) {
		return v1alpha1.PhaseExpired
	}
	return v1alpha1.PhaseActive
}

func exceptionResourceVersion(e *dbrepo.PolicyException) string {
	return strconv.FormatInt(e.UpdatedAt.UnixNano(), 10)
}

func toExceptionDetail(e *dbrepo.PolicyException) exceptionDetail {
	return exceptionDetail{
		Name:            e.Name,
		ResourceVersion: exceptionResourceVersion(e),
		Spec:            e.Spec.Data(),
		Status: exceptionStatus{
			Phase:       exceptionPhase(e.ExpiresAt),
			EffectiveAt: e.CreatedAt,
			ExpiresAt:   e.ExpiresAt,
		},
	}
}

func targetSummary(spec *v1alpha1.PolicyExceptionSpec) string {
	switch {
	case spec.AllPolicies:
		return "all policies"
	case len(spec.PolicyRefs) > 0:
		return "policies: " + joinMax(spec.PolicyRefs, 3)
	case len(spec.PolicyGroupRefs) > 0:
		return "groups: " + joinMax(spec.PolicyGroupRefs, 3)
	}
	return ""
}

func joinMax(items []string, max int) string {
	if len(items) <= max {
		return strings.Join(items, ", ")
	}
	return strings.Join(items[:max], ", ") + fmt.Sprintf(" (+%d more)", len(items)-max)
}

func (h *Handlers) listExceptions(w http.ResponseWriter, r *http.Request) {
	query, err := parsePageQuery(r.URL.Query(), pageSpec{
		DefaultSortBy:    "name",
		DefaultSortOrder: "asc",
		AllowedSort:      stringSet("name", "phase", "targetSummary", "duration", "expiresAt"),
	})
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	exceptions, err := h.Repos.Exceptions.List(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "list exceptions: "+err.Error())
		return
	}
	items := make([]exceptionListItem, 0, len(exceptions))
	for i := range exceptions {
		e := &exceptions[i]
		spec := e.Spec.Data()
		items = append(items, exceptionListItem{
			Name:          e.Name,
			Phase:         exceptionPhase(e.ExpiresAt),
			Reason:        spec.Reason,
			Duration:      spec.Duration,
			TargetSummary: targetSummary(&spec),
			ExpiresAt:     e.ExpiresAt,
		})
	}
	sortPageItems(items, query, func(a, b exceptionListItem) int {
		switch query.SortBy {
		case "phase":
			return cmp.Compare(a.Phase, b.Phase)
		case "targetSummary":
			return cmp.Compare(a.TargetSummary, b.TargetSummary)
		case "duration":
			aDuration, aErr := time.ParseDuration(a.Duration)
			bDuration, bErr := time.ParseDuration(b.Duration)
			if aErr == nil && bErr == nil {
				return cmp.Compare(aDuration, bDuration)
			}
			return cmp.Compare(a.Duration, b.Duration)
		case "expiresAt":
			return compareOptionalTime(a.ExpiresAt, b.ExpiresAt)
		default:
			return cmp.Compare(a.Name, b.Name)
		}
	}, func(a, b exceptionListItem) int {
		return cmp.Compare(a.Name, b.Name)
	})
	writeOK(w, paginate(items, query))
}

func compareOptionalTime(a, b *time.Time) int {
	switch {
	case a == nil && b == nil:
		return 0
	case a == nil:
		return -1
	case b == nil:
		return 1
	default:
		return a.Compare(*b)
	}
}

func (h *Handlers) getException(w http.ResponseWriter, r *http.Request) {
	e, ok := h.fetchException(w, r)
	if !ok {
		return
	}
	detail := toExceptionDetail(e)
	scope, err := h.getScopeBinding(r.Context(), scopeOwnerException, e.Name)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "get exception scope: "+err.Error())
		return
	}
	detail.Scope = scope
	writeOK(w, detail)
}

// fetchException loads the {name} path exception, writing 404/500 on failure.
func (h *Handlers) fetchException(w http.ResponseWriter, r *http.Request) (*dbrepo.PolicyException, bool) {
	name := r.PathValue("name")
	e, err := h.Repos.Exceptions.GetByName(r.Context(), name)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeErr(w, http.StatusNotFound, "exception "+name+" not found")
		} else {
			writeErr(w, http.StatusInternalServerError, "get exception: "+err.Error())
		}
		return nil, false
	}
	return e, true
}

func exceptionSpecFrom(req *exceptionRequest) v1alpha1.PolicyExceptionSpec {
	return v1alpha1.PolicyExceptionSpec{
		PolicyRefs:        req.PolicyRefs,
		PolicyGroupRefs:   req.PolicyGroupRefs,
		AllPolicies:       req.AllPolicies,
		Match:             req.Match,
		Duration:          req.Duration,
		RetainAfterExpiry: req.RetainAfterExpiry,
		Reason:            req.Reason,
	}
}

// exceptionExpiresAt parses spec.Duration (already validated as a positive
// Go duration by validateExceptionRequest) and returns the absolute
// expiration timestamp to persist — this replaces the old async
// ExceptionReconciler's Status.ExpiresAt computation with a synchronous,
// create/update-time calculation per the DB schema (no separate Status
// subresource to reconcile into).
func exceptionExpiresAt(duration string) (time.Time, error) {
	d, err := time.ParseDuration(duration)
	if err != nil || d <= 0 {
		return time.Time{}, fmt.Errorf("duration must be a positive Go duration string")
	}
	return time.Now().Add(d), nil
}

func (h *Handlers) createException(w http.ResponseWriter, r *http.Request) {
	var req exceptionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if err := validateExceptionRequest(&req, true); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	expiresAt, err := exceptionExpiresAt(req.Duration)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	e := &dbrepo.PolicyException{
		Name:      req.Name,
		Spec:      datatypes.NewJSONType(exceptionSpecFrom(&req)),
		ExpiresAt: &expiresAt,
	}
	if err := h.validateScopeTargets(r.Context(), req.AllClusters, req.ClusterNames); err != nil {
		writeErr(w, http.StatusBadRequest, "set exception scope: "+err.Error())
		return
	}
	if err := h.Repos.Exceptions.WithTx(r.Context(), func(exceptions *dbrepo.ExceptionRepo, distributions *dbrepo.DistributionRepo) error {
		if err := exceptions.Create(r.Context(), e); err != nil {
			return err
		}
		return syncScopeBinding(r.Context(), distributions, scopeOwnerException, e.Name, req.AllClusters, req.ClusterNames)
	}); err != nil {
		if isUniqueViolation(err) {
			writeErr(w, http.StatusConflict, "exception "+req.Name+" already exists")
		} else {
			writeErr(w, http.StatusInternalServerError, "create exception: "+err.Error())
		}
		return
	}
	h.notifyChange(r, "exception")
	writeOK(w, map[string]string{"name": e.Name})
}

func (h *Handlers) updateException(w http.ResponseWriter, r *http.Request) {
	var req exceptionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if err := validateExceptionRequest(&req, false); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	e, ok := h.fetchException(w, r)
	if !ok {
		return
	}
	if req.ResourceVersion != "" && req.ResourceVersion != exceptionResourceVersion(e) {
		writeErr(w, http.StatusConflict, "exception has been modified, refresh and retry")
		return
	}
	if err := h.validateScopeTargets(r.Context(), req.AllClusters, req.ClusterNames); err != nil {
		writeErr(w, http.StatusBadRequest, "set exception scope: "+err.Error())
		return
	}
	expiresAt, err := exceptionExpiresAt(req.Duration)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	// expectedUpdatedAt anchors the compare-and-swap to the exact row state
	// fetchException just read, closing the check-then-act race.
	expectedUpdatedAt := e.UpdatedAt
	e.Spec = datatypes.NewJSONType(exceptionSpecFrom(&req))
	e.ExpiresAt = &expiresAt
	fields := map[string]any{
		"spec":       e.Spec,
		"expires_at": e.ExpiresAt,
	}
	if err := h.Repos.Exceptions.WithTx(r.Context(), func(exceptions *dbrepo.ExceptionRepo, distributions *dbrepo.DistributionRepo) error {
		if err := exceptions.UpdateCAS(r.Context(), e.ID, expectedUpdatedAt, fields); err != nil {
			return err
		}
		return syncScopeBinding(r.Context(), distributions, scopeOwnerException, e.Name, req.AllClusters, req.ClusterNames)
	}); err != nil {
		if errors.Is(err, dbrepo.ErrConflict) {
			writeErr(w, http.StatusConflict, "exception has been modified, refresh and retry")
			return
		}
		writeErr(w, http.StatusInternalServerError, "update exception: "+err.Error())
		return
	}
	h.notifyChange(r, "exception")
	writeOK(w, map[string]string{"name": e.Name})
}

func (h *Handlers) deleteException(w http.ResponseWriter, r *http.Request) {
	e, ok := h.fetchException(w, r)
	if !ok {
		return
	}
	if err := h.Repos.Exceptions.WithTx(r.Context(), func(exceptions *dbrepo.ExceptionRepo, distributions *dbrepo.DistributionRepo) error {
		if err := exceptions.Delete(r.Context(), e.ID); err != nil {
			return err
		}
		return deleteScopeBinding(r.Context(), distributions, scopeOwnerException, e.Name)
	}); err != nil {
		writeErr(w, http.StatusInternalServerError, "delete exception: "+err.Error())
		return
	}
	h.notifyChange(r, "exception")
	writeOK(w, nil)
}
