package console

import (
	"net/http"
	"sort"

	admissionregv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"

	"github.com/Wynn-hub/kubesentry/internal/api/v1alpha1"
)

const defaultVWCName = "kubesentry"

const maxRecentVersions = 10

type webhookSummary struct {
	Configured  bool `json:"configured"`
	CABundleSet bool `json:"caBundleSet"`
	RuleCount   int  `json:"ruleCount"`
}

type nsCoverage struct {
	Protected int `json:"protected"`
	Total     int `json:"total"`
}

type groupSummary struct {
	Name              string `json:"name"`
	DisplayName       string `json:"displayName"`
	Enabled           bool   `json:"enabled"`
	Phase             string `json:"phase"`
	ResolvedCount     int    `json:"resolvedCount"`
	EnforceCount      int    `json:"enforceCount"`
	AuditCount        int    `json:"auditCount"`
	MatchedNamespaces int    `json:"matchedNamespaces"`
}

type recentVersion struct {
	Policy    string      `json:"policy"`
	Version   int64       `json:"version"`
	CreatedAt metav1.Time `json:"createdAt"`
}

type summaryData struct {
	Totals            map[string]int  `json:"totals"`
	PolicyPhases      map[string]int  `json:"policyPhases"`
	PolicyModes       map[string]int  `json:"policyModes"`
	GroupPhases       map[string]int  `json:"groupPhases"`
	ExceptionPhases   map[string]int  `json:"exceptionPhases"`
	Webhook           webhookSummary  `json:"webhook"`
	NamespaceCoverage nsCoverage      `json:"namespaceCoverage"`
	Groups            []groupSummary  `json:"groups"`
	RecentVersions    []recentVersion `json:"recentVersions"`
}

func (h *Handlers) summary(w http.ResponseWriter, r *http.Request) {
	var (
		policies   v1alpha1.PolicyList
		groups     v1alpha1.PolicyGroupList
		exceptions v1alpha1.PolicyExceptionList
		namespaces corev1.NamespaceList
		versions   v1alpha1.PolicyVersionList
	)
	if err := h.Client.List(r.Context(), &policies); err != nil {
		writeErr(w, http.StatusInternalServerError, "list policies: "+err.Error())
		return
	}
	if err := h.Client.List(r.Context(), &groups); err != nil {
		writeErr(w, http.StatusInternalServerError, "list policygroups: "+err.Error())
		return
	}
	if err := h.Client.List(r.Context(), &exceptions); err != nil {
		writeErr(w, http.StatusInternalServerError, "list exceptions: "+err.Error())
		return
	}
	if err := h.Client.List(r.Context(), &namespaces); err != nil {
		writeErr(w, http.StatusInternalServerError, "list namespaces: "+err.Error())
		return
	}
	if err := h.Client.List(r.Context(), &versions); err != nil {
		writeErr(w, http.StatusInternalServerError, "list policyversions: "+err.Error())
		return
	}

	s := summaryData{
		Totals: map[string]int{
			"policies":     len(policies.Items),
			"policygroups": len(groups.Items),
			"exceptions":   len(exceptions.Items),
		},
		PolicyPhases:      map[string]int{},
		PolicyModes:       map[string]int{},
		GroupPhases:       map[string]int{},
		ExceptionPhases:   map[string]int{},
		NamespaceCoverage: nsCoverage{Total: len(namespaces.Items)},
		Groups:            []groupSummary{},
		RecentVersions:    []recentVersion{},
	}
	for i := range policies.Items {
		s.PolicyPhases[policies.Items[i].Status.Phase]++
		s.PolicyModes[policies.Items[i].Spec.EnforcementMode]++
	}
	for i := range exceptions.Items {
		s.ExceptionPhases[exceptions.Items[i].Status.Phase]++
	}

	protected := map[string]bool{}
	for i := range groups.Items {
		g := &groups.Items[i]
		s.GroupPhases[g.Status.Phase]++
		gs := groupSummary{
			Name:          g.Name,
			DisplayName:   g.Spec.DisplayName,
			Enabled:       g.Spec.Enabled,
			Phase:         g.Status.Phase,
			ResolvedCount: len(g.Status.ResolvedPolicies),
		}
		for _, m := range g.Status.ResolvedPolicies {
			switch m.EnforcementMode {
			case "enforce":
				gs.EnforceCount++
			case "audit":
				gs.AuditCount++
			}
		}
		// nil selector = 匹配全部命名空间（与 webhook cache 语义一致）
		var sel labels.Selector
		if g.Spec.NamespaceSelector != nil {
			var err error
			sel, err = metav1.LabelSelectorAsSelector(g.Spec.NamespaceSelector)
			if err != nil {
				sel = labels.Nothing()
			}
		}
		for j := range namespaces.Items {
			if sel != nil && !sel.Matches(labels.Set(namespaces.Items[j].Labels)) {
				continue
			}
			gs.MatchedNamespaces++
			if g.Spec.Enabled {
				protected[namespaces.Items[j].Name] = true
			}
		}
		s.Groups = append(s.Groups, gs)
	}
	s.NamespaceCoverage.Protected = len(protected)

	sort.Slice(versions.Items, func(a, b int) bool {
		return versions.Items[a].Spec.CreatedAt.Time.After(versions.Items[b].Spec.CreatedAt.Time)
	})
	for i := 0; i < len(versions.Items) && i < maxRecentVersions; i++ {
		v := &versions.Items[i]
		s.RecentVersions = append(s.RecentVersions, recentVersion{
			Policy:    v.Spec.PolicyRef,
			Version:   v.Spec.Version,
			CreatedAt: v.Spec.CreatedAt,
		})
	}

	vwcName := h.VWCName
	if vwcName == "" {
		vwcName = defaultVWCName
	}
	var vwc admissionregv1.ValidatingWebhookConfiguration
	if err := h.Client.Get(r.Context(), types.NamespacedName{Name: vwcName}, &vwc); err == nil {
		s.Webhook.Configured = true
		if len(vwc.Webhooks) > 0 {
			s.Webhook.RuleCount = len(vwc.Webhooks[0].Rules)
			s.Webhook.CABundleSet = len(vwc.Webhooks[0].ClientConfig.CABundle) > 0
		}
	}

	writeOK(w, s)
}
