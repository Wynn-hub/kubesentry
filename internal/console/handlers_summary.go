package console

import (
	"net/http"

	"github.com/Wynn-hub/kubesentry/internal/api/v1alpha1"
)

type summaryData struct {
	Totals          map[string]int `json:"totals"`
	PolicyPhases    map[string]int `json:"policyPhases"`
	PolicyModes     map[string]int `json:"policyModes"`
	GroupPhases     map[string]int `json:"groupPhases"`
	ExceptionPhases map[string]int `json:"exceptionPhases"`
}

func (h *Handlers) summary(w http.ResponseWriter, r *http.Request) {
	var (
		policies   v1alpha1.PolicyList
		groups     v1alpha1.PolicyGroupList
		exceptions v1alpha1.PolicyExceptionList
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

	s := summaryData{
		Totals: map[string]int{
			"policies":     len(policies.Items),
			"policygroups": len(groups.Items),
			"exceptions":   len(exceptions.Items),
		},
		PolicyPhases:    map[string]int{},
		PolicyModes:     map[string]int{},
		GroupPhases:     map[string]int{},
		ExceptionPhases: map[string]int{},
	}
	for i := range policies.Items {
		s.PolicyPhases[policies.Items[i].Status.Phase]++
		s.PolicyModes[policies.Items[i].Spec.EnforcementMode]++
	}
	for i := range groups.Items {
		s.GroupPhases[groups.Items[i].Status.Phase]++
	}
	for i := range exceptions.Items {
		s.ExceptionPhases[exceptions.Items[i].Status.Phase]++
	}
	writeOK(w, s)
}
