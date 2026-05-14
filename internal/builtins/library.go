package builtins

import (
	"embed"
	"fmt"

	"github.com/Wynn-hub/kubesentry/internal/api/v1alpha1"
)

//go:embed rego
var regoFS embed.FS

// BuiltinPolicy is the metadata + Rego content for a single built-in policy.
type BuiltinPolicy struct {
	Key         string
	Description string
	DefaultMode string // v1alpha1.ModeEnforce or v1alpha1.ModeAudit
	Match       v1alpha1.PolicyMatch
	Rego        string
}

// Library is the complete built-in policy catalogue, keyed by policy key.
var Library = buildLibrary()

func mustRego(key string) string {
	data, err := regoFS.ReadFile("rego/" + key + ".rego")
	if err != nil {
		panic(fmt.Sprintf("builtins: missing rego file for key %q: %v", key, err))
	}
	return string(data)
}

// ---- common match specs ----

var (
	// pods + deployments + statefulsets + daemonsets
	matchWorkloads = v1alpha1.PolicyMatch{
		Operations: []string{"CREATE", "UPDATE"},
		Resources: []v1alpha1.MatchResource{
			{APIGroups: []string{""}, APIVersions: []string{"v1"}, Resources: []string{"pods"}},
			{APIGroups: []string{"apps"}, APIVersions: []string{"v1"}, Resources: []string{"deployments", "statefulsets", "daemonsets"}},
		},
	}
	// pods + deployments + statefulsets + daemonsets + jobs + cronjobs
	matchWorkloadsWithBatch = v1alpha1.PolicyMatch{
		Operations: []string{"CREATE", "UPDATE"},
		Resources: []v1alpha1.MatchResource{
			{APIGroups: []string{""}, APIVersions: []string{"v1"}, Resources: []string{"pods"}},
			{APIGroups: []string{"apps"}, APIVersions: []string{"v1"}, Resources: []string{"deployments", "statefulsets", "daemonsets"}},
			{APIGroups: []string{"batch"}, APIVersions: []string{"v1"}, Resources: []string{"jobs", "cronjobs"}},
		},
	}
	// pods + deployments + statefulsets (no daemonsets/jobs)
	matchWorkloadsNoDaemon = v1alpha1.PolicyMatch{
		Operations: []string{"CREATE", "UPDATE"},
		Resources: []v1alpha1.MatchResource{
			{APIGroups: []string{""}, APIVersions: []string{"v1"}, Resources: []string{"pods"}},
			{APIGroups: []string{"apps"}, APIVersions: []string{"v1"}, Resources: []string{"deployments", "statefulsets"}},
		},
	}
	// pods + deployments + statefulsets + daemonsets + serviceaccounts
	matchWorkloadsAndSA = v1alpha1.PolicyMatch{
		Operations: []string{"CREATE", "UPDATE"},
		Resources: []v1alpha1.MatchResource{
			{APIGroups: []string{""}, APIVersions: []string{"v1"}, Resources: []string{"pods", "serviceaccounts"}},
			{APIGroups: []string{"apps"}, APIVersions: []string{"v1"}, Resources: []string{"deployments", "statefulsets", "daemonsets"}},
		},
	}
	matchDeployments = v1alpha1.PolicyMatch{
		Operations: []string{"CREATE", "UPDATE"},
		Resources: []v1alpha1.MatchResource{
			{APIGroups: []string{"apps"}, APIVersions: []string{"v1"}, Resources: []string{"deployments"}},
		},
	}
	matchConfigMaps = v1alpha1.PolicyMatch{
		Operations: []string{"CREATE", "UPDATE"},
		Resources: []v1alpha1.MatchResource{
			{APIGroups: []string{""}, APIVersions: []string{"v1"}, Resources: []string{"configmaps"}},
		},
	}
	matchIngresses = v1alpha1.PolicyMatch{
		Operations: []string{"CREATE", "UPDATE"},
		Resources: []v1alpha1.MatchResource{
			{APIGroups: []string{"networking.k8s.io"}, APIVersions: []string{"v1"}, Resources: []string{"ingresses"}},
		},
	}
	matchClusterRoles = v1alpha1.PolicyMatch{
		Operations: []string{"CREATE", "UPDATE"},
		Resources: []v1alpha1.MatchResource{
			{APIGroups: []string{"rbac.authorization.k8s.io"}, APIVersions: []string{"v1"}, Resources: []string{"clusterroles"}},
		},
	}
	matchRoles = v1alpha1.PolicyMatch{
		Operations: []string{"CREATE", "UPDATE"},
		Resources: []v1alpha1.MatchResource{
			{APIGroups: []string{"rbac.authorization.k8s.io"}, APIVersions: []string{"v1"}, Resources: []string{"roles"}},
		},
	}
	matchClusterRoleBindings = v1alpha1.PolicyMatch{
		Operations: []string{"CREATE", "UPDATE"},
		Resources: []v1alpha1.MatchResource{
			{APIGroups: []string{"rbac.authorization.k8s.io"}, APIVersions: []string{"v1"}, Resources: []string{"clusterrolebindings"}},
		},
	}
	matchRoleBindings = v1alpha1.PolicyMatch{
		Operations: []string{"CREATE", "UPDATE"},
		Resources: []v1alpha1.MatchResource{
			{APIGroups: []string{"rbac.authorization.k8s.io"}, APIVersions: []string{"v1"}, Resources: []string{"rolebindings"}},
		},
	}
	matchHPAs = v1alpha1.PolicyMatch{
		Operations: []string{"CREATE", "UPDATE"},
		Resources: []v1alpha1.MatchResource{
			{APIGroups: []string{"autoscaling"}, APIVersions: []string{"v2"}, Resources: []string{"horizontalpodautoscalers"}},
		},
	}
)

func buildLibrary() map[string]BuiltinPolicy {
	return map[string]BuiltinPolicy{
		// ---- Security ----
		"runAsPrivileged": {
			Key: "runAsPrivileged", DefaultMode: v1alpha1.ModeEnforce, Match: matchWorkloads,
			Description: "Fails when securityContext.privileged is true.",
			Rego:        mustRego("runAsPrivileged"),
		},
		"privilegeEscalationAllowed": {
			Key: "privilegeEscalationAllowed", DefaultMode: v1alpha1.ModeEnforce, Match: matchWorkloads,
			Description: "Fails when securityContext.allowPrivilegeEscalation is true.",
			Rego:        mustRego("privilegeEscalationAllowed"),
		},
		"runAsRootAllowed": {
			Key: "runAsRootAllowed", DefaultMode: v1alpha1.ModeAudit, Match: matchWorkloads,
			Description: "Fails when securityContext.runAsNonRoot is not true.",
			Rego:        mustRego("runAsRootAllowed"),
		},
		"notReadOnlyRootFilesystem": {
			Key: "notReadOnlyRootFilesystem", DefaultMode: v1alpha1.ModeAudit, Match: matchWorkloads,
			Description: "Fails when securityContext.readOnlyRootFilesystem is not true.",
			Rego:        mustRego("notReadOnlyRootFilesystem"),
		},
		"linuxHardening": {
			Key: "linuxHardening", DefaultMode: v1alpha1.ModeEnforce, Match: matchWorkloads,
			Description: "Fails when neither Seccomp, SELinux, nor capabilities.drop is in use.",
			Rego:        mustRego("linuxHardening"),
		},
		"insecureCapabilities": {
			Key: "insecureCapabilities", DefaultMode: v1alpha1.ModeAudit, Match: matchWorkloads,
			Description: "Fails when securityContext.capabilities includes insecure capabilities.",
			Rego:        mustRego("insecureCapabilities"),
		},
		"dangerousCapabilities": {
			Key: "dangerousCapabilities", DefaultMode: v1alpha1.ModeEnforce, Match: matchWorkloads,
			Description: "Fails when securityContext.capabilities includes dangerous capabilities.",
			Rego:        mustRego("dangerousCapabilities"),
		},
		"hostPIDSet": {
			Key: "hostPIDSet", DefaultMode: v1alpha1.ModeEnforce, Match: matchWorkloads,
			Description: "Fails when hostPID attribute is configured.",
			Rego:        mustRego("hostPIDSet"),
		},
		"hostIPCSet": {
			Key: "hostIPCSet", DefaultMode: v1alpha1.ModeEnforce, Match: matchWorkloads,
			Description: "Fails when hostIPC attribute is configured.",
			Rego:        mustRego("hostIPCSet"),
		},
		"hostNetworkSet": {
			Key: "hostNetworkSet", DefaultMode: v1alpha1.ModeAudit, Match: matchWorkloads,
			Description: "Fails when hostNetwork attribute is configured.",
			Rego:        mustRego("hostNetworkSet"),
		},
		"hostPortSet": {
			Key: "hostPortSet", DefaultMode: v1alpha1.ModeAudit, Match: matchWorkloads,
			Description: "Fails when hostPort attribute is configured.",
			Rego:        mustRego("hostPortSet"),
		},
		"sensitiveContainerEnvVar": {
			Key: "sensitiveContainerEnvVar", DefaultMode: v1alpha1.ModeEnforce, Match: matchWorkloads,
			Description: "Fails when the container sets potentially sensitive environment variables.",
			Rego:        mustRego("sensitiveContainerEnvVar"),
		},
		"automountServiceAccountToken": {
			Key: "automountServiceAccountToken", DefaultMode: v1alpha1.ModeAudit, Match: matchWorkloadsAndSA,
			Description: "Fails when automountServiceAccountToken is not explicitly set to false.",
			Rego:        mustRego("automountServiceAccountToken"),
		},
		"sensitiveConfigmapContent": {
			Key: "sensitiveConfigmapContent", DefaultMode: v1alpha1.ModeEnforce, Match: matchConfigMaps,
			Description: "Fails when potentially sensitive content is detected in ConfigMap keys or values.",
			Rego:        mustRego("sensitiveConfigmapContent"),
		},
		"tlsSettingsMissing": {
			Key: "tlsSettingsMissing", DefaultMode: v1alpha1.ModeAudit, Match: matchIngresses,
			Description: "Fails when an Ingress lacks TLS settings.",
			Rego:        mustRego("tlsSettingsMissing"),
		},
		"clusterrolePodExecAttach": {
			Key: "clusterrolePodExecAttach", DefaultMode: v1alpha1.ModeEnforce, Match: matchClusterRoles,
			Description: "Fails when the ClusterRole allows pods/exec or pods/attach.",
			Rego:        mustRego("clusterrolePodExecAttach"),
		},
		"rolePodExecAttach": {
			Key: "rolePodExecAttach", DefaultMode: v1alpha1.ModeEnforce, Match: matchRoles,
			Description: "Fails when the Role allows pods/exec or pods/attach.",
			Rego:        mustRego("rolePodExecAttach"),
		},
		"clusterrolebindingPodExecAttach": {
			Key: "clusterrolebindingPodExecAttach", DefaultMode: v1alpha1.ModeEnforce, Match: matchClusterRoleBindings,
			Description: "Fails when the ClusterRoleBinding references a ClusterRole known to allow pods/exec or pods/attach.",
			Rego:        mustRego("clusterrolebindingPodExecAttach"),
		},
		"rolebindingRolePodExecAttach": {
			Key: "rolebindingRolePodExecAttach", DefaultMode: v1alpha1.ModeEnforce, Match: matchRoleBindings,
			Description: "Fails when the RoleBinding references a Role known to allow pods/exec or pods/attach.",
			Rego:        mustRego("rolebindingRolePodExecAttach"),
		},
		"rolebindingClusterRolePodExecAttach": {
			Key: "rolebindingClusterRolePodExecAttach", DefaultMode: v1alpha1.ModeEnforce, Match: matchRoleBindings,
			Description: "Fails when the RoleBinding references a ClusterRole known to allow pods/exec or pods/attach.",
			Rego:        mustRego("rolebindingClusterRolePodExecAttach"),
		},
		"clusterrolebindingClusterAdmin": {
			Key: "clusterrolebindingClusterAdmin", DefaultMode: v1alpha1.ModeEnforce, Match: matchClusterRoleBindings,
			Description: "Fails when the ClusterRoleBinding references the cluster-admin ClusterRole.",
			Rego:        mustRego("clusterrolebindingClusterAdmin"),
		},
		"rolebindingClusterAdminClusterRole": {
			Key: "rolebindingClusterAdminClusterRole", DefaultMode: v1alpha1.ModeEnforce, Match: matchRoleBindings,
			Description: "Fails when the RoleBinding references the cluster-admin ClusterRole.",
			Rego:        mustRego("rolebindingClusterAdminClusterRole"),
		},
		"rolebindingClusterAdminRole": {
			Key: "rolebindingClusterAdminRole", DefaultMode: v1alpha1.ModeEnforce, Match: matchRoleBindings,
			Description: "Fails when the RoleBinding references a Role with wildcard permissions.",
			Rego:        mustRego("rolebindingClusterAdminRole"),
		},
		// ---- Efficiency ----
		"cpuRequestsMissing": {
			Key: "cpuRequestsMissing", DefaultMode: v1alpha1.ModeAudit, Match: matchWorkloadsWithBatch,
			Description: "Fails when resources.requests.cpu is not configured.",
			Rego:        mustRego("cpuRequestsMissing"),
		},
		"memoryRequestsMissing": {
			Key: "memoryRequestsMissing", DefaultMode: v1alpha1.ModeAudit, Match: matchWorkloadsWithBatch,
			Description: "Fails when resources.requests.memory is not configured.",
			Rego:        mustRego("memoryRequestsMissing"),
		},
		"cpuLimitsMissing": {
			Key: "cpuLimitsMissing", DefaultMode: v1alpha1.ModeAudit, Match: matchWorkloadsWithBatch,
			Description: "Fails when resources.limits.cpu is not configured.",
			Rego:        mustRego("cpuLimitsMissing"),
		},
		"memoryLimitsMissing": {
			Key: "memoryLimitsMissing", DefaultMode: v1alpha1.ModeAudit, Match: matchWorkloadsWithBatch,
			Description: "Fails when resources.limits.memory is not configured.",
			Rego:        mustRego("memoryLimitsMissing"),
		},
		// ---- Reliability ----
		"readinessProbeMissing": {
			Key: "readinessProbeMissing", DefaultMode: v1alpha1.ModeAudit, Match: matchWorkloadsNoDaemon,
			Description: "Fails when a readiness probe is not configured.",
			Rego:        mustRego("readinessProbeMissing"),
		},
		"livenessProbeMissing": {
			Key: "livenessProbeMissing", DefaultMode: v1alpha1.ModeAudit, Match: matchWorkloads,
			Description: "Fails when a liveness probe is not configured.",
			Rego:        mustRego("livenessProbeMissing"),
		},
		"tagNotSpecified": {
			Key: "tagNotSpecified", DefaultMode: v1alpha1.ModeEnforce, Match: matchWorkloadsWithBatch,
			Description: "Fails when an image tag is not specified or is latest.",
			Rego:        mustRego("tagNotSpecified"),
		},
		"pullPolicyNotAlways": {
			Key: "pullPolicyNotAlways", DefaultMode: v1alpha1.ModeAudit, Match: matchWorkloadsWithBatch,
			Description: "Fails when imagePullPolicy is not Always.",
			Rego:        mustRego("pullPolicyNotAlways"),
		},
		"priorityClassNotSet": {
			Key: "priorityClassNotSet", DefaultMode: v1alpha1.ModeAudit, Match: matchWorkloadsNoDaemon,
			Description: "Fails when priorityClassName is not set.",
			Rego:        mustRego("priorityClassNotSet"),
		},
		"deploymentMissingReplicas": {
			Key: "deploymentMissingReplicas", DefaultMode: v1alpha1.ModeAudit, Match: matchDeployments,
			Description: "Fails when a Deployment has only one replica.",
			Rego:        mustRego("deploymentMissingReplicas"),
		},
		"metadataAndInstanceMismatched": {
			Key: "metadataAndInstanceMismatched", DefaultMode: v1alpha1.ModeAudit, Match: matchWorkloads,
			Description: "Fails when label app.kubernetes.io/instance and metadata.name mismatch.",
			Rego:        mustRego("metadataAndInstanceMismatched"),
		},
		"topologySpreadConstraint": {
			Key: "topologySpreadConstraint", DefaultMode: v1alpha1.ModeAudit, Match: matchWorkloadsNoDaemon,
			Description: "Fails when no topology spread constraint is defined.",
			Rego:        mustRego("topologySpreadConstraint"),
		},
		"hpaMaxAvailability": {
			Key: "hpaMaxAvailability", DefaultMode: v1alpha1.ModeAudit, Match: matchHPAs,
			Description: "Fails when maxReplicas is less than or equal to minReplicas.",
			Rego:        mustRego("hpaMaxAvailability"),
		},
		"hpaMinAvailability": {
			Key: "hpaMinAvailability", DefaultMode: v1alpha1.ModeAudit, Match: matchHPAs,
			Description: "Fails when minReplicas is less than or equal to 1.",
			Rego:        mustRego("hpaMinAvailability"),
		},
	}
}
