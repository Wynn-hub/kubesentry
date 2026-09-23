package gateway

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/Wynn-hub/kubesentry/api/kubesentry/v1alpha1"
	dbrepo "github.com/Wynn-hub/kubesentry/internal/database/repository"
	"github.com/Wynn-hub/kubesentry/internal/gatewaymodel"
)

// toV1alpha1Cluster/toV1alpha1Policies/toV1alpha1Groups/toV1alpha1Dists adapt
// dbrepo rows (Spec stored as datatypes.JSONType[T]) into the plain
// v1alpha1.X values Render expects — Render itself stays a pure function
// over in-memory slices, unaware its inputs now come from Postgres.

func toGatewayCluster(c *dbrepo.Cluster) *gatewaymodel.Cluster {
	return &gatewaymodel.Cluster{
		ObjectMeta: metav1.ObjectMeta{Name: c.Name},
		Spec:       gatewaymodel.ClusterSpec{Labels: c.Labels.Data()},
	}
}

func toV1alpha1Policies(in []dbrepo.Policy) []v1alpha1.Policy {
	out := make([]v1alpha1.Policy, len(in))
	for i, p := range in {
		out[i] = v1alpha1.Policy{
			ObjectMeta: metav1.ObjectMeta{Name: p.Name, Labels: p.Labels.Data(), Annotations: p.Annotations.Data()},
			Spec: v1alpha1.PolicySpec{
				Match: p.Match.Data(), EnforcementMode: p.EnforcementMode,
				Rego: p.Rego, Description: p.Description,
			},
		}
	}
	return out
}

func toV1alpha1Groups(in []dbrepo.PolicyGroup) []v1alpha1.PolicyGroup {
	out := make([]v1alpha1.PolicyGroup, len(in))
	for i, g := range in {
		out[i] = v1alpha1.PolicyGroup{
			ObjectMeta: metav1.ObjectMeta{Name: g.Name, Labels: g.Labels.Data(), Annotations: g.Annotations.Data()},
			Spec:       g.Spec.Data(),
		}
	}
	return out
}

func toGatewayDists(in []dbrepo.PolicyDistribution) []gatewaymodel.PolicyDistribution {
	out := make([]gatewaymodel.PolicyDistribution, len(in))
	for i, d := range in {
		out[i] = gatewaymodel.PolicyDistribution{ObjectMeta: metav1.ObjectMeta{Name: d.Name}, Spec: d.Spec.Data()}
	}
	return out
}

func toV1alpha1Exceptions(in []dbrepo.PolicyException) []v1alpha1.PolicyException {
	out := make([]v1alpha1.PolicyException, 0, len(in))
	for _, e := range in {
		out = append(out, v1alpha1.PolicyException{ObjectMeta: metav1.ObjectMeta{Name: e.Name, Labels: e.Labels.Data(), Annotations: e.Annotations.Data()}, Spec: e.Spec.Data()})
	}
	return out
}
