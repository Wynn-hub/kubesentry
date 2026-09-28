package dto

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/Wynn-hub/kubesentry/api/kubesentry/v1alpha1"
)

type PolicyGroupInput struct {
	DisplayName             *string                  `json:"displayName,omitempty"`
	Description             *string                  `json:"description,omitempty"`
	Enabled                 bool                     `json:"enabled"`
	NamespaceSelector       *LabelSelectorView       `json:"namespaceSelector,omitempty"`
	Policies                PolicyGroupPoliciesInput `json:"policies"`
	SelectorEnforcementMode *string                  `json:"selectorEnforcementMode,omitempty"`
	Name                    *string                  `json:"name,omitempty"`
	ResourceVersion         *string                  `json:"resourceVersion,omitempty"`
	AllClusters             *bool                    `json:"allClusters,omitempty"`
	ClusterNames            []string                 `json:"clusterNames"`
}

func (r *PolicyGroupInput) UnmarshalJSON(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	if token, err := decoder.Token(); err != nil || token != json.Delim('{') {
		return fmt.Errorf("decode policy group request: expected object")
	}
	var enabledSet, policiesSet bool
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
		case "displayName":
			err = decodeOptionalNonNullJSON(raw, "displayName", &r.DisplayName)
		case "description":
			err = decodeOptionalNonNullJSON(raw, "description", &r.Description)
		case "enabled":
			enabledSet = true
			err = decodeNonNullJSON(raw, "enabled", &r.Enabled)
		case "namespaceSelector":
			err = decodeOptionalNonNullJSON(raw, "namespaceSelector", &r.NamespaceSelector)
		case "policies":
			policiesSet = true
			err = decodeNonNullJSON(raw, "policies", &r.Policies)
		case "selectorEnforcementMode":
			err = decodeOptionalNonNullJSON(raw, "selectorEnforcementMode", &r.SelectorEnforcementMode)
		case "name":
			err = decodeOptionalNonNullJSON(raw, "name", &r.Name)
		case "resourceVersion":
			err = decodeOptionalNonNullJSON(raw, "resourceVersion", &r.ResourceVersion)
		case "allClusters":
			err = decodeOptionalNonNullJSON(raw, "allClusters", &r.AllClusters)
		case "clusterNames":
			r.ClusterNames = nil
			err = decodeNonNullJSON(raw, "clusterNames", &r.ClusterNames)
		}
		if err != nil {
			return err
		}
	}
	if _, err := decoder.Token(); err != nil {
		return err
	}
	if !enabledSet {
		return fmt.Errorf("enabled: field is required")
	}
	if !policiesSet {
		return fmt.Errorf("policies: field is required")
	}
	if r.SelectorEnforcementMode != nil && *r.SelectorEnforcementMode != "enforce" && *r.SelectorEnforcementMode != "audit" {
		return fmt.Errorf("selectorEnforcementMode: invalid value %q", *r.SelectorEnforcementMode)
	}
	return nil
}

type PolicyRefInput struct {
	Name            string  `json:"name" binding:"required"`
	EnforcementMode *string `json:"enforcementMode,omitempty" enums:"enforce,audit"`
}

func (r *PolicyRefInput) UnmarshalJSON(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	if token, err := decoder.Token(); err != nil || token != json.Delim('{') {
		return fmt.Errorf("decode policy ref: expected object")
	}
	var nameSet bool
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
			nameSet = true
			err = decodeNonNullJSON(raw, "name", &r.Name)
		case "enforcementMode":
			err = decodeOptionalNonNullJSON(raw, "enforcementMode", &r.EnforcementMode)
		}
		if err != nil {
			return err
		}
	}
	if _, err := decoder.Token(); err != nil {
		return err
	}
	if !nameSet {
		return fmt.Errorf("name: field is required")
	}
	if r.EnforcementMode != nil && *r.EnforcementMode != "enforce" && *r.EnforcementMode != "audit" {
		return fmt.Errorf("enforcementMode: invalid value %q", *r.EnforcementMode)
	}
	return nil
}

type PolicyGroupPoliciesInput struct {
	ByName     []PolicyRefInput   `json:"byName,omitempty"`
	BySelector *LabelSelectorView `json:"bySelector,omitempty"`
	Exclude    []string           `json:"exclude,omitempty"`
}

func (p *PolicyGroupPoliciesInput) UnmarshalJSON(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	if token, err := decoder.Token(); err != nil || token != json.Delim('{') {
		return fmt.Errorf("decode policy group policies: expected object")
	}
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
		case "byName":
			p.ByName = nil
			err = decodeNonNullJSON(raw, "byName", &p.ByName)
		case "bySelector":
			err = decodeOptionalNonNullJSON(raw, "bySelector", &p.BySelector)
		case "exclude":
			p.Exclude = nil
			err = decodeNonNullJSON(raw, "exclude", &p.Exclude)
		}
		if err != nil {
			return err
		}
	}
	_, err := decoder.Token()
	return err
}

type PolicyGroupEnabledInput struct {
	Enabled bool `json:"enabled"`
}

func (r *PolicyGroupEnabledInput) UnmarshalJSON(data []byte) error {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(data, &object); err != nil {
		return err
	}
	raw, ok := object["enabled"]
	if !ok {
		return fmt.Errorf("enabled: field is required")
	}
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return fmt.Errorf("enabled: must be a boolean")
	}
	if err := json.Unmarshal(raw, &r.Enabled); err != nil {
		return fmt.Errorf("enabled: %w", err)
	}
	return nil
}

// PolicyGroupPath describes the name path parameter shared by policy group detail operations.
type PolicyGroupPath struct {
	// 策略组名称
	Name string `uri:"name" binding:"required"`
}

// SwagPolicyGroupSpec is the Swagger-only representation of a policy group specification.
type SwagPolicyGroupSpec struct {
	DisplayName             *string                  `json:"displayName,omitempty"`
	Description             *string                  `json:"description,omitempty"`
	Enabled                 bool                     `json:"enabled" binding:"required"`
	NamespaceSelector       *LabelSelectorView       `json:"namespaceSelector,omitempty"`
	Policies                PolicyGroupPoliciesInput `json:"policies" binding:"required"`
	SelectorEnforcementMode *string                  `json:"selectorEnforcementMode,omitempty" enums:"enforce,audit"`
}

// SwagPolicyGroupInput extends the policy group spec with Console-only request fields.
type SwagPolicyGroupInput struct {
	SwagPolicyGroupSpec
	Name            *string  `json:"name,omitempty"`
	ResourceVersion *string  `json:"resourceVersion,omitempty"`
	AllClusters     *bool    `json:"allClusters,omitempty"`
	ClusterNames    []string `json:"clusterNames,omitempty"`
}

// SwagPolicyGroupDetail is the Swagger-only representation of a policy group detail response.
type SwagPolicyGroupDetail struct {
	Name            string                `json:"name" binding:"required"`
	Source          string                `json:"source" binding:"required" enums:"builtin,custom"`
	ResourceVersion string                `json:"resourceVersion" binding:"required"`
	Spec            SwagPolicyGroupSpec   `json:"spec" binding:"required"`
	Status          PolicyGroupStatusView `json:"status" binding:"required"`
	Scope           PolicyGroupScopeView  `json:"scope" binding:"required"`
}

type SwagPolicyGroupName struct {
	Name string `json:"name" binding:"required"`
}

type SwagPolicyGroupEnabledInput struct {
	Enabled bool `json:"enabled" binding:"required"`
}

type SwagPolicyGroupEnabled struct {
	Enabled bool `json:"enabled" binding:"required"`
}

type (
	PolicyGroupScopeView struct {
		AllClusters  bool     `json:"allClusters" binding:"required"`
		ClusterNames []string `json:"clusterNames"`
	}
	EffectiveMemberView struct {
		Name            string `json:"name" binding:"required"`
		EnforcementMode string `json:"enforcementMode" binding:"required" enums:"enforce,audit"`
		Source          string `json:"source" binding:"required" enums:"builtin,custom"`
	}
	PolicyGroupStatusView struct {
		Phase            string                `json:"phase" binding:"required" enums:"Ready,Invalid"`
		ResolvedCount    int                   `json:"resolvedCount" binding:"required"`
		ResolvedPolicies []EffectiveMemberView `json:"resolvedPolicies" binding:"required"`
		Issues           []string              `json:"issues" binding:"required"`
	}
	PolicyGroupView struct {
		Name            string                   `json:"name"`
		Source          string                   `json:"source"`
		ResourceVersion string                   `json:"resourceVersion"`
		Spec            v1alpha1.PolicyGroupSpec `json:"spec"`
		Status          PolicyGroupStatusView    `json:"status"`
		Scope           PolicyGroupScopeView     `json:"scope"`
	}
	PolicyGroupItem struct {
		Name          string `json:"name" binding:"required"`
		Source        string `json:"source" binding:"required" enums:"builtin,custom"`
		DisplayName   string `json:"displayName" binding:"required"`
		Enabled       bool   `json:"enabled" binding:"required"`
		Phase         string `json:"phase" binding:"required" enums:"Ready,Invalid"`
		ResolvedCount int    `json:"resolvedCount" binding:"required"`
	}
	PolicyGroupPageView struct {
		Total    int               `json:"total" binding:"required"`
		Page     int               `json:"page" binding:"required"`
		PageSize int               `json:"pageSize" binding:"required" enums:"10,20,50,100"`
		Items    []PolicyGroupItem `json:"items" binding:"required"`
	}
	PolicyGroupNameView struct {
		Name string `json:"name" binding:"required"`
	}
	PolicyGroupEnabledView struct {
		Enabled bool `json:"enabled" binding:"required"`
	}
)

type PolicyGroupListHTTPResponse struct {
	Code    int                  `json:"code"`
	Message string               `json:"message"`
	Data    *PolicyGroupPageView `json:"data"`
}

func (*PolicyGroupListHTTPResponse) listPolicyGroupsRes() {}

type PolicyGroupDetailHTTPResponse struct {
	Code    int              `json:"code"`
	Message string           `json:"message"`
	Data    *PolicyGroupView `json:"data"`
}

func (*PolicyGroupDetailHTTPResponse) getPolicyGroupRes() {}

type PolicyGroupNameHTTPResponse struct {
	Code    int                 `json:"code"`
	Message string              `json:"message"`
	Data    PolicyGroupNameView `json:"data"`
}

func (*PolicyGroupNameHTTPResponse) createPolicyGroupRes() {}
func (*PolicyGroupNameHTTPResponse) updatePolicyGroupRes() {}

type PolicyGroupEnabledHTTPResponse struct {
	Code    int                    `json:"code"`
	Message string                 `json:"message"`
	Data    PolicyGroupEnabledView `json:"data"`
}

func (*PolicyGroupEnabledHTTPResponse) setPolicyGroupEnabledRes() {}

type PolicyGroupEmptyHTTPResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (*PolicyGroupEmptyHTTPResponse) deletePolicyGroupRes() {}

type PolicyGroupBadRequestHTTPError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (*PolicyGroupBadRequestHTTPError) HTTPStatus() int           { return 400 }
func (*PolicyGroupBadRequestHTTPError) listPolicyGroupsRes()      {}
func (*PolicyGroupBadRequestHTTPError) createPolicyGroupRes()     {}
func (*PolicyGroupBadRequestHTTPError) updatePolicyGroupRes()     {}
func (*PolicyGroupBadRequestHTTPError) setPolicyGroupEnabledRes() {}

type PolicyGroupForbiddenHTTPError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (*PolicyGroupForbiddenHTTPError) HTTPStatus() int           { return 403 }
func (*PolicyGroupForbiddenHTTPError) createPolicyGroupRes()     {}
func (*PolicyGroupForbiddenHTTPError) updatePolicyGroupRes()     {}
func (*PolicyGroupForbiddenHTTPError) setPolicyGroupEnabledRes() {}
func (*PolicyGroupForbiddenHTTPError) deletePolicyGroupRes()     {}

type PolicyGroupNotFoundHTTPError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (*PolicyGroupNotFoundHTTPError) HTTPStatus() int           { return 404 }
func (*PolicyGroupNotFoundHTTPError) getPolicyGroupRes()        {}
func (*PolicyGroupNotFoundHTTPError) updatePolicyGroupRes()     {}
func (*PolicyGroupNotFoundHTTPError) setPolicyGroupEnabledRes() {}
func (*PolicyGroupNotFoundHTTPError) deletePolicyGroupRes()     {}

type PolicyGroupConflictHTTPError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (*PolicyGroupConflictHTTPError) HTTPStatus() int           { return 409 }
func (*PolicyGroupConflictHTTPError) createPolicyGroupRes()     {}
func (*PolicyGroupConflictHTTPError) updatePolicyGroupRes()     {}
func (*PolicyGroupConflictHTTPError) setPolicyGroupEnabledRes() {}

type PolicyGroupInternalServerHTTPError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (*PolicyGroupInternalServerHTTPError) HTTPStatus() int           { return 500 }
func (*PolicyGroupInternalServerHTTPError) listPolicyGroupsRes()      {}
func (*PolicyGroupInternalServerHTTPError) getPolicyGroupRes()        {}
func (*PolicyGroupInternalServerHTTPError) createPolicyGroupRes()     {}
func (*PolicyGroupInternalServerHTTPError) updatePolicyGroupRes()     {}
func (*PolicyGroupInternalServerHTTPError) setPolicyGroupEnabledRes() {}
func (*PolicyGroupInternalServerHTTPError) deletePolicyGroupRes()     {}
