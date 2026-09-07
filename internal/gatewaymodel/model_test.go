package gatewaymodel

import (
	"encoding/json"
	"reflect"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestGatewayModelsJSONRoundTrip(t *testing.T) {
	tests := []any{
		Cluster{
			ObjectMeta: metav1.ObjectMeta{Name: "prod"},
			Spec:       ClusterSpec{DisplayName: "Production", Labels: map[string]string{"env": "prod"}},
			Status:     ClusterStatus{Phase: ClusterPhaseOnline, PolicySummary: &PolicySummary{Ready: 2, Invalid: 1}},
		},
		PolicyDistribution{
			ObjectMeta: metav1.ObjectMeta{Name: "baseline"},
			Spec: PolicyDistributionSpec{
				Clusters:     DistributionClusters{ByName: []string{"prod"}},
				PolicyGroups: []string{"baseline"},
				Overrides:    []DistributionOverride{{Cluster: "prod", EnforcementMode: "Enforce"}},
			},
			Status: PolicyDistributionStatus{Clusters: []DistributionClusterStatus{{Name: "prod", State: DistributionSynced}}},
		},
	}

	for _, original := range tests {
		data, err := json.Marshal(original)
		if err != nil {
			t.Fatal(err)
		}
		decoded := reflect.New(reflect.TypeOf(original))
		if err := json.Unmarshal(data, decoded.Interface()); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(original, decoded.Elem().Interface()) {
			t.Fatalf("round trip changed model: %s", data)
		}
	}
}
