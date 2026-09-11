package dto

import (
	"time"

	"github.com/Wynn-hub/kubesentry/internal/api/v1alpha1"
)

type PolicyInput struct {
	Name            *string              `json:"name,omitempty"`
	Description     *string              `json:"description,omitempty"`
	EnforcementMode string               `json:"enforcementMode"`
	Match           v1alpha1.PolicyMatch `json:"match"`
	Rego            string               `json:"rego"`
	Labels          *map[string]string   `json:"labels,omitempty"`
	Annotations     *map[string]string   `json:"annotations,omitempty"`
	ResourceVersion *string              `json:"resourceVersion,omitempty"`
}

type PolicyItem struct {
	Name            string    `json:"name"`
	Source          string    `json:"source"`
	EnforcementMode string    `json:"enforcementMode"`
	Phase           string    `json:"phase"`
	Description     string    `json:"description"`
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
}

type PolicyPageView struct {
	Total    int          `json:"total"`
	Page     int          `json:"page"`
	PageSize int          `json:"pageSize"`
	Items    []PolicyItem `json:"items"`
}

type PolicyView struct {
	Name            string               `json:"name"`
	Source          string               `json:"source"`
	Description     string               `json:"description"`
	Labels          map[string]string    `json:"labels"`
	Annotations     map[string]string    `json:"annotations"`
	EnforcementMode string               `json:"enforcementMode"`
	Match           v1alpha1.PolicyMatch `json:"match"`
	Rego            string               `json:"rego"`
	Phase           string               `json:"phase"`
	CompileError    *string              `json:"compileError,omitempty"`
	ReferencedBy    []string             `json:"referencedBy"`
	ResourceVersion string               `json:"resourceVersion"`
	CreatedAt       time.Time            `json:"createdAt"`
	UpdatedAt       time.Time            `json:"updatedAt"`
}

type PolicyVersionItemView struct {
	Version         int                  `json:"version"`
	CreatedAt       time.Time            `json:"createdAt"`
	Phase           string               `json:"phase"`
	Rego            string               `json:"rego"`
	Match           v1alpha1.PolicyMatch `json:"match"`
	EnforcementMode string               `json:"enforcementMode"`
	IsCurrent       bool                 `json:"isCurrent"`
}

type PolicyVersionListView struct {
	CurrentVersion int                     `json:"currentVersion"`
	Cursor         int                     `json:"cursor"`
	Head           int                     `json:"head"`
	InFlight       bool                    `json:"inFlight"`
	PrevEnabled    bool                    `json:"prevEnabled"`
	NextEnabled    bool                    `json:"nextEnabled"`
	Versions       []PolicyVersionItemView `json:"versions"`
}

type PolicyRollbackView struct {
	RestoredFromVersion int `json:"restoredFromVersion"`
	NewVersion          int `json:"newVersion"`
}

type ResourceSuggestionsView struct {
	APIGroups        []string            `json:"apiGroups"`
	APIVersions      []string            `json:"apiVersions"`
	Resources        []string            `json:"resources"`
	ResourcesByGroup map[string][]string `json:"resourcesByGroup"`
}
