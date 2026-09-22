package dto

import (
	"bytes"
	"encoding/json"
	"fmt"
)

type DistributionClusters struct {
	ByName          []string           `json:"byName,omitempty"`
	ClusterSelector *LabelSelectorView `json:"clusterSelector,omitempty"`
}

func (c *DistributionClusters) UnmarshalJSON(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	if token, err := decoder.Token(); err != nil || token != json.Delim('{') {
		return fmt.Errorf("decode distribution clusters: expected object")
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
		case "byName":
			c.ByName = nil
			err = decodeNonNullJSON(raw, "byName", &c.ByName)
		case "clusterSelector":
			c.ClusterSelector = nil
			err = decodeNonNullJSON(raw, "clusterSelector", &c.ClusterSelector)
		}
		if err != nil {
			return err
		}
	}
	_, err := decoder.Token()
	return err
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

func (d *DistributionInput) UnmarshalJSON(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	if token, err := decoder.Token(); err != nil || token != json.Delim('{') {
		return fmt.Errorf("decode distribution request: expected object")
	}
	var specSet bool
	for decoder.More() {
		key, err := decoder.Token()
		if err != nil {
			return err
		}
		switch key {
		case "name":
			var raw json.RawMessage
			if err = decoder.Decode(&raw); err == nil {
				err = decodeNonNullJSON(raw, "name", &d.Name)
			}
		case "resourceVersion":
			var raw json.RawMessage
			if err = decoder.Decode(&raw); err == nil {
				err = decodeNonNullJSON(raw, "resourceVersion", &d.ResourceVersion)
			}
		case "spec":
			var raw json.RawMessage
			if err = decoder.Decode(&raw); err == nil {
				specSet = true
				err = decodeDistributionSpec(raw, &d.Spec)
			}
		default:
			var ignored json.RawMessage
			err = decoder.Decode(&ignored)
		}
		if err != nil {
			return fmt.Errorf("decode field %q: %w", key, err)
		}
	}
	if _, err := decoder.Token(); err != nil {
		return err
	}
	if !specSet {
		return fmt.Errorf("field \"spec\" is required")
	}
	return nil
}

func decodeDistributionSpec(data []byte, result *DistributionSpecView) error {
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return fmt.Errorf("field \"spec\" is required")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	if token, err := decoder.Token(); err != nil || token != json.Delim('{') {
		return fmt.Errorf("expected object")
	}
	var policyGroupsSet bool
	var decoded DistributionSpecView
	for decoder.More() {
		key, err := decoder.Token()
		if err != nil {
			return err
		}
		switch key {
		case "clusters":
			decoded.Clusters = DistributionClusters{}
			var raw json.RawMessage
			if err = decoder.Decode(&raw); err == nil {
				err = decodeNonNullJSON(raw, "clusters", &decoded.Clusters)
			}
		case "policyGroups":
			policyGroupsSet = true
			err = decoder.Decode(&decoded.PolicyGroups)
		case "policyExceptions":
			var raw json.RawMessage
			if err = decoder.Decode(&raw); err == nil {
				err = decodeNonNullJSON(raw, "policyExceptions", &decoded.PolicyExceptions)
			}
		case "overrides":
			var raw json.RawMessage
			if err = decoder.Decode(&raw); err == nil {
				err = decodeNonNullJSON(raw, "overrides", &decoded.Overrides)
			}
		default:
			var ignored json.RawMessage
			err = decoder.Decode(&ignored)
		}
		if err != nil {
			return fmt.Errorf("decode field %q: %w", key, err)
		}
	}
	if _, err := decoder.Token(); err != nil {
		return err
	}
	if !policyGroupsSet || decoded.PolicyGroups == nil {
		return fmt.Errorf("field \"policyGroups\" is required")
	}
	*result = decoded
	return nil
}

func (o *DistributionOverride) UnmarshalJSON(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	if token, err := decoder.Token(); err != nil || token != json.Delim('{') {
		return fmt.Errorf("decode distribution override: expected object")
	}
	var clusterSet, enforcementModeSet bool
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
		case "cluster":
			clusterSet = true
			if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
				return fmt.Errorf("cluster: must be a string")
			}
			if err := json.Unmarshal(raw, &o.Cluster); err != nil {
				return fmt.Errorf("cluster: %w", err)
			}
		case "enforcementMode":
			enforcementModeSet = true
			if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
				return fmt.Errorf("enforcementMode: must be a string")
			}
			if err := json.Unmarshal(raw, &o.EnforcementMode); err != nil {
				return fmt.Errorf("enforcementMode: %w", err)
			}
			if o.EnforcementMode != "audit" && o.EnforcementMode != "enforce" {
				return fmt.Errorf("enforcementMode: invalid value %q", o.EnforcementMode)
			}
		}
	}
	if _, err := decoder.Token(); err != nil {
		return err
	}
	if !clusterSet {
		return fmt.Errorf("cluster: field is required")
	}
	if !enforcementModeSet {
		return fmt.Errorf("enforcementMode: field is required")
	}
	return nil
}

type DistributionNameView struct {
	Name string `json:"name" binding:"required"`
}

type DistributionListHTTPResponse struct {
	Code    int                   `json:"code"`
	Message string                `json:"message"`
	Data    *DistributionPageView `json:"data"`
}

func (*DistributionListHTTPResponse) listDistributionsRes() {}

type DistributionDetailHTTPResponse struct {
	Code    int               `json:"code"`
	Message string            `json:"message"`
	Data    *DistributionView `json:"data"`
}

func (*DistributionDetailHTTPResponse) getDistributionRes() {}

type DistributionNameHTTPResponse struct {
	Code    int                  `json:"code"`
	Message string               `json:"message"`
	Data    DistributionNameView `json:"data"`
}

func (*DistributionNameHTTPResponse) createDistributionRes() {}
func (*DistributionNameHTTPResponse) updateDistributionRes() {}

type DistributionEmptyHTTPResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (*DistributionEmptyHTTPResponse) deleteDistributionRes() {}

type DistributionBadRequestHTTPError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (*DistributionBadRequestHTTPError) HTTPStatus() int        { return 400 }
func (*DistributionBadRequestHTTPError) createDistributionRes() {}
func (*DistributionBadRequestHTTPError) updateDistributionRes() {}

type DistributionForbiddenHTTPError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (*DistributionForbiddenHTTPError) HTTPStatus() int        { return 403 }
func (*DistributionForbiddenHTTPError) deleteDistributionRes() {}

type DistributionNotFoundHTTPError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (*DistributionNotFoundHTTPError) HTTPStatus() int        { return 404 }
func (*DistributionNotFoundHTTPError) getDistributionRes()    {}
func (*DistributionNotFoundHTTPError) updateDistributionRes() {}
func (*DistributionNotFoundHTTPError) deleteDistributionRes() {}

type DistributionConflictHTTPError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (*DistributionConflictHTTPError) HTTPStatus() int        { return 409 }
func (*DistributionConflictHTTPError) createDistributionRes() {}
func (*DistributionConflictHTTPError) updateDistributionRes() {}
func (*DistributionConflictHTTPError) deleteDistributionRes() {}

type DistributionInternalServerHTTPError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (*DistributionInternalServerHTTPError) HTTPStatus() int        { return 500 }
func (*DistributionInternalServerHTTPError) listDistributionsRes()  {}
func (*DistributionInternalServerHTTPError) createDistributionRes() {}
func (*DistributionInternalServerHTTPError) getDistributionRes()    {}
func (*DistributionInternalServerHTTPError) updateDistributionRes() {}
func (*DistributionInternalServerHTTPError) deleteDistributionRes() {}
