package dto

import "time"

type IncidentListOptions struct {
	Page, PageSize                int
	Clusters, Components, Reasons []string
	Statuses                      []string
	From, To                      *time.Time
}

type IncidentItem struct {
	ID                 string    `json:"id" binding:"required" pattern:"^[0-9]+$"`
	ClusterName        string    `json:"clusterName" binding:"required"`
	ReporterInstanceID string    `json:"reporterInstanceId" binding:"required"`
	Component          string    `json:"component" binding:"required" enums:"webhook,operator,gateway"`
	Reason             string    `json:"reason" binding:"required"`
	StartedAt          time.Time `json:"startedAt" binding:"required" format:"date-time"`
	LastSeenAt         time.Time `json:"lastSeenAt" binding:"required" format:"date-time"`
	AffectedCount      int64     `json:"affectedCount" binding:"required" format:"int64"`
	Status             string    `json:"status" binding:"required" enums:"active,interrupted,closed"`
}

type IncidentPageView struct {
	Total    int            `json:"total" binding:"required"`
	Page     int            `json:"page" binding:"required"`
	PageSize int            `json:"pageSize" binding:"required" enums:"10,20,50,100"`
	Items    []IncidentItem `json:"items" binding:"required"`
}

type IncidentSampleView struct {
	EventID         string  `json:"eventId" binding:"required"`
	EventDatabaseID *string `json:"eventDatabaseId,omitempty" pattern:"^[0-9]+$"`
	Expired         bool    `json:"expired" binding:"required"`
}

type IncidentView struct {
	IncidentItem
	Detail  string               `json:"detail" binding:"required"`
	Samples []IncidentSampleView `json:"samples" binding:"required"`
}

type AdmissionLinkIncidentPath struct {
	// 准入链路异常记录 ID
	ID string `uri:"id" binding:"required" pattern:"^[0-9]+$"`
}

type AdmissionLinkIncidentListHTTPResponse struct {
	Code    int               `json:"code"`
	Message string            `json:"message"`
	Data    *IncidentPageView `json:"data"`
}

func (*AdmissionLinkIncidentListHTTPResponse) listAdmissionLinkIncidentsRes() {}

type AdmissionLinkIncidentDetailHTTPResponse struct {
	Code    int           `json:"code"`
	Message string        `json:"message"`
	Data    *IncidentView `json:"data"`
}

func (*AdmissionLinkIncidentDetailHTTPResponse) getAdmissionLinkIncidentRes() {}

type AdmissionLinkIncidentBadRequestHTTPError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (*AdmissionLinkIncidentBadRequestHTTPError) HTTPStatus() int                { return 400 }
func (*AdmissionLinkIncidentBadRequestHTTPError) listAdmissionLinkIncidentsRes() {}
func (*AdmissionLinkIncidentBadRequestHTTPError) getAdmissionLinkIncidentRes()   {}

type AdmissionLinkIncidentNotFoundHTTPError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (*AdmissionLinkIncidentNotFoundHTTPError) HTTPStatus() int              { return 404 }
func (*AdmissionLinkIncidentNotFoundHTTPError) getAdmissionLinkIncidentRes() {}

type AdmissionLinkIncidentInternalServerHTTPError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (*AdmissionLinkIncidentInternalServerHTTPError) HTTPStatus() int                { return 500 }
func (*AdmissionLinkIncidentInternalServerHTTPError) listAdmissionLinkIncidentsRes() {}
func (*AdmissionLinkIncidentInternalServerHTTPError) getAdmissionLinkIncidentRes()   {}

type AdmissionLinkIncidentServiceUnavailableHTTPError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (*AdmissionLinkIncidentServiceUnavailableHTTPError) HTTPStatus() int                { return 503 }
func (*AdmissionLinkIncidentServiceUnavailableHTTPError) listAdmissionLinkIncidentsRes() {}
func (*AdmissionLinkIncidentServiceUnavailableHTTPError) getAdmissionLinkIncidentRes()   {}
