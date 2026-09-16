package dto

import (
	"time"

	"github.com/Wynn-hub/kubesentry/internal/api/v1alpha1"
)

type ExceptionScopeView struct {
	AllClusters  bool     `json:"allClusters" binding:"required"`
	ClusterNames []string `json:"clusterNames"`
}

type ExceptionStatusView struct {
	Phase       string     `json:"phase" binding:"required" enums:"Active,Expired"`
	EffectiveAt time.Time  `json:"effectiveAt" binding:"required" format:"date-time"`
	ExpiresAt   *time.Time `json:"expiresAt,omitempty" format:"date-time"`
}

type ExceptionView struct {
	Name            string                       `json:"name" binding:"required"`
	ResourceVersion string                       `json:"resourceVersion" binding:"required"`
	Spec            v1alpha1.PolicyExceptionSpec `json:"spec"`
	Status          ExceptionStatusView          `json:"status" binding:"required"`
	Scope           ExceptionScopeView           `json:"scope" binding:"required"`
}

type ExceptionItem struct {
	Name          string     `json:"name" binding:"required"`
	Phase         string     `json:"phase" binding:"required" enums:"Active,Expired"`
	Reason        string     `json:"reason" binding:"required"`
	Duration      string     `json:"duration" binding:"required"`
	TargetSummary string     `json:"targetSummary" binding:"required"`
	ExpiresAt     *time.Time `json:"expiresAt,omitempty" format:"date-time"`
}

type ExceptionPageView struct {
	Total    int             `json:"total" binding:"required"`
	Page     int             `json:"page" binding:"required"`
	PageSize int             `json:"pageSize" binding:"required" enums:"10,20,50,100"`
	Items    []ExceptionItem `json:"items" binding:"required"`
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

// ExceptionPath describes the name path parameter shared by exception detail operations.
type ExceptionPath struct {
	// 策略例外名称
	Name string `uri:"name" binding:"required"`
}

// SwagExceptionMatch is the Swagger-only representation of the stable exception match JSON shape.
type SwagExceptionMatch struct {
	Namespaces        []string           `json:"namespaces,omitempty"`
	NamespaceSelector *LabelSelectorView `json:"namespaceSelector,omitempty"`
	ResourceSelector  *LabelSelectorView `json:"resourceSelector,omitempty"`
}

// SwagExceptionSpec is the Swagger-only representation of the stable exception request JSON shape.
type SwagExceptionSpec struct {
	PolicyRefs        []string           `json:"policyRefs,omitempty"`
	PolicyGroupRefs   []string           `json:"policyGroupRefs,omitempty"`
	AllPolicies       *bool              `json:"allPolicies,omitempty"`
	Match             SwagExceptionMatch `json:"match" binding:"required"`
	Duration          string             `json:"duration" binding:"required"`
	RetainAfterExpiry *string            `json:"retainAfterExpiry,omitempty"`
	Reason            string             `json:"reason" binding:"required"`
}

// SwagExceptionInput extends the exception spec with Console-only request fields.
type SwagExceptionInput struct {
	SwagExceptionSpec
	Name            *string  `json:"name,omitempty"`
	ResourceVersion *string  `json:"resourceVersion,omitempty"`
	AllClusters     *bool    `json:"allClusters,omitempty"`
	ClusterNames    []string `json:"clusterNames"`
}

// SwagExceptionDetail is the Swagger-only representation of the stable exception detail JSON shape.
type SwagExceptionDetail struct {
	Name            string              `json:"name" binding:"required"`
	ResourceVersion string              `json:"resourceVersion" binding:"required"`
	Spec            SwagExceptionSpec   `json:"spec" binding:"required"`
	Status          ExceptionStatusView `json:"status" binding:"required"`
	Scope           ExceptionScopeView  `json:"scope" binding:"required"`
}

type ExceptionNameView struct {
	Name string `json:"name" binding:"required"`
}
