// Package dbrepo is the GORM data-access layer shared by cmd/gateway and
// cmd/console. Table shapes match spec section 二; jsonb columns reuse the
// existing v1alpha1 Spec structs verbatim instead of redefining them.
package dbrepo

import (
	"time"

	"gorm.io/datatypes"

	"github.com/Wynn-hub/kubesentry/internal/api/v1alpha1"
	"github.com/Wynn-hub/kubesentry/internal/dbmodel"
)

type Cluster struct {
	dbmodel.Base
	Name            string `gorm:"uniqueIndex:,where:deleted_at IS NULL"`
	ClusterUID      string
	Labels          datatypes.JSONType[map[string]string]
	Phase           string
	OperatorVersion string
	LastHeartbeatAt *time.Time
	RegTokenHash    string
	AuthTokenHash   string
}

func (Cluster) TableName() string { return "kubesentry.clusters" }

type Policy struct {
	dbmodel.Base
	Name            string `gorm:"uniqueIndex:,where:deleted_at IS NULL"`
	Description     string
	Rego            string
	EnforcementMode string
	Match           datatypes.JSONType[v1alpha1.PolicyMatch]
	CompileOK       bool
	CompileError    string
	Labels          datatypes.JSONType[map[string]string]
	Annotations     datatypes.JSONType[map[string]string]
}

func (Policy) TableName() string { return "kubesentry.policies" }

type PolicyVersion struct {
	dbmodel.Base
	PolicyID     int64
	Version      int
	Rego         string
	SpecSnapshot datatypes.JSONType[v1alpha1.PolicySpec]
}

func (PolicyVersion) TableName() string { return "kubesentry.policy_versions" }

type PolicyGroup struct {
	dbmodel.Base
	Name        string `gorm:"uniqueIndex:,where:deleted_at IS NULL"`
	Spec        datatypes.JSONType[v1alpha1.PolicyGroupSpec]
	Labels      datatypes.JSONType[map[string]string]
	Annotations datatypes.JSONType[map[string]string]
}

func (PolicyGroup) TableName() string { return "kubesentry.policy_groups" }

type PolicyException struct {
	dbmodel.Base
	Name        string `gorm:"uniqueIndex:,where:deleted_at IS NULL"`
	Spec        datatypes.JSONType[v1alpha1.PolicyExceptionSpec]
	ExpiresAt   *time.Time
	Labels      datatypes.JSONType[map[string]string]
	Annotations datatypes.JSONType[map[string]string]
}

func (PolicyException) TableName() string { return "kubesentry.policy_exceptions" }

type PolicyDistribution struct {
	dbmodel.Base
	Name string `gorm:"uniqueIndex:,where:deleted_at IS NULL"`
	Spec datatypes.JSONType[v1alpha1.PolicyDistributionSpec]
}

func (PolicyDistribution) TableName() string { return "kubesentry.policy_distributions" }

type DistributionClusterStatus struct {
	dbmodel.Base
	DistributionID     int64
	ClusterID          int64
	Status             string
	AppliedVersionHash string
	LastError          string
}

func (DistributionClusterStatus) TableName() string { return "kubesentry.distribution_cluster_status" }
