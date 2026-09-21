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
	ID           string    `json:"id" binding:"required" pattern:"^[0-9]+$"`
	EventID      string    `json:"eventId" binding:"required"`
	ClusterName  string    `json:"clusterName" binding:"required"`
	OccurredAt   time.Time `json:"occurredAt" binding:"required" format:"date-time"`
	Verdict      string    `json:"verdict" binding:"required" enums:"audit,deny"`
	APIGroup     string    `json:"apiGroup" binding:"required"`
	APIVersion   string    `json:"apiVersion" binding:"required"`
	Resource     string    `json:"resource" binding:"required"`
	Operation    string    `json:"operation" binding:"required"`
	Namespace    string    `json:"namespace" binding:"required"`
	ResourceName string    `json:"resourceName" binding:"required"`
	Username     string    `json:"username" binding:"required"`
	PolicyNames  []string  `json:"policyNames" binding:"required"`
	Violations   []string  `json:"violations" binding:"required"`
}

type AdmissionEventPageView struct {
	Total    int                      `json:"total" binding:"required"`
	Page     int                      `json:"page" binding:"required"`
	PageSize int                      `json:"pageSize" binding:"required" enums:"10,20,50,100"`
	Items    []AdmissionEventItemView `json:"items" binding:"required"`
}

type AdmissionEventDetailView struct {
	AdmissionEventItemView
	RequestUID           string                     `json:"requestUid" binding:"required"`
	UserGroups           []string                   `json:"userGroups" binding:"required"`
	PolicyGroupNames     []string                   `json:"policyGroupNames" binding:"required"`
	MetadataSnapshot     map[string]json.RawMessage `json:"metadataSnapshot" binding:"required" swaggertype:"object"`
	SpecSnapshot         map[string]json.RawMessage `json:"specSnapshot" binding:"required" swaggertype:"object"`
	StoragePolicyVersion string                     `json:"storagePolicyVersion" binding:"required"`
}

type AdmissionEventPath struct {
	// 准入事件数据库 ID
	ID string `uri:"id" binding:"required" pattern:"^[0-9]+$"`
}

type AdmissionStoragePolicyView struct {
	Verdicts   []string `json:"verdicts" binding:"required" enums:"audit,deny"`
	Clusters   []string `json:"clusters" binding:"required"`
	GVRs       []string `json:"gvks" binding:"required"`
	Namespaces []string `json:"namespaces" binding:"required"`
	MatchMode  string   `json:"matchMode" binding:"required" enums:"include,exclude"`
}

type AdmissionSettingsView struct {
	DefaultRetentionDays  int                          `json:"defaultRetentionDays" binding:"required" minimum:"1" maximum:"365"`
	RetryEnabled          bool                         `json:"retryEnabled" binding:"required"`
	MaxRetryAttempts      int                          `json:"maxRetryAttempts" binding:"required" minimum:"1" maximum:"100"`
	QueueCapacity         int                          `json:"queueCapacity" binding:"required" minimum:"1" maximum:"10000"`
	IncidentRetentionDays int                          `json:"incidentRetentionDays" binding:"required" minimum:"1" maximum:"365"`
	ObservationEnabled    bool                         `json:"observationEnabled" binding:"required"`
	StoragePolicyVersion  int64                        `json:"storagePolicyVersion" binding:"required" format:"int64"`
	StoragePolicies       []AdmissionStoragePolicyView `json:"storagePolicies" binding:"required"`
}

type AdmissionSettingsInput struct {
	DefaultRetentionDays  int                          `json:"defaultRetentionDays" binding:"required" minimum:"1" maximum:"365"`
	RetryEnabled          *bool                        `json:"retryEnabled,omitempty"`
	MaxRetryAttempts      *int                         `json:"maxRetryAttempts,omitempty" minimum:"1" maximum:"100"`
	QueueCapacity         *int                         `json:"queueCapacity,omitempty" minimum:"1" maximum:"10000"`
	IncidentRetentionDays *int                         `json:"incidentRetentionDays,omitempty" minimum:"1" maximum:"365"`
	ObservationEnabled    *bool                        `json:"observationEnabled,omitempty"`
	StoragePolicies       []AdmissionStoragePolicyView `json:"storagePolicies"`
}

type AdmissionTrendItemView struct {
	Bucket  time.Time `json:"bucket" binding:"required" format:"date-time"`
	Verdict string    `json:"verdict" binding:"required" enums:"audit,deny"`
	Count   int64     `json:"count" binding:"required" format:"int64"`
}

type AdmissionEventListHTTPResponse struct {
	Code    int                     `json:"code"`
	Message string                  `json:"message"`
	Data    *AdmissionEventPageView `json:"data"`
}

func (*AdmissionEventListHTTPResponse) listAdmissionEventsRes() {}

type AdmissionEventDetailHTTPResponse struct {
	Code    int                       `json:"code"`
	Message string                    `json:"message"`
	Data    *AdmissionEventDetailView `json:"data"`
}

func (*AdmissionEventDetailHTTPResponse) getAdmissionEventRes() {}

type AdmissionEventTrendHTTPResponse struct {
	Code    int                      `json:"code"`
	Message string                   `json:"message"`
	Data    []AdmissionTrendItemView `json:"data"`
}

func (*AdmissionEventTrendHTTPResponse) getAdmissionEventTrendRes() {}

type AdmissionEventSettingsHTTPResponse struct {
	Code    int                    `json:"code"`
	Message string                 `json:"message"`
	Data    *AdmissionSettingsView `json:"data"`
}

func (*AdmissionEventSettingsHTTPResponse) getAdmissionEventSettingsRes()    {}
func (*AdmissionEventSettingsHTTPResponse) updateAdmissionEventSettingsRes() {}

type AdmissionEventBadRequestHTTPError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (*AdmissionEventBadRequestHTTPError) HTTPStatus() int                  { return 400 }
func (*AdmissionEventBadRequestHTTPError) listAdmissionEventsRes()          {}
func (*AdmissionEventBadRequestHTTPError) getAdmissionEventRes()            {}
func (*AdmissionEventBadRequestHTTPError) getAdmissionEventTrendRes()       {}
func (*AdmissionEventBadRequestHTTPError) updateAdmissionEventSettingsRes() {}

type AdmissionEventNotFoundHTTPError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (*AdmissionEventNotFoundHTTPError) HTTPStatus() int       { return 404 }
func (*AdmissionEventNotFoundHTTPError) getAdmissionEventRes() {}

type AdmissionEventInternalServerHTTPError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (*AdmissionEventInternalServerHTTPError) HTTPStatus() int                  { return 500 }
func (*AdmissionEventInternalServerHTTPError) listAdmissionEventsRes()          {}
func (*AdmissionEventInternalServerHTTPError) getAdmissionEventRes()            {}
func (*AdmissionEventInternalServerHTTPError) getAdmissionEventTrendRes()       {}
func (*AdmissionEventInternalServerHTTPError) getAdmissionEventSettingsRes()    {}
func (*AdmissionEventInternalServerHTTPError) updateAdmissionEventSettingsRes() {}

type AdmissionEventServiceUnavailableHTTPError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (*AdmissionEventServiceUnavailableHTTPError) HTTPStatus() int               { return 503 }
func (*AdmissionEventServiceUnavailableHTTPError) getAdmissionEventSettingsRes() {}
