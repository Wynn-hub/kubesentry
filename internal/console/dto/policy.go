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

// PolicyPath describes the name path parameter shared by policy detail operations.
type PolicyPath struct {
	// 策略名称
	Name string `uri:"name" binding:"required"`
}

// SwagPolicyMatch is the Swagger-only representation of a policy match.
type SwagPolicyMatch struct {
	Operations []string            `json:"operations" binding:"required" enums:"CREATE,UPDATE,DELETE,CONNECT"`
	Resources  []SwagMatchResource `json:"resources" binding:"required"`
}

// SwagMatchResource is the Swagger-only representation of one matched resource rule.
type SwagMatchResource struct {
	APIGroups   []string `json:"apiGroups" binding:"required"`
	APIVersions []string `json:"apiVersions" binding:"required"`
	Resources   []string `json:"resources" binding:"required"`
}

// SwagPolicyInput describes the stable policy create and update JSON shape.
type SwagPolicyInput struct {
	Name            *string           `json:"name,omitempty"`
	Description     *string           `json:"description,omitempty"`
	EnforcementMode string            `json:"enforcementMode" binding:"required" enums:"enforce,audit"`
	Match           SwagPolicyMatch   `json:"match" binding:"required"`
	Rego            string            `json:"rego" binding:"required"`
	Labels          map[string]string `json:"labels,omitempty"`
	Annotations     map[string]string `json:"annotations,omitempty"`
	ResourceVersion *string           `json:"resourceVersion,omitempty"`
}

// SwagPolicyDetail describes the stable policy detail JSON shape.
type SwagPolicyDetail struct {
	Name            string            `json:"name" binding:"required"`
	Source          string            `json:"source" binding:"required" enums:"builtin,custom"`
	Description     string            `json:"description" binding:"required"`
	Labels          map[string]string `json:"labels" binding:"required"`
	Annotations     map[string]string `json:"annotations" binding:"required"`
	EnforcementMode string            `json:"enforcementMode" binding:"required" enums:"enforce,audit"`
	Match           SwagPolicyMatch   `json:"match" binding:"required"`
	Rego            string            `json:"rego" binding:"required"`
	Phase           string            `json:"phase" binding:"required" enums:"Ready,Invalid"`
	CompileError    *string           `json:"compileError,omitempty"`
	ReferencedBy    []string          `json:"referencedBy" binding:"required"`
	ResourceVersion string            `json:"resourceVersion" binding:"required"`
	CreatedAt       time.Time         `json:"createdAt" binding:"required" format:"date-time"`
	UpdatedAt       time.Time         `json:"updatedAt" binding:"required" format:"date-time"`
}

type PolicyNameView struct {
	Name string `json:"name" binding:"required"`
}

type SwagPolicyVersionEntry struct {
	Version         int             `json:"version" binding:"required"`
	CreatedAt       time.Time       `json:"createdAt" binding:"required" format:"date-time"`
	Phase           string          `json:"phase" binding:"required" enums:"Ready,Invalid"`
	Rego            string          `json:"rego" binding:"required"`
	Match           SwagPolicyMatch `json:"match" binding:"required"`
	EnforcementMode string          `json:"enforcementMode" binding:"required" enums:"enforce,audit"`
	IsCurrent       bool            `json:"isCurrent" binding:"required"`
}

type SwagPolicyVersionList struct {
	CurrentVersion int                      `json:"currentVersion" binding:"required"`
	Cursor         int                      `json:"cursor" binding:"required"`
	Head           int                      `json:"head" binding:"required"`
	InFlight       bool                     `json:"inFlight" binding:"required"`
	PrevEnabled    bool                     `json:"prevEnabled" binding:"required"`
	NextEnabled    bool                     `json:"nextEnabled" binding:"required"`
	Versions       []SwagPolicyVersionEntry `json:"versions" binding:"required"`
}

type SwagResourceSuggestions struct {
	APIGroups        []string            `json:"apiGroups" binding:"required"`
	APIVersions      []string            `json:"apiVersions" binding:"required"`
	Resources        []string            `json:"resources" binding:"required"`
	ResourcesByGroup map[string][]string `json:"resourcesByGroup" binding:"required"`
}

type SwagRollbackInput struct {
	Direction string `json:"direction" binding:"required" enums:"prev,next"`
}

type SwagRollbackResult struct {
	RestoredFromVersion int `json:"restoredFromVersion" binding:"required"`
	NewVersion          int `json:"newVersion" binding:"required"`
}

type SwagValidateRegoInput struct {
	Rego string `json:"rego" binding:"required"`
}
