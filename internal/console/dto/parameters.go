package dto

import "time"

type (
	CreateUninstallTaskParams struct{ Name string }
	DeleteClusterParams       struct{ Name string }
	DeleteDistributionParams  struct{ Name string }
	DeleteExceptionParams     struct{ Name string }
	DeletePolicyParams        struct {
		Name  string
		Force OptBool
	}
)
type (
	DeletePolicyGroupParams      struct{ Name string }
	GetAdmissionEventParams      struct{ ID ResourceID }
	GetAdmissionEventTrendParams struct {
		From time.Time
		To   time.Time
	}
)
type (
	GetAdmissionLinkIncidentParams struct{ ID ResourceID }
	GetClusterParams               struct{ Name string }
	GetDistributionParams          struct{ Name string }
	GetExceptionParams             struct{ Name string }
	GetFieldSchemaParams           struct {
		Group    OptString
		Version  string
		Resource string
		Refresh  OptBool
	}
)
type (
	GetLatestInstallParamsParams   struct{ Name string }
	GetPlatformInstanceTrendParams struct {
		Component GetPlatformInstanceTrendComponent
		Instance  string
		Cluster   OptString
		From      OptDateTime
		To        OptDateTime
	}
)
type GetPlatformOverviewParams struct {
	Page     OptInt
	PageSize OptPageSize
	Health   OptGetPlatformOverviewHealth
}
type (
	GetPolicyParams           struct{ Name string }
	GetPolicyGroupParams      struct{ Name string }
	ListAdmissionEventsParams struct {
		Page         OptInt
		PageSize     OptPageSize
		SortBy       OptListAdmissionEventsSortBy
		SortOrder    OptListAdmissionEventsSortOrder
		Cluster      []string
		Verdict      []ListAdmissionEventsVerdictItem
		Policy       OptString
		Namespace    []string
		Gvk          []string
		ResourceName OptString
		Username     OptString
		From         OptDateTime
		To           OptDateTime
	}
)
type ListAdmissionLinkIncidentsParams struct {
	Page      OptInt
	PageSize  OptPageSize
	Cluster   []string
	Component []ListAdmissionLinkIncidentsComponentItem
	Reason    []string
	Status    []ListAdmissionLinkIncidentsStatusItem
	From      OptDateTime
	To        OptDateTime
}
type ListClustersParams struct {
	Page      OptInt
	PageSize  OptPageSize
	SortBy    OptListClustersSortBy
	SortOrder OptListClustersSortOrder
}
type ListDistributionsParams struct {
	Page      OptInt
	PageSize  OptListDistributionsPageSize
	SortBy    OptListDistributionsSortBy
	SortOrder OptListDistributionsSortOrder
}
type ListExceptionsParams struct {
	Page      OptInt
	PageSize  OptPageSize
	SortBy    OptListExceptionsSortBy
	SortOrder OptListExceptionsSortOrder
}
type ListInstallRecordsParams struct {
	Page      OptInt
	PageSize  OptPageSize
	SortBy    OptListInstallRecordsSortBy
	SortOrder OptListInstallRecordsSortOrder
}
type ListPlatformLeadershipEventsParams struct {
	Page      OptInt
	PageSize  OptPageSize
	Component OptListPlatformLeadershipEventsComponent
	Cluster   OptString
	From      OptDateTime
	To        OptDateTime
}
type ListPoliciesParams struct {
	Page      OptInt
	PageSize  OptPageSize
	SortBy    OptListPoliciesSortBy
	SortOrder OptListPoliciesSortOrder
	Phase     []ListPoliciesPhaseItem
	Mode      []ListPoliciesModeItem
	Source    []ListPoliciesSourceItem
	Keyword   OptString
}
type ListPolicyGroupsParams struct {
	Page      OptInt
	PageSize  OptPageSize
	SortBy    OptListPolicyGroupsSortBy
	SortOrder OptListPolicyGroupsSortOrder
}
type (
	ListPolicyVersionsParams  struct{ Name string }
	ListUninstallTasksParams  struct{ Name string }
	ReissueClusterTokenParams struct{ Name string }
	RetryUninstallTaskParams  struct {
		Name   string
		TaskID ResourceID
	}
)
type (
	RollbackPolicyParams        struct{ Name string }
	SetPolicyGroupEnabledParams struct{ Name string }
	UpdateClusterParams         struct{ Name string }
	UpdateDistributionParams    struct{ Name string }
	UpdateExceptionParams       struct{ Name string }
	UpdatePolicyParams          struct{ Name string }
	UpdatePolicyGroupParams     struct{ Name string }
)
