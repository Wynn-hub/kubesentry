package dto

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

type DistributionClusters struct {
	ByName          []string              `json:"byName,omitempty"`
	ClusterSelector *metav1.LabelSelector `json:"clusterSelector,omitempty"`
}

type DistributionOverride struct {
	Cluster         string `json:"cluster"`
	EnforcementMode string `json:"enforcementMode"`
}

type DistributionSpecView struct {
	Clusters         DistributionClusters   `json:"clusters,omitempty"`
	PolicyGroups     []string               `json:"policyGroups"`
	PolicyExceptions []string               `json:"policyExceptions,omitempty"`
	Overrides        []DistributionOverride `json:"overrides,omitempty"`
}

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
	Name            string               `json:"name"`
	ResourceVersion string               `json:"resourceVersion"`
	Spec            DistributionSpecView `json:"spec"`
}

type DistributionInput struct {
	Name            string               `json:"name,omitempty"`
	Spec            DistributionSpecView `json:"spec"`
	ResourceVersion string               `json:"resourceVersion,omitempty"`
}
