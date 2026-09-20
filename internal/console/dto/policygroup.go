package dto

import (
	"github.com/Wynn-hub/kubesentry/internal/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type PolicyGroupInput struct {
	DisplayName             *string                      `json:"displayName,omitempty"`
	Description             *string                      `json:"description,omitempty"`
	Enabled                 bool                         `json:"enabled"`
	NamespaceSelector       *metav1.LabelSelector        `json:"namespaceSelector,omitempty"`
	Policies                v1alpha1.PolicyGroupPolicies `json:"policies"`
	SelectorEnforcementMode *string                      `json:"selectorEnforcementMode,omitempty"`
	Name                    *string                      `json:"name,omitempty"`
	ResourceVersion         *string                      `json:"resourceVersion,omitempty"`
	AllClusters             *bool                        `json:"allClusters,omitempty"`
	ClusterNames            []string                     `json:"clusterNames"`
}

// PolicyGroupPath describes the name path parameter shared by policy group detail operations.
type PolicyGroupPath struct {
	// 策略组名称
	Name string `uri:"name" binding:"required"`
}

// SwagPolicyRef describes a policy reference with an optional enforcement override.
type SwagPolicyRef struct {
	Name            string `json:"name" binding:"required"`
	EnforcementMode string `json:"enforcementMode,omitempty" enums:"enforce,audit"`
}

// SwagPolicyGroupPolicies describes the policy selection rules for a policy group.
type SwagPolicyGroupPolicies struct {
	ByName     []SwagPolicyRef    `json:"byName,omitempty"`
	BySelector *LabelSelectorView `json:"bySelector,omitempty"`
	Exclude    []string           `json:"exclude,omitempty"`
}

// SwagPolicyGroupSpec is the Swagger-only representation of a policy group specification.
type SwagPolicyGroupSpec struct {
	DisplayName             *string                 `json:"displayName,omitempty"`
	Description             *string                 `json:"description,omitempty"`
	Enabled                 bool                    `json:"enabled" binding:"required"`
	NamespaceSelector       *LabelSelectorView      `json:"namespaceSelector,omitempty"`
	Policies                SwagPolicyGroupPolicies `json:"policies" binding:"required"`
	SelectorEnforcementMode *string                 `json:"selectorEnforcementMode,omitempty" enums:"enforce,audit"`
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

func (*PolicyGroupForbiddenHTTPError) HTTPStatus() int       { return 403 }
func (*PolicyGroupForbiddenHTTPError) updatePolicyGroupRes() {}
func (*PolicyGroupForbiddenHTTPError) deletePolicyGroupRes() {}

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
