package dto

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Wynn-hub/kubesentry/api/kubesentry/v1alpha1"
)

type PolicyInput struct {
	Name            *string            `json:"name,omitempty"`
	Description     *string            `json:"description,omitempty"`
	EnforcementMode string             `json:"enforcementMode"`
	Match           PolicyMatchInput   `json:"match"`
	Rego            string             `json:"rego"`
	Labels          *map[string]string `json:"labels,omitempty"`
	Annotations     *map[string]string `json:"annotations,omitempty"`
	ResourceVersion *string            `json:"resourceVersion,omitempty"`
}

func (r *PolicyInput) UnmarshalJSON(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	if token, err := decoder.Token(); err != nil || token != json.Delim('{') {
		return fmt.Errorf("decode policy request: expected object")
	}
	required := map[string]bool{"enforcementMode": false, "match": false, "rego": false}
	for decoder.More() {
		key, err := decoder.Token()
		if err != nil {
			return err
		}
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil {
			return err
		}
		switch key {
		case "name":
			err = decodeOptionalNonNullJSON(raw, "name", &r.Name)
		case "description":
			err = decodeOptionalNonNullJSON(raw, "description", &r.Description)
		case "enforcementMode":
			required["enforcementMode"] = true
			err = decodeNonNullJSON(raw, "enforcementMode", &r.EnforcementMode)
		case "match":
			required["match"] = true
			err = decodeNonNullJSON(raw, "match", &r.Match)
		case "rego":
			required["rego"] = true
			err = decodeNonNullJSON(raw, "rego", &r.Rego)
		case "labels":
			err = decodeOptionalNonNullJSON(raw, "labels", &r.Labels)
		case "annotations":
			err = decodeOptionalNonNullJSON(raw, "annotations", &r.Annotations)
		case "resourceVersion":
			err = decodeOptionalNonNullJSON(raw, "resourceVersion", &r.ResourceVersion)
		}
		if err != nil {
			return err
		}
	}
	if _, err := decoder.Token(); err != nil {
		return err
	}
	for _, field := range []string{"enforcementMode", "match", "rego"} {
		if !required[field] {
			return fmt.Errorf("%s: field is required", field)
		}
	}
	if r.EnforcementMode != "enforce" && r.EnforcementMode != "audit" {
		return fmt.Errorf("enforcementMode: invalid value %q", r.EnforcementMode)
	}
	return nil
}

type PolicyMatchInput struct {
	Operations []string             `json:"operations" binding:"required" enums:"CREATE,UPDATE,DELETE,CONNECT"`
	Resources  []MatchResourceInput `json:"resources" binding:"required"`
}

func (m *PolicyMatchInput) UnmarshalJSON(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	if token, err := decoder.Token(); err != nil || token != json.Delim('{') {
		return fmt.Errorf("decode policy match: expected object")
	}
	var operationsSet, resourcesSet bool
	for decoder.More() {
		key, err := decoder.Token()
		if err != nil {
			return err
		}
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil {
			return err
		}
		switch key {
		case "operations":
			operationsSet = true
			m.Operations = nil
			err = decodeNonNullJSON(raw, "operations", &m.Operations)
		case "resources":
			resourcesSet = true
			m.Resources = nil
			err = decodeNonNullJSON(raw, "resources", &m.Resources)
		}
		if err != nil {
			return err
		}
	}
	if _, err := decoder.Token(); err != nil {
		return err
	}
	if !operationsSet {
		return fmt.Errorf("operations: field is required")
	}
	if !resourcesSet {
		return fmt.Errorf("resources: field is required")
	}
	for _, operation := range m.Operations {
		if operation != "CREATE" && operation != "UPDATE" && operation != "DELETE" && operation != "CONNECT" {
			return fmt.Errorf("operations: invalid value %q", operation)
		}
	}
	return nil
}

type MatchResourceInput struct {
	APIGroups   []string `json:"apiGroups" binding:"required"`
	APIVersions []string `json:"apiVersions" binding:"required"`
	Resources   []string `json:"resources" binding:"required"`
}

func (m *MatchResourceInput) UnmarshalJSON(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	if token, err := decoder.Token(); err != nil || token != json.Delim('{') {
		return fmt.Errorf("decode match resource: expected object")
	}
	required := map[string]bool{"apiGroups": false, "apiVersions": false, "resources": false}
	for decoder.More() {
		key, err := decoder.Token()
		if err != nil {
			return err
		}
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil {
			return err
		}
		switch key {
		case "apiGroups":
			required["apiGroups"] = true
			m.APIGroups = nil
			err = decodeNonNullJSON(raw, "apiGroups", &m.APIGroups)
		case "apiVersions":
			required["apiVersions"] = true
			m.APIVersions = nil
			err = decodeNonNullJSON(raw, "apiVersions", &m.APIVersions)
		case "resources":
			required["resources"] = true
			m.Resources = nil
			err = decodeNonNullJSON(raw, "resources", &m.Resources)
		}
		if err != nil {
			return err
		}
	}
	if _, err := decoder.Token(); err != nil {
		return err
	}
	for _, field := range []string{"apiGroups", "apiVersions", "resources"} {
		if !required[field] {
			return fmt.Errorf("%s: field is required", field)
		}
	}
	return nil
}

type PolicyRollbackInput struct {
	Direction string `json:"direction"`
}

func (r *PolicyRollbackInput) UnmarshalJSON(data []byte) error {
	direction, err := decodeRequiredString(data, "direction")
	if err != nil {
		return err
	}
	r.Direction = direction
	return nil
}

func (r *PolicyRollbackInput) Validate() error {
	if r.Direction != "prev" && r.Direction != "next" {
		return fmt.Errorf("direction: invalid value %q", r.Direction)
	}
	return nil
}

type PolicyValidateRegoInput struct {
	Rego string `json:"rego"`
}

func (r *PolicyValidateRegoInput) UnmarshalJSON(data []byte) error {
	rego, err := decodeRequiredString(data, "rego")
	if err != nil {
		return err
	}
	r.Rego = rego
	return nil
}

func decodeRequiredString(data []byte, field string) (string, error) {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(data, &object); err != nil {
		return "", err
	}
	raw, ok := object[field]
	if !ok {
		return "", fmt.Errorf("%s: field is required", field)
	}
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return "", fmt.Errorf("%s: must be a string", field)
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", fmt.Errorf("%s: %w", field, err)
	}
	return value, nil
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

type PolicyReferencedByView struct {
	ReferencedBy []string `json:"referencedBy"`
}

type PolicyBadRequestHTTPError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (*PolicyBadRequestHTTPError) HTTPStatus() int            { return 400 }
func (*PolicyBadRequestHTTPError) listPoliciesRes()           {}
func (*PolicyBadRequestHTTPError) createPolicyRes()           {}
func (*PolicyBadRequestHTTPError) updatePolicyRes()           {}
func (*PolicyBadRequestHTTPError) rollbackPolicyRes()         {}
func (*PolicyBadRequestHTTPError) validatePolicyRegoRes()     {}
func (*PolicyBadRequestHTTPError) getResourceSuggestionsRes() {}

type PolicyNotFoundHTTPError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (*PolicyNotFoundHTTPError) HTTPStatus() int        { return 404 }
func (*PolicyNotFoundHTTPError) getPolicyRes()          {}
func (*PolicyNotFoundHTTPError) updatePolicyRes()       {}
func (*PolicyNotFoundHTTPError) deletePolicyRes()       {}
func (*PolicyNotFoundHTTPError) listPolicyVersionsRes() {}
func (*PolicyNotFoundHTTPError) rollbackPolicyRes()     {}

type PolicyConflictHTTPError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (*PolicyConflictHTTPError) HTTPStatus() int    { return 409 }
func (*PolicyConflictHTTPError) createPolicyRes()   {}
func (*PolicyConflictHTTPError) updatePolicyRes()   {}
func (*PolicyConflictHTTPError) rollbackPolicyRes() {}

type DeletePolicyConflictHTTPError struct {
	Code    int                    `json:"code"`
	Message string                 `json:"message"`
	Data    PolicyReferencedByView `json:"data"`
}

func (*DeletePolicyConflictHTTPError) HTTPStatus() int  { return 409 }
func (*DeletePolicyConflictHTTPError) deletePolicyRes() {}

type PolicyGoneHTTPError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (*PolicyGoneHTTPError) HTTPStatus() int    { return 410 }
func (*PolicyGoneHTTPError) rollbackPolicyRes() {}

type PolicyInternalServerHTTPError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (*PolicyInternalServerHTTPError) HTTPStatus() int            { return 500 }
func (*PolicyInternalServerHTTPError) listPoliciesRes()           {}
func (*PolicyInternalServerHTTPError) createPolicyRes()           {}
func (*PolicyInternalServerHTTPError) getPolicyRes()              {}
func (*PolicyInternalServerHTTPError) updatePolicyRes()           {}
func (*PolicyInternalServerHTTPError) deletePolicyRes()           {}
func (*PolicyInternalServerHTTPError) listPolicyVersionsRes()     {}
func (*PolicyInternalServerHTTPError) rollbackPolicyRes()         {}
func (*PolicyInternalServerHTTPError) getResourceSuggestionsRes() {}

type PolicyListHTTPResponse struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    *PolicyPageView `json:"data"`
}

func (*PolicyListHTTPResponse) listPoliciesRes() {}

type PolicyDetailHTTPResponse struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    *PolicyView `json:"data"`
}

func (*PolicyDetailHTTPResponse) getPolicyRes() {}

type PolicyNameHTTPResponse struct {
	Code    int            `json:"code"`
	Message string         `json:"message"`
	Data    PolicyNameView `json:"data"`
}

func (*PolicyNameHTTPResponse) createPolicyRes() {}
func (*PolicyNameHTTPResponse) updatePolicyRes() {}

type PolicyEmptyHTTPResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (*PolicyEmptyHTTPResponse) deletePolicyRes()       {}
func (*PolicyEmptyHTTPResponse) validatePolicyRegoRes() {}

type PolicyVersionListHTTPResponse struct {
	Code    int                    `json:"code"`
	Message string                 `json:"message"`
	Data    *PolicyVersionListView `json:"data"`
}

func (*PolicyVersionListHTTPResponse) listPolicyVersionsRes() {}

type PolicyRollbackHTTPResponse struct {
	Code    int                 `json:"code"`
	Message string              `json:"message"`
	Data    *PolicyRollbackView `json:"data"`
}

func (*PolicyRollbackHTTPResponse) rollbackPolicyRes() {}

type ResourceSuggestionsHTTPResponse struct {
	Code    int                      `json:"code"`
	Message string                   `json:"message"`
	Data    *ResourceSuggestionsView `json:"data"`
}

func (*ResourceSuggestionsHTTPResponse) getResourceSuggestionsRes() {}

// PolicyPath describes the name path parameter shared by policy detail operations.
type PolicyPath struct {
	// 策略名称
	Name string `uri:"name" binding:"required"`
}

// SwagPolicyInput describes the stable policy create and update JSON shape.
type SwagPolicyInput struct {
	Name            *string           `json:"name,omitempty"`
	Description     *string           `json:"description,omitempty"`
	EnforcementMode string            `json:"enforcementMode" binding:"required" enums:"enforce,audit"`
	Match           PolicyMatchInput  `json:"match" binding:"required"`
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
	Match           PolicyMatchInput  `json:"match" binding:"required"`
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
	Version         int              `json:"version" binding:"required"`
	CreatedAt       time.Time        `json:"createdAt" binding:"required" format:"date-time"`
	Phase           string           `json:"phase" binding:"required" enums:"Ready,Invalid"`
	Rego            string           `json:"rego" binding:"required"`
	Match           PolicyMatchInput `json:"match" binding:"required"`
	EnforcementMode string           `json:"enforcementMode" binding:"required" enums:"enforce,audit"`
	IsCurrent       bool             `json:"isCurrent" binding:"required"`
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
