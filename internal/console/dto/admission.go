package dto

import (
	"encoding/json"
	"time"
)

type AdmissionEventListOptions struct {
	Page, PageSize                     int
	SortBy, SortOrder                  string
	Clusters, Verdicts, Namespaces     []string
	GVRs                               []string
	PolicyName, ResourceName, Username string
	From, To                           *time.Time
}

type AdmissionEventItemView struct {
	ID           string    `json:"id"`
	EventID      string    `json:"eventId"`
	ClusterName  string    `json:"clusterName"`
	OccurredAt   time.Time `json:"occurredAt"`
	Verdict      string    `json:"verdict"`
	APIGroup     string    `json:"apiGroup"`
	APIVersion   string    `json:"apiVersion"`
	Resource     string    `json:"resource"`
	Operation    string    `json:"operation"`
	Namespace    string    `json:"namespace"`
	ResourceName string    `json:"resourceName"`
	Username     string    `json:"username"`
	PolicyNames  []string  `json:"policyNames"`
	Violations   []string  `json:"violations"`
}

type AdmissionEventPageView struct {
	Total    int                      `json:"total"`
	Page     int                      `json:"page"`
	PageSize int                      `json:"pageSize"`
	Items    []AdmissionEventItemView `json:"items"`
}

type AdmissionEventDetailView struct {
	AdmissionEventItemView
	RequestUID           string                     `json:"requestUid"`
	UserGroups           []string                   `json:"userGroups"`
	PolicyGroupNames     []string                   `json:"policyGroupNames"`
	MetadataSnapshot     map[string]json.RawMessage `json:"metadataSnapshot"`
	SpecSnapshot         map[string]json.RawMessage `json:"specSnapshot"`
	StoragePolicyVersion string                     `json:"storagePolicyVersion"`
}

type AdmissionStoragePolicyView struct {
	Verdicts   []string `json:"verdicts"`
	Clusters   []string `json:"clusters"`
	GVRs       []string `json:"gvks"`
	Namespaces []string `json:"namespaces"`
	MatchMode  string   `json:"matchMode"`
}

type AdmissionSettingsView struct {
	DefaultRetentionDays  int                          `json:"defaultRetentionDays"`
	RetryEnabled          bool                         `json:"retryEnabled"`
	MaxRetryAttempts      int                          `json:"maxRetryAttempts"`
	QueueCapacity         int                          `json:"queueCapacity"`
	IncidentRetentionDays int                          `json:"incidentRetentionDays"`
	ObservationEnabled    bool                         `json:"observationEnabled"`
	StoragePolicyVersion  int64                        `json:"storagePolicyVersion"`
	StoragePolicies       []AdmissionStoragePolicyView `json:"storagePolicies"`
}

type AdmissionSettingsInput struct {
	DefaultRetentionDays  int                          `json:"defaultRetentionDays"`
	RetryEnabled          *bool                        `json:"retryEnabled,omitempty"`
	MaxRetryAttempts      *int                         `json:"maxRetryAttempts,omitempty"`
	QueueCapacity         *int                         `json:"queueCapacity,omitempty"`
	IncidentRetentionDays *int                         `json:"incidentRetentionDays,omitempty"`
	ObservationEnabled    *bool                        `json:"observationEnabled,omitempty"`
	StoragePolicies       []AdmissionStoragePolicyView `json:"storagePolicies"`
}

type AdmissionTrendItemView struct {
	Bucket  time.Time `json:"bucket"`
	Verdict string    `json:"verdict"`
	Count   int64     `json:"count"`
}
