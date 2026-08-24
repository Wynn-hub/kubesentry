package handler

import (
	"encoding/json"
	"reflect"
	"testing"

	"gorm.io/datatypes"

	"github.com/Wynn-hub/kubesentry/internal/dbmodel"
	"github.com/Wynn-hub/kubesentry/internal/dbrepo"
)

func TestAdmissionItemFallsBackToViolationPolicyReferences(t *testing.T) {
	violations, err := json.Marshal([]string{"[host-ipc-set via default] host IPC is not allowed"})
	if err != nil {
		t.Fatal(err)
	}

	item := admissionItem(dbrepo.AdmissionEvent{Base: dbmodel.Base{ID: 2081582208838783001}, Violations: datatypes.JSON(violations)})
	if item.ID != "2081582208838783001" {
		t.Errorf("event ID = %q, want exact Snowflake ID", item.ID)
	}
	if want := []string{"host-ipc-set"}; !reflect.DeepEqual(item.PolicyNames, want) {
		t.Errorf("policy names = %#v, want %#v", item.PolicyNames, want)
	}
}
