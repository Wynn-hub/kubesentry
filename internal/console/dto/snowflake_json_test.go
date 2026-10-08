package dto

import (
	"encoding/json"
	"strings"
	"testing"

	dbrepo "github.com/Wynn-hub/kubesentry/internal/database/repository"
)

func TestSnowflakeIDsMarshalAsDecimalStrings(t *testing.T) {
	tenantID := int64(2108006645551247360)
	tests := []struct {
		name  string
		value any
		want  string
	}{
		{name: "tenant", value: TenantView{ID: tenantID}, want: `"id":"2108006645551247360"`},
		{name: "user", value: UserView{ID: tenantID + 1, TenantID: &tenantID}, want: `"tenantId":"2108006645551247360"`},
		{name: "current user", value: MeView{ID: tenantID + 1, TenantID: &tenantID}, want: `"tenantId":"2108006645551247360"`},
		{name: "cluster", value: ClusterItem{TenantID: &tenantID}, want: `"tenantId":"2108006645551247360"`},
		{name: "policy", value: PolicyItem{TenantID: &tenantID}, want: `"tenantId":"2108006645551247360"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload, err := json.Marshal(tt.value)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(payload), tt.want) {
				t.Fatalf("payload = %s, want it to contain %s", payload, tt.want)
			}
		})
	}
}

func TestSnowflakeTenantIDRequestsDecodeFromDecimalStrings(t *testing.T) {
	const payload = `{"username":"tenant-admin","password":"password123","role":"tenant_admin","tenantId":"2108006645551247360"}`
	var request CreateUserRequest
	if err := json.Unmarshal([]byte(payload), &request); err != nil {
		t.Fatal(err)
	}
	if request.Role != dbrepo.RoleTenantAdmin || request.TenantID == nil || *request.TenantID != 2108006645551247360 {
		t.Fatalf("request = %#v", request)
	}
}
