package dto

import (
	"encoding/json"
	"testing"
)

func TestAdmissionStoragePolicyValidatesFinalDuplicateEnumValues(t *testing.T) {
	data := []byte(`{"verdicts":["bad"],"verdicts":["audit"],"clusters":[],"gvks":[],"namespaces":[],"matchMode":"bad","matchMode":"include"}`)
	var policy AdmissionStoragePolicyView
	if err := json.Unmarshal(data, &policy); err != nil {
		t.Fatal(err)
	}
	if len(policy.Verdicts) != 1 || policy.Verdicts[0] != "audit" || policy.MatchMode != "include" {
		t.Fatalf("policy = %+v, want final duplicate values", policy)
	}
}
