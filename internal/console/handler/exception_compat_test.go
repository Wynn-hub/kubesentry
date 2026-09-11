package handler

import (
	"testing"
	"time"

	"github.com/Wynn-hub/kubesentry/internal/api/v1alpha1"
	"github.com/Wynn-hub/kubesentry/internal/console/dto"
)

func TestExceptionStableDetailAdaptsToOgen(t *testing.T) {
	expiresAt := time.Now().Add(time.Hour)
	stable := dto.ExceptionView{Spec: v1alpha1.PolicyExceptionSpec{AllPolicies: true, Duration: "1h", Reason: "drill"}, Status: dto.ExceptionStatusView{Phase: "Active", EffectiveAt: time.Now(), ExpiresAt: &expiresAt}, Scope: dto.ExceptionScopeView{AllClusters: true, ClusterNames: []string{}}}
	converted, err := dtoData[dto.ExceptionDetail](stable)
	if err != nil {
		t.Fatal(err)
	}
	if err := converted.Validate(); err != nil {
		t.Fatalf("converted detail is invalid: %v", err)
	}
	if !converted.Spec.AllPolicies.Or(false) || converted.Status.Phase != dto.ExceptionStatusPhaseActive {
		t.Fatalf("converted detail = %+v", converted)
	}
}
