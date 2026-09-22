package dto

import (
	"fmt"
	"time"
)

type Option[T any] struct {
	value T
	set   bool
}

func NewOption[T any](value T) Option[T] { return Option[T]{value: value, set: true} }
func (o *Option[T]) SetTo(value T)       { o.value, o.set = value, true }
func (o Option[T]) Get() (T, bool)       { return o.value, o.set }
func (o Option[T]) Or(fallback T) T {
	if o.set {
		return o.value
	}
	return fallback
}

type (
	ResourceID string
	PageSize   int

	GetPlatformInstanceTrendComponent       string
	GetPlatformOverviewHealth               string
	ListAdmissionEventsSortBy               string
	ListAdmissionEventsSortOrder            string
	ListAdmissionEventsVerdictItem          string
	ListAdmissionLinkIncidentsComponentItem string
	ListAdmissionLinkIncidentsStatusItem    string
	ListClustersSortBy                      string
	ListClustersSortOrder                   string
	ListDistributionsPageSize               int
	ListDistributionsSortBy                 string
	ListDistributionsSortOrder              string
	ListExceptionsSortBy                    string
	ListExceptionsSortOrder                 string
	ListInstallRecordsSortBy                string
	ListInstallRecordsSortOrder             string
	ListPlatformLeadershipEventsComponent   string
	ListPoliciesModeItem                    string
	ListPoliciesPhaseItem                   string
	ListPoliciesSortBy                      string
	ListPoliciesSortOrder                   string
	ListPoliciesSourceItem                  string
	ListPolicyGroupsSortBy                  string
	ListPolicyGroupsSortOrder               string

	OptBool                                  = Option[bool]
	OptDateTime                              = Option[time.Time]
	OptGetPlatformOverviewHealth             = Option[GetPlatformOverviewHealth]
	OptInt                                   = Option[int]
	OptListAdmissionEventsSortBy             = Option[ListAdmissionEventsSortBy]
	OptListAdmissionEventsSortOrder          = Option[ListAdmissionEventsSortOrder]
	OptListClustersSortBy                    = Option[ListClustersSortBy]
	OptListClustersSortOrder                 = Option[ListClustersSortOrder]
	OptListDistributionsPageSize             = Option[ListDistributionsPageSize]
	OptListDistributionsSortBy               = Option[ListDistributionsSortBy]
	OptListDistributionsSortOrder            = Option[ListDistributionsSortOrder]
	OptListExceptionsSortBy                  = Option[ListExceptionsSortBy]
	OptListExceptionsSortOrder               = Option[ListExceptionsSortOrder]
	OptListInstallRecordsSortBy              = Option[ListInstallRecordsSortBy]
	OptListInstallRecordsSortOrder           = Option[ListInstallRecordsSortOrder]
	OptListPlatformLeadershipEventsComponent = Option[ListPlatformLeadershipEventsComponent]
	OptListPoliciesSortBy                    = Option[ListPoliciesSortBy]
	OptListPoliciesSortOrder                 = Option[ListPoliciesSortOrder]
	OptListPolicyGroupsSortBy                = Option[ListPolicyGroupsSortBy]
	OptListPolicyGroupsSortOrder             = Option[ListPolicyGroupsSortOrder]
	OptPageSize                              = Option[PageSize]
	OptString                                = Option[string]
)

const (
	PageSize20 PageSize = 20
	PageSize50 PageSize = 50

	GetPlatformInstanceTrendComponentGateway      GetPlatformInstanceTrendComponent     = "gateway"
	GetPlatformOverviewHealthDegraded             GetPlatformOverviewHealth             = "degraded"
	ListAdmissionEventsSortByOccurredAt           ListAdmissionEventsSortBy             = "occurredAt"
	ListAdmissionEventsSortOrderDesc              ListAdmissionEventsSortOrder          = "desc"
	ListAdmissionEventsVerdictItemDeny            ListAdmissionEventsVerdictItem        = "deny"
	ListClustersSortByName                        ListClustersSortBy                    = "name"
	ListClustersSortOrderAsc                      ListClustersSortOrder                 = "asc"
	ListDistributionsPageSize20                   ListDistributionsPageSize             = 20
	ListDistributionsSortByName                   ListDistributionsSortBy               = "name"
	ListDistributionsSortOrderAsc                 ListDistributionsSortOrder            = "asc"
	ListExceptionsSortByName                      ListExceptionsSortBy                  = "name"
	ListExceptionsSortOrderAsc                    ListExceptionsSortOrder               = "asc"
	ListInstallRecordsSortByCreatedAt             ListInstallRecordsSortBy              = "createdAt"
	ListInstallRecordsSortOrderDesc               ListInstallRecordsSortOrder           = "desc"
	ListPlatformLeadershipEventsComponentOperator ListPlatformLeadershipEventsComponent = "operator"
	ListPoliciesSortByName                        ListPoliciesSortBy                    = "name"
	ListPoliciesSortOrderAsc                      ListPoliciesSortOrder                 = "asc"
	ListPolicyGroupsSortByName                    ListPolicyGroupsSortBy                = "name"
	ListPolicyGroupsSortOrderAsc                  ListPolicyGroupsSortOrder             = "asc"
)

func NewOptBool(v bool) OptBool              { return NewOption(v) }
func NewOptDateTime(v time.Time) OptDateTime { return NewOption(v) }
func NewOptGetPlatformOverviewHealth(v GetPlatformOverviewHealth) OptGetPlatformOverviewHealth {
	return NewOption(v)
}
func NewOptInt(v int) OptInt { return NewOption(v) }
func NewOptListAdmissionEventsSortBy(v ListAdmissionEventsSortBy) OptListAdmissionEventsSortBy {
	return NewOption(v)
}

func NewOptListAdmissionEventsSortOrder(v ListAdmissionEventsSortOrder) OptListAdmissionEventsSortOrder {
	return NewOption(v)
}
func NewOptListClustersSortBy(v ListClustersSortBy) OptListClustersSortBy { return NewOption(v) }
func NewOptListClustersSortOrder(v ListClustersSortOrder) OptListClustersSortOrder {
	return NewOption(v)
}

func NewOptListDistributionsPageSize(v ListDistributionsPageSize) OptListDistributionsPageSize {
	return NewOption(v)
}

func NewOptListDistributionsSortBy(v ListDistributionsSortBy) OptListDistributionsSortBy {
	return NewOption(v)
}

func NewOptListDistributionsSortOrder(v ListDistributionsSortOrder) OptListDistributionsSortOrder {
	return NewOption(v)
}
func NewOptListExceptionsSortBy(v ListExceptionsSortBy) OptListExceptionsSortBy { return NewOption(v) }
func NewOptListExceptionsSortOrder(v ListExceptionsSortOrder) OptListExceptionsSortOrder {
	return NewOption(v)
}

func NewOptListInstallRecordsSortBy(v ListInstallRecordsSortBy) OptListInstallRecordsSortBy {
	return NewOption(v)
}

func NewOptListInstallRecordsSortOrder(v ListInstallRecordsSortOrder) OptListInstallRecordsSortOrder {
	return NewOption(v)
}

func NewOptListPlatformLeadershipEventsComponent(v ListPlatformLeadershipEventsComponent) OptListPlatformLeadershipEventsComponent {
	return NewOption(v)
}

func NewOptListPolicyGroupsSortBy(v ListPolicyGroupsSortBy) OptListPolicyGroupsSortBy {
	return NewOption(v)
}

func NewOptListPolicyGroupsSortOrder(v ListPolicyGroupsSortOrder) OptListPolicyGroupsSortOrder {
	return NewOption(v)
}
func NewOptPageSize(v PageSize) OptPageSize { return NewOption(v) }
func NewOptString(v string) OptString       { return NewOption(v) }

func validateStringEnum(value string, allowed ...string) error {
	for _, candidate := range allowed {
		if value == candidate {
			return nil
		}
	}
	return fmt.Errorf("invalid value: %v", value)
}

func validatePageSize(value int) error {
	if value == 10 || value == 20 || value == 50 || value == 100 {
		return nil
	}
	return fmt.Errorf("invalid value: %v", value)
}

func (s ResourceID) Validate() error {
	if s == "" {
		return fmt.Errorf("invalid value: %v", s)
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return fmt.Errorf("invalid value: %v", s)
		}
	}
	return nil
}

func (s PageSize) Validate() error { return validatePageSize(int(s)) }
func (s GetPlatformInstanceTrendComponent) Validate() error {
	return validateStringEnum(string(s), "webhook", "operator", "gateway", "console-api", "console-web", "postgres")
}

func (s GetPlatformOverviewHealth) Validate() error {
	return validateStringEnum(string(s), "ok", "degraded", "unavailable")
}

func (s ListAdmissionEventsSortBy) Validate() error {
	return validateStringEnum(string(s), "occurredAt", "createdAt", "verdict")
}

func (s ListAdmissionEventsSortOrder) Validate() error {
	return validateStringEnum(string(s), "asc", "desc")
}

func (s ListAdmissionEventsVerdictItem) Validate() error {
	return validateStringEnum(string(s), "audit", "deny")
}

func (s ListAdmissionLinkIncidentsComponentItem) Validate() error {
	return validateStringEnum(string(s), "webhook", "operator", "gateway")
}

func (s ListAdmissionLinkIncidentsStatusItem) Validate() error {
	return validateStringEnum(string(s), "active", "interrupted", "closed")
}

func (s ListClustersSortBy) Validate() error {
	return validateStringEnum(string(s), "name", "phase", "operatorVersion", "lastHeartbeatAt", "createdAt")
}

func (s ListClustersSortOrder) Validate() error {
	return validateStringEnum(string(s), "asc", "desc")
}
func (s ListDistributionsPageSize) Validate() error { return validatePageSize(int(s)) }
func (s ListDistributionsSortBy) Validate() error {
	return validateStringEnum(string(s), "name")
}

func (s ListDistributionsSortOrder) Validate() error {
	return validateStringEnum(string(s), "asc", "desc")
}

func (s ListExceptionsSortBy) Validate() error {
	return validateStringEnum(string(s), "name", "phase", "targetSummary", "duration", "expiresAt")
}

func (s ListExceptionsSortOrder) Validate() error {
	return validateStringEnum(string(s), "asc", "desc")
}

func (s ListInstallRecordsSortBy) Validate() error {
	return validateStringEnum(string(s), "clusterName", "namespace", "createdAt")
}

func (s ListInstallRecordsSortOrder) Validate() error {
	return validateStringEnum(string(s), "asc", "desc")
}

func (s ListPlatformLeadershipEventsComponent) Validate() error {
	return validateStringEnum(string(s), "webhook", "operator", "gateway", "console-api", "console-web", "postgres")
}

func (s ListPoliciesModeItem) Validate() error {
	return validateStringEnum(string(s), "enforce", "audit")
}

func (s ListPoliciesPhaseItem) Validate() error {
	return validateStringEnum(string(s), "Ready", "Invalid")
}

func (s ListPoliciesSortBy) Validate() error {
	return validateStringEnum(string(s), "name", "source", "enforcementMode", "phase", "createdAt", "updatedAt")
}

func (s ListPoliciesSortOrder) Validate() error {
	return validateStringEnum(string(s), "asc", "desc")
}

func (s ListPoliciesSourceItem) Validate() error {
	return validateStringEnum(string(s), "builtin", "custom")
}

func (s ListPolicyGroupsSortBy) Validate() error {
	return validateStringEnum(string(s), "name", "displayName", "enabled", "phase", "resolvedCount")
}

func (s ListPolicyGroupsSortOrder) Validate() error {
	return validateStringEnum(string(s), "asc", "desc")
}

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
