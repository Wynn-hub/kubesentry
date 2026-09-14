package dto

type DistributionClusters struct {
	ByName          []string           `json:"byName,omitempty"`
	ClusterSelector *LabelSelectorView `json:"clusterSelector,omitempty"`
}

type DistributionOverride struct {
	Cluster         string `json:"cluster" binding:"required"`
	EnforcementMode string `json:"enforcementMode" binding:"required" enums:"audit,enforce"`
}

type DistributionSpecView struct {
	Clusters         DistributionClusters   `json:"clusters,omitempty"`
	PolicyGroups     []string               `json:"policyGroups" binding:"required"`
	PolicyExceptions []string               `json:"policyExceptions,omitempty"`
	Overrides        []DistributionOverride `json:"overrides,omitempty"`
}

type DistributionItem struct {
	Name         string   `json:"name" binding:"required"`
	PolicyGroups []string `json:"policyGroups" binding:"required"`
}

type DistributionPageView struct {
	Items    []DistributionItem `json:"items" binding:"required"`
	Total    int                `json:"total" binding:"required"`
	Page     int                `json:"page" binding:"required"`
	PageSize int                `json:"pageSize" binding:"required" enums:"10,20,50,100"`
}

type DistributionView struct {
	Name            string               `json:"name" binding:"required"`
	ResourceVersion string               `json:"resourceVersion" binding:"required"`
	Spec            DistributionSpecView `json:"spec" binding:"required"`
}

type DistributionInput struct {
	Name            string               `json:"name,omitempty"`
	Spec            DistributionSpecView `json:"spec" binding:"required"`
	ResourceVersion string               `json:"resourceVersion,omitempty"`
}

type DistributionNameView struct {
	Name string `json:"name" binding:"required"`
}
