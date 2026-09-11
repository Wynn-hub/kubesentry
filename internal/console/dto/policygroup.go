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

type (
	PolicyGroupScopeView struct {
		AllClusters  bool     `json:"allClusters"`
		ClusterNames []string `json:"clusterNames"`
	}
	EffectiveMemberView struct {
		Name            string `json:"name"`
		EnforcementMode string `json:"enforcementMode"`
		Source          string `json:"source"`
	}
	PolicyGroupStatusView struct {
		Phase            string                `json:"phase"`
		ResolvedCount    int                   `json:"resolvedCount"`
		ResolvedPolicies []EffectiveMemberView `json:"resolvedPolicies"`
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
		Name          string `json:"name"`
		Source        string `json:"source"`
		DisplayName   string `json:"displayName"`
		Enabled       bool   `json:"enabled"`
		Phase         string `json:"phase"`
		ResolvedCount int    `json:"resolvedCount"`
	}
	PolicyGroupPageView struct {
		Total    int               `json:"total"`
		Page     int               `json:"page"`
		PageSize int               `json:"pageSize"`
		Items    []PolicyGroupItem `json:"items"`
	}
)
