package dto

import "github.com/Wynn-hub/kubesentry/internal/gatewaymodel"

type DistributionItem struct {
	Name         string   `json:"name"`
	PolicyGroups []string `json:"policyGroups"`
}

type DistributionPageView struct {
	Items    []DistributionItem `json:"items"`
	Total    int                `json:"total"`
	Page     int                `json:"page"`
	PageSize int                `json:"pageSize"`
}

type DistributionView struct {
	Name            string                              `json:"name"`
	ResourceVersion string                              `json:"resourceVersion"`
	Spec            gatewaymodel.PolicyDistributionSpec `json:"spec"`
}

type DistributionInput struct {
	Name            string                              `json:"name,omitempty"`
	Spec            gatewaymodel.PolicyDistributionSpec `json:"spec"`
	ResourceVersion string                              `json:"resourceVersion,omitempty"`
}
