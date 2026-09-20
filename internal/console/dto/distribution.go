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
