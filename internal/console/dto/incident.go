package dto

import "time"

type IncidentListOptions struct {
	Page, PageSize                int
	Clusters, Components, Reasons []string
	Statuses                      []string
	From, To                      *time.Time
}

type IncidentItem struct {
	ID                 string    `json:"id"`
	ClusterName        string    `json:"clusterName"`
	ReporterInstanceID string    `json:"reporterInstanceId"`
	Component          string    `json:"component"`
	Reason             string    `json:"reason"`
	StartedAt          time.Time `json:"startedAt"`
	LastSeenAt         time.Time `json:"lastSeenAt"`
	AffectedCount      int64     `json:"affectedCount"`
	Status             string    `json:"status"`
}

type IncidentPageView struct {
	Total    int            `json:"total"`
	Page     int            `json:"page"`
	PageSize int            `json:"pageSize"`
	Items    []IncidentItem `json:"items"`
}

type IncidentSampleView struct {
	EventID         string  `json:"eventId"`
	EventDatabaseID *string `json:"eventDatabaseId,omitempty"`
	Expired         bool    `json:"expired"`
}

type IncidentView struct {
	IncidentItem
	Detail  string               `json:"detail"`
	Samples []IncidentSampleView `json:"samples"`
}
