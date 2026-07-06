package console

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/Wynn-hub/kubesentry/internal/api/v1alpha1"
)

type exceptionListItem struct {
	Name          string       `json:"name"`
	Phase         string       `json:"phase"`
	Reason        string       `json:"reason"`
	Duration      string       `json:"duration"`
	TargetSummary string       `json:"targetSummary"`
	ExpiresAt     *metav1.Time `json:"expiresAt,omitempty"`
}

type exceptionDetail struct {
	Name            string                         `json:"name"`
	ResourceVersion string                         `json:"resourceVersion"`
	Spec            v1alpha1.PolicyExceptionSpec   `json:"spec"`
	Status          v1alpha1.PolicyExceptionStatus `json:"status"`
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
	var list v1alpha1.PolicyExceptionList
	if err := h.Client.List(r.Context(), &list); err != nil {
		writeErr(w, http.StatusInternalServerError, "list exceptions: "+err.Error())
		return
	}
	items := make([]exceptionListItem, 0, len(list.Items))
	for i := range list.Items {
		e := &list.Items[i]
		items = append(items, exceptionListItem{
			Name:          e.Name,
			Phase:         e.Status.Phase,
			Reason:        e.Spec.Reason,
			Duration:      e.Spec.Duration,
			TargetSummary: targetSummary(&e.Spec),
			ExpiresAt:     e.Status.ExpiresAt,
		})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	writeOK(w, items)
}

func (h *Handlers) getException(w http.ResponseWriter, r *http.Request) {
	e, ok := h.fetchException(w, r)
	if !ok {
		return
	}
	writeOK(w, exceptionDetail{
		Name:            e.Name,
		ResourceVersion: e.ResourceVersion,
		Spec:            e.Spec,
		Status:          e.Status,
	})
}

func (h *Handlers) fetchException(w http.ResponseWriter, r *http.Request) (*v1alpha1.PolicyException, bool) {
	name := r.PathValue("name")
	var e v1alpha1.PolicyException
	if err := h.Client.Get(r.Context(), client.ObjectKey{Name: name}, &e); err != nil {
		if apierrors.IsNotFound(err) {
			writeErr(w, http.StatusNotFound, "exception "+name+" not found")
		} else {
			writeErr(w, http.StatusInternalServerError, "get exception: "+err.Error())
		}
		return nil, false
	}
	return &e, true
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
	e := &v1alpha1.PolicyException{
		ObjectMeta: metav1.ObjectMeta{Name: req.Name},
		Spec:       exceptionSpecFrom(&req),
	}
	if err := h.Client.Create(r.Context(), e); err != nil {
		if apierrors.IsAlreadyExists(err) {
			writeErr(w, http.StatusConflict, "exception "+req.Name+" already exists")
		} else {
			writeErr(w, http.StatusInternalServerError, "create exception: "+err.Error())
		}
		return
	}
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
	if req.ResourceVersion != "" && req.ResourceVersion != e.ResourceVersion {
		writeErr(w, http.StatusConflict, "exception has been modified, refresh and retry")
		return
	}
	e.Spec = exceptionSpecFrom(&req)
	if err := h.Client.Update(r.Context(), e); err != nil {
		if apierrors.IsConflict(err) {
			writeErr(w, http.StatusConflict, "exception has been modified, refresh and retry")
		} else {
			writeErr(w, http.StatusInternalServerError, "update exception: "+err.Error())
		}
		return
	}
	writeOK(w, map[string]string{"name": e.Name})
}

func (h *Handlers) deleteException(w http.ResponseWriter, r *http.Request) {
	e, ok := h.fetchException(w, r)
	if !ok {
		return
	}
	if err := h.Client.Delete(r.Context(), e); err != nil {
		writeErr(w, http.StatusInternalServerError, "delete exception: "+err.Error())
		return
	}
	writeOK(w, nil)
}
