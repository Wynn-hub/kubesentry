package dto

type SummaryView struct {
	Totals        map[string]int     `json:"totals"`
	ClusterPhases SummaryPhasesView  `json:"clusterPhases"`
	Groups        []SummaryGroupView `json:"groups"`
}

type SummaryPhasesView struct {
	Online      int `json:"online"`
	Offline     int `json:"offline"`
	Registering int `json:"registering"`
}

type SummaryGroupView struct {
	Name          string `json:"name"`
	DisplayName   string `json:"displayName"`
	Enabled       bool   `json:"enabled"`
	ResolvedCount int    `json:"resolvedCount"`
	EnforceCount  int    `json:"enforceCount"`
	AuditCount    int    `json:"auditCount"`
}
