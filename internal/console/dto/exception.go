package dto

import (
	"time"

	"github.com/Wynn-hub/kubesentry/internal/api/v1alpha1"
)

type ExceptionScopeView struct {
	AllClusters  bool     `json:"allClusters"`
	ClusterNames []string `json:"clusterNames"`
}

type ExceptionStatusView struct {
	Phase       string     `json:"phase"`
	EffectiveAt time.Time  `json:"effectiveAt"`
	ExpiresAt   *time.Time `json:"expiresAt,omitempty"`
}

type ExceptionView struct {
	Name            string                       `json:"name"`
	ResourceVersion string                       `json:"resourceVersion"`
	Spec            v1alpha1.PolicyExceptionSpec `json:"spec"`
	Status          ExceptionStatusView          `json:"status"`
	Scope           ExceptionScopeView           `json:"scope"`
}

type ExceptionItem struct {
	Name          string     `json:"name"`
	Phase         string     `json:"phase"`
	Reason        string     `json:"reason"`
	Duration      string     `json:"duration"`
	TargetSummary string     `json:"targetSummary"`
	ExpiresAt     *time.Time `json:"expiresAt,omitempty"`
}

type ExceptionPageView struct {
	Total    int             `json:"total"`
	Page     int             `json:"page"`
	PageSize int             `json:"pageSize"`
	Items    []ExceptionItem `json:"items"`
}

type ExceptionInput struct {
	PolicyRefs        []string                      `json:"policyRefs"`
	PolicyGroupRefs   []string                      `json:"policyGroupRefs"`
	AllPolicies       *bool                         `json:"allPolicies,omitempty"`
	Match             v1alpha1.PolicyExceptionMatch `json:"match"`
	Duration          string                        `json:"duration"`
	RetainAfterExpiry *string                       `json:"retainAfterExpiry,omitempty"`
	Reason            string                        `json:"reason"`
	Name              *string                       `json:"name,omitempty"`
	ResourceVersion   *string                       `json:"resourceVersion,omitempty"`
	AllClusters       *bool                         `json:"allClusters,omitempty"`
	ClusterNames      []string                      `json:"clusterNames"`
}
