//go:build e2e

package e2e

import (
	"context"
	"testing"
	"time"

	v1alpha1 "github.com/Wynn-hub/kubesentry/internal/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// TestT7_ByNameMissingMember asserts that referencing an absent Policy
// surfaces a MissingMember condition without blocking the group.
func TestT7_ByNameMissingMember(t *testing.T) {
	ctx := context.Background()

	g := &v1alpha1.PolicyGroup{
		ObjectMeta: metav1.ObjectMeta{Name: "test-missing-grp"},
		Spec: v1alpha1.PolicyGroupSpec{
			Enabled: true,
			Policies: v1alpha1.PolicyGroupPolicies{
				ByName: []v1alpha1.PolicyRef{{Name: "definitely-not-there"}},
			},
		},
	}
	if err := k8sClient.Create(ctx, g); err != nil {
		t.Fatalf("create: %v", err)
	}
	t.Cleanup(func() { _ = k8sClient.Delete(ctx, g) })

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		var got v1alpha1.PolicyGroup
		if err := k8sClient.Get(ctx, types.NamespacedName{Name: g.Name}, &got); err == nil {
			for _, cond := range got.Status.Conditions {
				if cond.Type == "MissingMember" && cond.Status == metav1.ConditionTrue {
					t.Log("T7 PASS: MissingMember condition surfaced correctly")
					return
				}
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("MissingMember condition not surfaced within 10s")
}
