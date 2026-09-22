package dto

import (
	"encoding/json"
	"testing"
)

func TestDistributionInputDuplicateClustersReplacesEarlierValue(t *testing.T) {
	var input DistributionInput
	err := json.Unmarshal([]byte(`{"spec":{"policyGroups":[],"clusters":{"byName":["first"]},"clusters":{}}}`), &input)
	if err != nil {
		t.Fatal(err)
	}
	if len(input.Spec.Clusters.ByName) != 0 {
		t.Fatalf("clusters.byName = %v, want replacement by empty object", input.Spec.Clusters.ByName)
	}
}
