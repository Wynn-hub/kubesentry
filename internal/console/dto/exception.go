package dto

import (
	"bytes"
	"encoding/json"
	"fmt"
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
	PolicyRefs        []string            `json:"policyRefs"`
	PolicyGroupRefs   []string            `json:"policyGroupRefs"`
	AllPolicies       *bool               `json:"allPolicies,omitempty"`
	Match             ExceptionMatchInput `json:"match"`
	Duration          string              `json:"duration"`
	RetainAfterExpiry *string             `json:"retainAfterExpiry,omitempty"`
	Reason            string              `json:"reason"`
	Name              *string             `json:"name,omitempty"`
	ResourceVersion   *string             `json:"resourceVersion,omitempty"`
	AllClusters       *bool               `json:"allClusters,omitempty"`
	ClusterNames      []string            `json:"clusterNames"`
}

func (r *ExceptionInput) UnmarshalJSON(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	if token, err := decoder.Token(); err != nil || token != json.Delim('{') {
		return fmt.Errorf("decode exception request: expected object")
	}
	required := map[string]bool{"match": false, "duration": false, "reason": false}
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
		case "policyRefs":
			r.PolicyRefs = nil
			err = decodeNonNullJSON(raw, "policyRefs", &r.PolicyRefs)
		case "policyGroupRefs":
			r.PolicyGroupRefs = nil
			err = decodeNonNullJSON(raw, "policyGroupRefs", &r.PolicyGroupRefs)
		case "allPolicies":
			err = decodeOptionalNonNullJSON(raw, "allPolicies", &r.AllPolicies)
		case "match":
			required["match"] = true
			err = decodeNonNullJSON(raw, "match", &r.Match)
		case "duration":
			required["duration"] = true
			err = decodeNonNullJSON(raw, "duration", &r.Duration)
		case "retainAfterExpiry":
			err = decodeOptionalNonNullJSON(raw, "retainAfterExpiry", &r.RetainAfterExpiry)
		case "reason":
			required["reason"] = true
			err = decodeNonNullJSON(raw, "reason", &r.Reason)
		case "name":
			err = decodeOptionalNonNullJSON(raw, "name", &r.Name)
		case "resourceVersion":
			err = decodeOptionalNonNullJSON(raw, "resourceVersion", &r.ResourceVersion)
		case "allClusters":
			err = decodeOptionalNonNullJSON(raw, "allClusters", &r.AllClusters)
		case "clusterNames":
			r.ClusterNames = nil
			err = decodeNonNullJSON(raw, "clusterNames", &r.ClusterNames)
		}
		if err != nil {
			return err
		}
	}
	if _, err := decoder.Token(); err != nil {
		return err
	}
	for _, field := range []string{"match", "duration", "reason"} {
		if !required[field] {
			return fmt.Errorf("%s: field is required", field)
		}
	}
	return nil
}

func decodeOptionalNonNullJSON[T any](data json.RawMessage, field string, dst **T) error {
	var value T
	if err := decodeNonNullJSON(data, field, &value); err != nil {
		return err
	}
	*dst = &value
	return nil
}

// ExceptionPath describes the name path parameter shared by exception detail operations.
type ExceptionPath struct {
	// 策略例外名称
	Name string `uri:"name" binding:"required"`
}

type ExceptionMatchInput struct {
	Namespaces        []string           `json:"namespaces,omitempty"`
	NamespaceSelector *LabelSelectorView `json:"namespaceSelector,omitempty"`
	ResourceSelector  *LabelSelectorView `json:"resourceSelector,omitempty"`
}

func (m *ExceptionMatchInput) UnmarshalJSON(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	if token, err := decoder.Token(); err != nil || token != json.Delim('{') {
		return fmt.Errorf("decode exception match: expected object")
	}
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
		case "namespaces":
			m.Namespaces = nil
			err = decodeNonNullJSON(raw, "namespaces", &m.Namespaces)
		case "namespaceSelector":
			err = decodeOptionalNonNullJSON(raw, "namespaceSelector", &m.NamespaceSelector)
		case "resourceSelector":
			err = decodeOptionalNonNullJSON(raw, "resourceSelector", &m.ResourceSelector)
		}
		if err != nil {
			return err
		}
	}
	_, err := decoder.Token()
	return err
}

// SwagExceptionSpec is the Swagger-only representation of the stable exception request JSON shape.
type SwagExceptionSpec struct {
	PolicyRefs        []string            `json:"policyRefs,omitempty"`
	PolicyGroupRefs   []string            `json:"policyGroupRefs,omitempty"`
	AllPolicies       *bool               `json:"allPolicies,omitempty"`
	Match             ExceptionMatchInput `json:"match" binding:"required"`
	Duration          string              `json:"duration" binding:"required"`
	RetainAfterExpiry *string             `json:"retainAfterExpiry,omitempty"`
	Reason            string              `json:"reason" binding:"required"`
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

type ExceptionListHTTPResponse struct {
	Code    int                `json:"code"`
	Message string             `json:"message"`
	Data    *ExceptionPageView `json:"data"`
}

func (*ExceptionListHTTPResponse) listExceptionsRes() {}

type ExceptionDetailHTTPResponse struct {
	Code    int            `json:"code"`
	Message string         `json:"message"`
	Data    *ExceptionView `json:"data"`
}

func (*ExceptionDetailHTTPResponse) getExceptionRes() {}

type ExceptionNameHTTPResponse struct {
	Code    int               `json:"code"`
	Message string            `json:"message"`
	Data    ExceptionNameView `json:"data"`
}

func (*ExceptionNameHTTPResponse) createExceptionRes() {}
func (*ExceptionNameHTTPResponse) updateExceptionRes() {}

type ExceptionEmptyHTTPResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (*ExceptionEmptyHTTPResponse) deleteExceptionRes() {}

type ExceptionBadRequestHTTPError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (*ExceptionBadRequestHTTPError) HTTPStatus() int     { return 400 }
func (*ExceptionBadRequestHTTPError) listExceptionsRes()  {}
func (*ExceptionBadRequestHTTPError) createExceptionRes() {}
func (*ExceptionBadRequestHTTPError) updateExceptionRes() {}

type ExceptionNotFoundHTTPError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (*ExceptionNotFoundHTTPError) HTTPStatus() int     { return 404 }
func (*ExceptionNotFoundHTTPError) getExceptionRes()    {}
func (*ExceptionNotFoundHTTPError) updateExceptionRes() {}
func (*ExceptionNotFoundHTTPError) deleteExceptionRes() {}

type ExceptionConflictHTTPError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (*ExceptionConflictHTTPError) HTTPStatus() int     { return 409 }
func (*ExceptionConflictHTTPError) createExceptionRes() {}
func (*ExceptionConflictHTTPError) updateExceptionRes() {}

type ExceptionInternalServerHTTPError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (*ExceptionInternalServerHTTPError) HTTPStatus() int     { return 500 }
func (*ExceptionInternalServerHTTPError) listExceptionsRes()  {}
func (*ExceptionInternalServerHTTPError) createExceptionRes() {}
func (*ExceptionInternalServerHTTPError) getExceptionRes()    {}
func (*ExceptionInternalServerHTTPError) updateExceptionRes() {}
func (*ExceptionInternalServerHTTPError) deleteExceptionRes() {}
