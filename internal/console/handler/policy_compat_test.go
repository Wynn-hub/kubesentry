package handler

import (
	"testing"

	"github.com/Wynn-hub/kubesentry/api/kubesentry/v1alpha1"
	"github.com/Wynn-hub/kubesentry/internal/console/dto"
)

func TestNormalizePolicyMatchFillsNilSlices(t *testing.T) {
	match := dto.NormalizePolicyMatch(v1alpha1.PolicyMatch{Resources: []v1alpha1.MatchResource{{}}})
	if match.Operations == nil || match.Resources[0].APIGroups == nil || match.Resources[0].APIVersions == nil || match.Resources[0].Resources == nil {
		t.Fatalf("match arrays were not normalized: %+v", match)
	}
}
