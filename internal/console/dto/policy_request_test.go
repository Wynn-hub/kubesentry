package dto

import (
	"encoding/json"
	"testing"
)

func TestPolicyInputValidatesFinalDuplicateEnums(t *testing.T) {
	data := []byte(`{"enforcementMode":"bad","enforcementMode":"audit","match":{"operations":["bad"],"operations":["CREATE"],"resources":[]},"rego":"package test"}`)
	var input PolicyInput
	if err := json.Unmarshal(data, &input); err != nil {
		t.Fatal(err)
	}
	if input.EnforcementMode != "audit" || len(input.Match.Operations) != 1 || input.Match.Operations[0] != "CREATE" {
		t.Fatalf("input = %+v, want final duplicate enum values", input)
	}
}
