package dto

import (
	"bytes"
	"encoding/json"
	"fmt"
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

func (p *AdmissionStoragePolicyView) UnmarshalJSON(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	if token, err := decoder.Token(); err != nil || token != json.Delim('{') {
		return fmt.Errorf("decode admission storage policy: expected object")
	}
	required := map[string]bool{"verdicts": false, "clusters": false, "gvks": false, "namespaces": false, "matchMode": false}
	for decoder.More() {
		key, err := decoder.Token()
		if err != nil {
			return err
		}
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil {
			return err
		}
		switch key {
		case "verdicts":
			required["verdicts"] = true
			p.Verdicts = nil
			err = decodeNonNullJSON(raw, "verdicts", &p.Verdicts)
		case "clusters":
			required["clusters"] = true
			p.Clusters = nil
			err = decodeNonNullJSON(raw, "clusters", &p.Clusters)
		case "gvks":
			required["gvks"] = true
			p.GVRs = nil
			err = decodeNonNullJSON(raw, "gvks", &p.GVRs)
		case "namespaces":
			required["namespaces"] = true
			p.Namespaces = nil
			err = decodeNonNullJSON(raw, "namespaces", &p.Namespaces)
		case "matchMode":
			required["matchMode"] = true
			err = decodeNonNullJSON(raw, "matchMode", &p.MatchMode)
		}
		if err != nil {
			return err
		}
	}
	if _, err := decoder.Token(); err != nil {
		return err
	}
	for _, field := range []string{"verdicts", "clusters", "gvks", "namespaces", "matchMode"} {
		if !required[field] {
			return fmt.Errorf("%s: field is required", field)
		}
	}
	for _, verdict := range p.Verdicts {
		if verdict != "audit" && verdict != "deny" {
			return fmt.Errorf("verdicts: invalid value %q", verdict)
		}
	}
	if p.MatchMode != "include" && p.MatchMode != "exclude" {
		return fmt.Errorf("matchMode: invalid value %q", p.MatchMode)
	}
	return nil
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

func (s *AdmissionSettingsInput) UnmarshalJSON(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	if token, err := decoder.Token(); err != nil || token != json.Delim('{') {
		return fmt.Errorf("decode admission settings: expected object")
	}
	var retentionSet bool
	for decoder.More() {
		key, err := decoder.Token()
		if err != nil {
			return err
		}
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil {
			return err
		}
		switch key {
		case "defaultRetentionDays":
			retentionSet = true
			err = decodeNonNullJSON(raw, "defaultRetentionDays", &s.DefaultRetentionDays)
		case "retryEnabled":
			err = decodeOptionalNonNullJSON(raw, "retryEnabled", &s.RetryEnabled)
		case "maxRetryAttempts":
			err = decodeOptionalNonNullJSON(raw, "maxRetryAttempts", &s.MaxRetryAttempts)
		case "queueCapacity":
			err = decodeOptionalNonNullJSON(raw, "queueCapacity", &s.QueueCapacity)
		case "incidentRetentionDays":
			err = decodeOptionalNonNullJSON(raw, "incidentRetentionDays", &s.IncidentRetentionDays)
		case "observationEnabled":
			err = decodeOptionalNonNullJSON(raw, "observationEnabled", &s.ObservationEnabled)
		case "storagePolicies":
			s.StoragePolicies = nil
			err = decodeNonNullJSON(raw, "storagePolicies", &s.StoragePolicies)
		}
		if err != nil {
			return err
		}
	}
	if _, err := decoder.Token(); err != nil {
		return err
	}
	if !retentionSet {
		return fmt.Errorf("defaultRetentionDays: field is required")
	}
	return nil
}

func (s *AdmissionSettingsInput) Validate() error {
	if s.DefaultRetentionDays < 1 || s.DefaultRetentionDays > 365 {
		return fmt.Errorf("defaultRetentionDays must be between 1 and 365")
	}
	if s.MaxRetryAttempts != nil && (*s.MaxRetryAttempts < 1 || *s.MaxRetryAttempts > 100) {
		return fmt.Errorf("maxRetryAttempts must be between 1 and 100")
	}
	if s.QueueCapacity != nil && (*s.QueueCapacity < 1 || *s.QueueCapacity > 10000) {
		return fmt.Errorf("queueCapacity must be between 1 and 10000")
	}
	if s.IncidentRetentionDays != nil && (*s.IncidentRetentionDays < 1 || *s.IncidentRetentionDays > 365) {
		return fmt.Errorf("incidentRetentionDays must be between 1 and 365")
	}
	return nil
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
