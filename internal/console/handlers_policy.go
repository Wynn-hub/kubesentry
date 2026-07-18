package console

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/Wynn-hub/kubesentry/internal/api/v1alpha1"
	"github.com/Wynn-hub/kubesentry/internal/dbrepo"
	"github.com/Wynn-hub/kubesentry/internal/groupresolve"
	"github.com/Wynn-hub/kubesentry/internal/webhook"
)

const (
	// cursorAnnotation is kept only so cursor.go / cursor_test.go (out of
	// scope for this task, still exercised as a pure function over
	// v1alpha1.Policy) keep compiling. The GORM-backed handlers below no
	// longer read or write it via this constant: rollback here is
	// roll-forward via a freshly appended PolicyVersion row rather than an
	// async in-flight marker a controller has to settle — see
	// rollbackPolicy. dbrepo.Policy.Annotations (added alongside Labels in
	// migration 0003) is a plain user-editable annotations bag now, not a
	// vehicle for this internal bookkeeping key.
	cursorAnnotation = "kubesentry.io/logical-cursor"
	// visualBuilderSpecAnnotation stores the visual policy builder's editor
	// state as JSON. Cleared by sending an empty string for this key — see
	// applyAnnotations.
	visualBuilderSpecAnnotation = "kubesentry.io/visual-builder-spec"
)

type policyListItem struct {
	Name            string    `json:"name"`
	EnforcementMode string    `json:"enforcementMode"`
	Phase           string    `json:"phase"`
	Description     string    `json:"description"`
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
}

type policyDetail struct {
	Name            string               `json:"name"`
	Description     string               `json:"description"`
	Labels          map[string]string    `json:"labels"`
	Annotations     map[string]string    `json:"annotations"`
	EnforcementMode string               `json:"enforcementMode"`
	Match           v1alpha1.PolicyMatch `json:"match"`
	Rego            string               `json:"rego"`
	Phase           string               `json:"phase"`
	CompileError    string               `json:"compileError,omitempty"`
	ResourceVersion string               `json:"resourceVersion"`
	CreatedAt       time.Time            `json:"createdAt"`
	UpdatedAt       time.Time            `json:"updatedAt"`
}

// applyAnnotations merges src into dst (lazy-initializing dst), treating
// empty-string values as "do not set" — this is how the visual builder
// clears its annotation on save (see visualBuilderSpecAnnotation).
func applyAnnotations(dst map[string]string, src map[string]string) map[string]string {
	for k, v := range src {
		if v == "" {
			continue
		}
		if dst == nil {
			dst = map[string]string{}
		}
		dst[k] = v
	}
	return dst
}

type resourceSuggestionsResponse struct {
	APIGroups        []string            `json:"apiGroups"`
	APIVersions      []string            `json:"apiVersions"`
	Resources        []string            `json:"resources"`
	ResourcesByGroup map[string][]string `json:"resourcesByGroup"`
}

// multiValue splits a comma-separated query parameter into trimmed non-empty values.
func multiValue(raw string) []string {
	if raw == "" {
		return nil
	}
	vals := make([]string, 0, 2)
	for _, v := range strings.Split(raw, ",") {
		if v = strings.TrimSpace(v); v != "" {
			vals = append(vals, v)
		}
	}
	return vals
}

// matchAny reports whether v matches the filter: an empty filter matches everything.
func matchAny(filter []string, v string) bool {
	if len(filter) == 0 {
		return true
	}
	for _, f := range filter {
		if f == v {
			return true
		}
	}
	return false
}

// policyPhase derives Ready/Invalid from CompileOK at read time. There is no
// async controller re-validating rows anymore (validatePolicyRequest already
// runs webhook.CompileRego synchronously before any write reaches the DB —
// see createPolicy/updatePolicy), so in practice every row a console client
// can see has CompileOK=true; the field is kept for API-shape continuity and
// in case a future non-console writer ever persists a failed compile.
func policyPhase(compileOK bool) string {
	if compileOK {
		return v1alpha1.PhaseReady
	}
	return v1alpha1.PhaseInvalid
}

// policyResourceVersion derives an opaque optimistic-concurrency token from
// the row's UpdatedAt, mirroring the CRD-era resourceVersion contract
// without needing a dedicated column.
func policyResourceVersion(p *dbrepo.Policy) string {
	return strconv.FormatInt(p.UpdatedAt.UnixNano(), 10)
}

func toPolicyDetail(p *dbrepo.Policy) policyDetail {
	return policyDetail{
		Name:            p.Name,
		Description:     p.Description,
		Labels:          p.Labels.Data(),
		Annotations:     p.Annotations.Data(),
		EnforcementMode: p.EnforcementMode,
		Match:           p.Match.Data(),
		Rego:            p.Rego,
		Phase:           policyPhase(p.CompileOK),
		CompileError:    p.CompileError,
		ResourceVersion: policyResourceVersion(p),
		CreatedAt:       p.CreatedAt,
		UpdatedAt:       p.UpdatedAt,
	}
}

func (h *Handlers) listPolicies(w http.ResponseWriter, r *http.Request) {
	policies, err := h.Repos.Policies.List(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "list policies: "+err.Error())
		return
	}
	q := r.URL.Query()
	phases := multiValue(q.Get("phase"))
	modes := multiValue(q.Get("mode"))
	keyword := strings.ToLower(q.Get("keyword"))

	items := make([]policyListItem, 0, len(policies))
	for i := range policies {
		p := &policies[i]
		phase := policyPhase(p.CompileOK)
		if !matchAny(phases, phase) {
			continue
		}
		if !matchAny(modes, p.EnforcementMode) {
			continue
		}
		if keyword != "" &&
			!strings.Contains(strings.ToLower(p.Name), keyword) &&
			!strings.Contains(strings.ToLower(p.Description), keyword) {
			continue
		}
		items = append(items, policyListItem{
			Name:            p.Name,
			EnforcementMode: p.EnforcementMode,
			Phase:           phase,
			Description:     p.Description,
			CreatedAt:       p.CreatedAt,
			UpdatedAt:       p.UpdatedAt,
		})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	writeOK(w, items)
}

func (h *Handlers) getPolicy(w http.ResponseWriter, r *http.Request) {
	p, ok := h.fetchPolicy(w, r)
	if !ok {
		return
	}
	writeOK(w, toPolicyDetail(p))
}

// fetchPolicy loads the {name} path policy, writing 404/500 on failure.
func (h *Handlers) fetchPolicy(w http.ResponseWriter, r *http.Request) (*dbrepo.Policy, bool) {
	name := r.PathValue("name")
	p, err := h.Repos.Policies.GetByName(r.Context(), name)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeErr(w, http.StatusNotFound, "policy "+name+" not found")
		} else {
			writeErr(w, http.StatusInternalServerError, "get policy: "+err.Error())
		}
		return nil, false
	}
	return p, true
}

func (h *Handlers) createPolicy(w http.ResponseWriter, r *http.Request) {
	var req policyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	// validatePolicyRequest calls webhook.CompileRego synchronously — invalid
	// rego is rejected here with 400 and never reaches the database.
	if err := validatePolicyRequest(&req, true); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	p := &dbrepo.Policy{
		Name:            req.Name,
		Description:     req.Description,
		Rego:            req.Rego,
		EnforcementMode: req.EnforcementMode,
		Match:           datatypes.NewJSONType(req.Match),
		CompileOK:       true,
		Labels:          datatypes.NewJSONType(req.Labels),
		Annotations:     datatypes.NewJSONType(applyAnnotations(nil, req.Annotations)),
	}
	// Policy row + its v1 PolicyVersion snapshot commit or roll back
	// together — a failed version insert (e.g. a constraint violation) must
	// not leave an orphaned Policy row that would spuriously 409 on retry.
	err := h.Repos.Policies.WithTx(r.Context(), func(tx *dbrepo.PolicyRepo) error {
		if err := tx.Create(r.Context(), p); err != nil {
			return err
		}
		return tx.CreateVersion(r.Context(), &dbrepo.PolicyVersion{
			PolicyID:     p.ID,
			Version:      1,
			Rego:         p.Rego,
			SpecSnapshot: datatypes.NewJSONType(policySpecSnapshot(p)),
		})
	})
	if err != nil {
		if isUniqueViolation(err) {
			writeErr(w, http.StatusConflict, "policy "+req.Name+" already exists")
		} else {
			writeErr(w, http.StatusInternalServerError, "create policy: "+err.Error())
		}
		return
	}
	h.notifyChange(r, "policy")
	writeOK(w, map[string]string{"name": p.Name})
}

func policySpecSnapshot(p *dbrepo.Policy) v1alpha1.PolicySpec {
	return v1alpha1.PolicySpec{
		Match:           p.Match.Data(),
		EnforcementMode: p.EnforcementMode,
		Rego:            p.Rego,
		Description:     p.Description,
	}
}

// latestPolicyVersion returns a policy's version rows (newest first) and its
// current (highest) version number.
func (h *Handlers) latestPolicyVersion(r *http.Request, policyID int64) ([]dbrepo.PolicyVersion, int, error) {
	versions, err := h.Repos.Policies.ListVersions(r.Context(), policyID)
	if err != nil {
		return nil, 0, err
	}
	current := 0
	if len(versions) > 0 {
		current = versions[0].Version
	}
	return versions, current, nil
}

func (h *Handlers) updatePolicy(w http.ResponseWriter, r *http.Request) {
	var req policyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if err := validatePolicyRequest(&req, false); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	p, ok := h.fetchPolicy(w, r)
	if !ok {
		return
	}
	if req.ResourceVersion != "" && req.ResourceVersion != policyResourceVersion(p) {
		writeErr(w, http.StatusConflict, "policy has been modified, refresh and retry")
		return
	}
	// expectedUpdatedAt anchors the compare-and-swap below to the exact row
	// state we just read — closing the gap between this check and the write
	// that a plain "read, compare, then unconditional Save" leaves open.
	expectedUpdatedAt := p.UpdatedAt
	lbl := p.Labels.Data()
	if req.Labels != nil {
		if lbl == nil {
			lbl = map[string]string{}
		}
		for k, v := range req.Labels {
			lbl[k] = v
		}
	}
	ann := p.Annotations.Data()
	delete(ann, visualBuilderSpecAnnotation)
	ann = applyAnnotations(ann, req.Annotations)

	_, current, err := h.latestPolicyVersion(r, p.ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "list versions: "+err.Error())
		return
	}

	p.Match = datatypes.NewJSONType(req.Match)
	p.EnforcementMode = req.EnforcementMode
	p.Rego = req.Rego
	p.Description = req.Description
	p.CompileOK = true
	p.CompileError = ""
	p.Labels = datatypes.NewJSONType(lbl)
	p.Annotations = datatypes.NewJSONType(ann)

	fields := map[string]any{
		"match":            p.Match,
		"enforcement_mode": p.EnforcementMode,
		"rego":             p.Rego,
		"description":      p.Description,
		"compile_ok":       p.CompileOK,
		"compile_error":    p.CompileError,
		"labels":           p.Labels,
		"annotations":      p.Annotations,
	}
	// Policy row update + its new PolicyVersion snapshot commit or roll back
	// together (same reasoning as createPolicy above).
	err = h.Repos.Policies.WithTx(r.Context(), func(tx *dbrepo.PolicyRepo) error {
		if err := tx.UpdateCAS(r.Context(), p.ID, expectedUpdatedAt, fields); err != nil {
			return err
		}
		return tx.CreateVersion(r.Context(), &dbrepo.PolicyVersion{
			PolicyID:     p.ID,
			Version:      current + 1,
			Rego:         p.Rego,
			SpecSnapshot: datatypes.NewJSONType(policySpecSnapshot(p)),
		})
	})
	if err != nil {
		if errors.Is(err, dbrepo.ErrConflict) {
			writeErr(w, http.StatusConflict, "policy has been modified, refresh and retry")
			return
		}
		writeErr(w, http.StatusInternalServerError, "update policy: "+err.Error())
		return
	}
	h.notifyChange(r, "policy")
	writeOK(w, map[string]string{"name": p.Name})
}

func (h *Handlers) validatePolicy(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Rego string `json:"rego"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if _, err := webhook.CompileRego(req.Rego); err != nil {
		writeErr(w, http.StatusBadRequest, "rego compile failed: "+err.Error())
		return
	}
	writeOK(w, nil)
}

// referencingGroups returns the names of PolicyGroups that resolve
// policyName as an effective member (byName or bySelector, minus exclude),
// using the same groupresolve.Resolve the operator and gateway use — now
// that dbrepo.Policy carries real Labels (migration 0003), bySelector
// membership can be evaluated here too, not just byName.
func (h *Handlers) referencingGroups(r *http.Request, policyName string) ([]string, error) {
	groups, err := h.Repos.Groups.List(r.Context())
	if err != nil {
		return nil, err
	}
	policies, err := h.Repos.Policies.List(r.Context())
	if err != nil {
		return nil, err
	}
	resolvePolicies := make([]v1alpha1.Policy, len(policies))
	for i := range policies {
		resolvePolicies[i] = v1alpha1.Policy{
			ObjectMeta: metav1.ObjectMeta{Name: policies[i].Name, Labels: policies[i].Labels.Data()},
			Spec:       v1alpha1.PolicySpec{EnforcementMode: policies[i].EnforcementMode},
		}
	}
	var refs []string
	for i := range groups {
		members, _ := groupresolve.Resolve(groups[i].Spec.Data(), resolvePolicies)
		for _, m := range members {
			if m.Name == policyName {
				refs = append(refs, groups[i].Name)
				break
			}
		}
	}
	sort.Strings(refs)
	return refs, nil
}

func (h *Handlers) deletePolicy(w http.ResponseWriter, r *http.Request) {
	p, ok := h.fetchPolicy(w, r)
	if !ok {
		return
	}
	if r.URL.Query().Get("force") != "true" {
		refs, err := h.referencingGroups(r, p.Name)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "list policy groups: "+err.Error())
			return
		}
		if len(refs) > 0 {
			writeErrData(w, http.StatusConflict,
				"policy is referenced by policy groups",
				map[string][]string{"referencedBy": refs})
			return
		}
	}
	if err := h.Repos.Policies.Delete(r.Context(), p.ID); err != nil {
		writeErr(w, http.StatusInternalServerError, "delete policy: "+err.Error())
		return
	}
	h.notifyChange(r, "policy")
	writeOK(w, nil)
}

type versionEntry struct {
	Version         int                  `json:"version"`
	CreatedAt       time.Time            `json:"createdAt"`
	Rego            string               `json:"rego"`
	Match           v1alpha1.PolicyMatch `json:"match"`
	EnforcementMode string               `json:"enforcementMode"`
	IsCurrent       bool                 `json:"isCurrent"`
}

type versionList struct {
	CurrentVersion int            `json:"currentVersion"`
	Versions       []versionEntry `json:"versions"`
}

// listPolicyVersions returns snapshots for a policy, newest first.
func (h *Handlers) listPolicyVersions(w http.ResponseWriter, r *http.Request) {
	p, ok := h.fetchPolicy(w, r)
	if !ok {
		return
	}
	versions, current, err := h.latestPolicyVersion(r, p.ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "list versions: "+err.Error())
		return
	}
	entries := make([]versionEntry, 0, len(versions))
	for i := range versions {
		v := &versions[i]
		snap := v.SpecSnapshot.Data()
		entries = append(entries, versionEntry{
			Version:         v.Version,
			CreatedAt:       v.CreatedAt,
			Rego:            v.Rego,
			Match:           snap.Match,
			EnforcementMode: snap.EnforcementMode,
			IsCurrent:       v.Version == current,
		})
	}
	writeOK(w, versionList{CurrentVersion: current, Versions: entries})
}

// rollbackPolicy restores a policy to an earlier version's content. Unlike
// the CRD-era flow (which set spec.rollbackTo and waited for an operator to
// asynchronously settle it, tracked via a logical-cursor annotation — see
// cursor.go), this is synchronous and roll-forward only: "prev" appends a
// brand-new PolicyVersion whose content matches the snapshot one below the
// current head, exactly like a git revert. There is no controller loop left
// to settle an in-flight marker against, so "next" (undoing an undo) is not
// supported — the current version is always the head.
func (h *Handlers) rollbackPolicy(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Direction string `json:"direction"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if req.Direction != "prev" && req.Direction != "next" {
		writeErr(w, http.StatusBadRequest, `direction must be "prev" or "next"`)
		return
	}
	p, ok := h.fetchPolicy(w, r)
	if !ok {
		return
	}
	if req.Direction == "next" {
		writeErr(w, http.StatusBadRequest, "already at the latest version")
		return
	}

	versions, current, err := h.latestPolicyVersion(r, p.ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "list versions: "+err.Error())
		return
	}
	target := current - 1
	if target < 1 {
		writeErr(w, http.StatusBadRequest, "already at the oldest version")
		return
	}
	var targetVersion *dbrepo.PolicyVersion
	for i := range versions {
		if versions[i].Version == target {
			targetVersion = &versions[i]
			break
		}
	}
	if targetVersion == nil {
		writeErr(w, http.StatusGone, fmt.Sprintf("version %d snapshot has been pruned", target))
		return
	}

	// expectedUpdatedAt anchors the compare-and-swap below to the exact row
	// state fetchPolicy just read.
	expectedUpdatedAt := p.UpdatedAt
	snap := targetVersion.SpecSnapshot.Data()
	p.Rego = targetVersion.Rego
	p.Match = datatypes.NewJSONType(snap.Match)
	p.EnforcementMode = snap.EnforcementMode
	p.Description = snap.Description
	p.CompileOK = true
	p.CompileError = ""
	fields := map[string]any{
		"rego":             p.Rego,
		"match":            p.Match,
		"enforcement_mode": p.EnforcementMode,
		"description":      p.Description,
		"compile_ok":       p.CompileOK,
		"compile_error":    p.CompileError,
	}
	newVersion := current + 1
	// Policy row rollback + its new PolicyVersion snapshot commit or roll
	// back together (same reasoning as createPolicy above).
	err = h.Repos.Policies.WithTx(r.Context(), func(tx *dbrepo.PolicyRepo) error {
		if err := tx.UpdateCAS(r.Context(), p.ID, expectedUpdatedAt, fields); err != nil {
			return err
		}
		return tx.CreateVersion(r.Context(), &dbrepo.PolicyVersion{
			PolicyID:     p.ID,
			Version:      newVersion,
			Rego:         targetVersion.Rego,
			SpecSnapshot: targetVersion.SpecSnapshot,
		})
	})
	if err != nil {
		if errors.Is(err, dbrepo.ErrConflict) {
			writeErr(w, http.StatusConflict, "policy has been modified, refresh and retry")
			return
		}
		writeErr(w, http.StatusInternalServerError, "update policy: "+err.Error())
		return
	}
	h.notifyChange(r, "policy")
	writeOK(w, map[string]int{"restoredFromVersion": target, "newVersion": newVersion})
}

// resourceSuggestions 聚合所有 Policy 用过的 apiGroups/apiVersions/resources，
// 去重后供前端下拉建议使用；不读取 rego，避免不必要的大字段传输。
func (h *Handlers) resourceSuggestions(w http.ResponseWriter, r *http.Request) {
	policies, err := h.Repos.Policies.List(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "list policies: "+err.Error())
		return
	}
	groups := map[string]struct{}{}
	versions := map[string]struct{}{}
	resources := map[string]struct{}{}
	resourcesByGroup := map[string]map[string]struct{}{}
	for i := range policies {
		match := policies[i].Match.Data()
		for _, res := range match.Resources {
			for _, g := range res.APIGroups {
				groups[g] = struct{}{}
				if resourcesByGroup[g] == nil {
					resourcesByGroup[g] = map[string]struct{}{}
				}
				for _, rsc := range res.Resources {
					resourcesByGroup[g][rsc] = struct{}{}
				}
			}
			for _, v := range res.APIVersions {
				versions[v] = struct{}{}
			}
			for _, rsc := range res.Resources {
				resources[rsc] = struct{}{}
			}
		}
	}
	byGroup := make(map[string][]string, len(resourcesByGroup))
	for g, rsc := range resourcesByGroup {
		byGroup[g] = sortedKeys(rsc)
	}
	resp := resourceSuggestionsResponse{
		APIGroups:        sortedKeys(groups),
		APIVersions:      sortedKeys(versions),
		Resources:        sortedKeys(resources),
		ResourcesByGroup: byGroup,
	}
	writeOK(w, resp)
}

func sortedKeys(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
