package v1alpha1

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestPolicyDistributionDeepCopyIsIndependent(t *testing.T) {
	now := metav1.Now()
	orig := &PolicyDistribution{
		ObjectMeta: metav1.ObjectMeta{Name: "d1"},
		Spec: PolicyDistributionSpec{
			Clusters: DistributionClusters{
				ByName:          []string{"c1"},
				ClusterSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"env": "prod"}},
			},
			PolicyGroups: []string{"g1"},
			Overrides:    []DistributionOverride{{Cluster: "c1", EnforcementMode: ModeAudit}},
		},
		Status: PolicyDistributionStatus{
			Clusters: []DistributionClusterStatus{{Name: "c1", State: DistributionSynced, LastSyncTime: &now}},
		},
	}
	cp := orig.DeepCopy()
	cp.Spec.Clusters.ByName[0] = "mutated"
	cp.Spec.Clusters.ClusterSelector.MatchLabels["env"] = "mutated"
	cp.Spec.PolicyGroups[0] = "mutated"
	cp.Spec.Overrides[0].Cluster = "mutated"
	cp.Status.Clusters[0].State = "mutated"

	if orig.Spec.Clusters.ByName[0] != "c1" ||
		orig.Spec.Clusters.ClusterSelector.MatchLabels["env"] != "prod" ||
		orig.Spec.PolicyGroups[0] != "g1" ||
		orig.Spec.Overrides[0].Cluster != "c1" ||
		orig.Status.Clusters[0].State != DistributionSynced {
		t.Fatal("DeepCopy shares memory with original")
	}
}

func TestClusterStatusPolicySummaryDeepCopy(t *testing.T) {
	orig := &ClusterStatus{PolicySummary: &PolicySummary{Ready: 3, Invalid: 1}}
	cp := orig.DeepCopy()
	cp.PolicySummary.Ready = 99
	if orig.PolicySummary.Ready != 3 {
		t.Fatal("PolicySummary shared between copies")
	}
}
