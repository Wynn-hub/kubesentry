package console

import (
	"net/http"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/Wynn-hub/kubesentry/internal/api/v1alpha1"
	"github.com/Wynn-hub/kubesentry/internal/dbrepo"
	"github.com/Wynn-hub/kubesentry/internal/groupresolve"
)

type groupSummary struct {
	Name          string `json:"name"`
	DisplayName   string `json:"displayName"`
	Enabled       bool   `json:"enabled"`
	ResolvedCount int    `json:"resolvedCount"`
	EnforceCount  int    `json:"enforceCount"`
	AuditCount    int    `json:"auditCount"`
}

type summaryData struct {
	Totals map[string]int `json:"totals"`
	Groups []groupSummary `json:"groups"`
}

// resolvablePolicies adapts dbrepo.Policy rows into the plain v1alpha1.Policy
// values groupresolve.Resolve expects. Mirrors the same conversion
// referencingGroups (handlers_policy.go) does for the same reason — kept as a
// separate copy here rather than a shared helper since handlers_policy.go is
// out of scope for this task.
func resolvablePolicies(in []dbrepo.Policy) []v1alpha1.Policy {
	out := make([]v1alpha1.Policy, len(in))
	for i := range in {
		out[i] = v1alpha1.Policy{
			ObjectMeta: metav1.ObjectMeta{Name: in[i].Name, Labels: in[i].Labels.Data()},
			Spec:       v1alpha1.PolicySpec{EnforcementMode: in[i].EnforcementMode},
		}
	}
	return out
}

// summary aggregates dashboard counts from the GORM store. Per-group
// resolved/enforce/audit counts are computed live via groupresolve.Resolve
// against the current policy list (spec D7: PolicyGroup resolved status is
// never stored, always computed on read) — there is no stored status field
// to read them from on a DB-backed, hub-only console.
func (h *Handlers) summary(w http.ResponseWriter, r *http.Request) {
	policies, err := h.Repos.Policies.List(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "list policies: "+err.Error())
		return
	}
	groups, err := h.Repos.Groups.List(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "list groups: "+err.Error())
		return
	}
	exceptions, err := h.Repos.Exceptions.List(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "list exceptions: "+err.Error())
		return
	}

	s := summaryData{
		Totals: map[string]int{"policies": len(policies), "policygroups": len(groups), "exceptions": len(exceptions)},
		Groups: []groupSummary{},
	}
	resolvePolicies := resolvablePolicies(policies)
	for i := range groups {
		g := &groups[i]
		spec := g.Spec.Data()
		members, _ := groupresolve.Resolve(spec, resolvePolicies)
		gs := groupSummary{Name: g.Name, DisplayName: spec.DisplayName, Enabled: spec.Enabled, ResolvedCount: len(members)}
		for _, m := range members {
			switch m.EnforcementMode {
			case "enforce":
				gs.EnforceCount++
			case "audit":
				gs.AuditCount++
			}
		}
		s.Groups = append(s.Groups, gs)
	}
	writeOK(w, s)
}
