package dto

import (
	"encoding/json"
	"testing"
)

func TestPolicyGroupInputValidatesFinalDuplicateEnumValues(t *testing.T) {
	data := []byte(`{"enabled":true,"policies":{"byName":[{"name":"p","enforcementMode":"bad","enforcementMode":"audit"}]},"selectorEnforcementMode":"bad","selectorEnforcementMode":"enforce"}`)
	var input PolicyGroupInput
	if err := json.Unmarshal(data, &input); err != nil {
		t.Fatal(err)
	}
	if input.SelectorEnforcementMode == nil || *input.SelectorEnforcementMode != "enforce" || input.Policies.ByName[0].EnforcementMode == nil || *input.Policies.ByName[0].EnforcementMode != "audit" {
		t.Fatalf("input = %+v, want final duplicate enum values", input)
	}
}
