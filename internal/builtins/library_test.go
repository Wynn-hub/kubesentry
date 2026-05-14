package builtins_test

import (
	"strings"
	"testing"

	"github.com/Wynn-hub/kubesentry/internal/api/v1alpha1"
	"github.com/Wynn-hub/kubesentry/internal/builtins"
	"github.com/Wynn-hub/kubesentry/internal/webhook"
)

var expectedKeys = []string{
	// Security (23)
	"runAsPrivileged", "privilegeEscalationAllowed", "runAsRootAllowed",
	"notReadOnlyRootFilesystem", "linuxHardening", "insecureCapabilities",
	"dangerousCapabilities", "hostPIDSet", "hostIPCSet", "hostNetworkSet",
	"hostPortSet", "sensitiveContainerEnvVar", "automountServiceAccountToken",
	"sensitiveConfigmapContent", "tlsSettingsMissing",
	"clusterrolePodExecAttach", "rolePodExecAttach",
	"clusterrolebindingPodExecAttach", "rolebindingRolePodExecAttach",
	"rolebindingClusterRolePodExecAttach", "clusterrolebindingClusterAdmin",
	"rolebindingClusterAdminClusterRole", "rolebindingClusterAdminRole",
	// Efficiency (4)
	"cpuRequestsMissing", "memoryRequestsMissing",
	"cpuLimitsMissing", "memoryLimitsMissing",
	// Reliability (10)
	"readinessProbeMissing", "livenessProbeMissing", "tagNotSpecified",
	"pullPolicyNotAlways", "priorityClassNotSet", "deploymentMissingReplicas",
	"metadataAndInstanceMismatched", "topologySpreadConstraint",
	"hpaMaxAvailability", "hpaMinAvailability",
}

func TestLibraryHasAllKeys(t *testing.T) {
	for _, key := range expectedKeys {
		if _, ok := builtins.Library[key]; !ok {
			t.Errorf("library missing key: %q", key)
		}
	}
	if len(builtins.Library) != 37 {
		t.Errorf("expected 37 entries, got %d", len(builtins.Library))
	}
}

func TestLibraryRegoCompiles(t *testing.T) {
	for key, p := range builtins.Library {
		if _, err := webhook.CompileRego(p.Rego); err != nil {
			t.Errorf("key %q rego does not compile: %v", key, err)
		}
	}
}

func TestLibraryDefaultModeValid(t *testing.T) {
	for key, p := range builtins.Library {
		if p.DefaultMode != v1alpha1.ModeEnforce && p.DefaultMode != v1alpha1.ModeAudit {
			t.Errorf("key %q has invalid defaultMode: %q", key, p.DefaultMode)
		}
	}
}

func TestLibraryMatchNotEmpty(t *testing.T) {
	for key, p := range builtins.Library {
		if len(p.Match.Operations) == 0 {
			t.Errorf("key %q has empty operations", key)
		}
		if len(p.Match.Resources) == 0 {
			t.Errorf("key %q has empty resources", key)
		}
	}
}

func TestLibraryDescriptionNotEmpty(t *testing.T) {
	for key, p := range builtins.Library {
		if strings.TrimSpace(p.Description) == "" {
			t.Errorf("key %q has empty description", key)
		}
	}
}
