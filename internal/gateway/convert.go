package gateway

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/Wynn-hub/kubesentry/internal/api/v1alpha1"
	"github.com/Wynn-hub/kubesentry/internal/dbrepo"
)

// toV1alpha1Cluster/toV1alpha1Policies/toV1alpha1Groups/toV1alpha1Dists adapt
// dbrepo rows (Spec stored as datatypes.JSONType[T]) into the plain
// v1alpha1.X values Render expects — Render itself stays a pure function
// over in-memory slices, unaware its inputs now come from Postgres.

func toV1alpha1Cluster(c *dbrepo.Cluster) *v1alpha1.Cluster {
	return &v1alpha1.Cluster{
		ObjectMeta: metav1.ObjectMeta{Name: c.Name},
		Spec:       v1alpha1.ClusterSpec{Labels: c.Labels.Data()},
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

func toV1alpha1Dists(in []dbrepo.PolicyDistribution) []v1alpha1.PolicyDistribution {
	out := make([]v1alpha1.PolicyDistribution, len(in))
	for i, d := range in {
		out[i] = v1alpha1.PolicyDistribution{ObjectMeta: metav1.ObjectMeta{Name: d.Name}, Spec: d.Spec.Data()}
	}
	return out
}
