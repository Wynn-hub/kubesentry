package handler

import (
	"testing"

	"github.com/Wynn-hub/kubesentry/internal/api/v1alpha1"
	"github.com/Wynn-hub/kubesentry/internal/console/dto"
)

func TestPolicyStableMatchNilSlicesAdaptToOgen(t *testing.T) {
	match := dto.NormalizePolicyMatch(v1alpha1.PolicyMatch{Resources: []v1alpha1.MatchResource{{}}})
	detail, err := dtoData[dto.PolicyDetail](dto.PolicyView{Labels: map[string]string{}, Annotations: map[string]string{}, Match: match, ReferencedBy: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	if detail.Match.Operations == nil || detail.Match.Resources[0].ApiGroups == nil || detail.Match.Resources[0].ApiVersions == nil || detail.Match.Resources[0].Resources == nil {
		t.Fatalf("detail arrays were not normalized: %+v", detail.Match)
	}

	versions, err := dtoData[dto.PolicyVersionList](dto.PolicyVersionListView{Versions: []dto.PolicyVersionItemView{{Match: match, Phase: "Ready", EnforcementMode: "audit"}}})
	if err != nil {
		t.Fatal(err)
	}
	if versions.Versions[0].Match.Operations == nil {
		t.Fatal("version match operations is nil")
	}
}
