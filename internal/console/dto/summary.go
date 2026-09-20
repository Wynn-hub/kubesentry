package dto

type SummaryView struct {
	Totals        map[string]int     `json:"totals" binding:"required"`
	ClusterPhases SummaryPhasesView  `json:"clusterPhases" binding:"required"`
	Groups        []SummaryGroupView `json:"groups" binding:"required"`
}

type SummaryPhasesView struct {
	Online      int `json:"online" binding:"required"`
	Offline     int `json:"offline" binding:"required"`
	Registering int `json:"registering" binding:"required"`
}

type SummaryGroupView struct {
	Name          string `json:"name" binding:"required"`
	DisplayName   string `json:"displayName" binding:"required"`
	Enabled       bool   `json:"enabled" binding:"required"`
	ResolvedCount int    `json:"resolvedCount" binding:"required"`
	EnforceCount  int    `json:"enforceCount" binding:"required"`
	AuditCount    int    `json:"auditCount" binding:"required"`
}

type SummaryHTTPResponse struct {
	Code    int          `json:"code"`
	Message string       `json:"message"`
	Data    *SummaryView `json:"data"`
}

func (*SummaryHTTPResponse) getSummaryRes() {}

type SummaryHTTPError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (*SummaryHTTPError) getSummaryRes() {}
