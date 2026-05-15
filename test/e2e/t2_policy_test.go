//go:build e2e

package e2e

import (
	"context"
	"testing"
	"time"

	v1alpha1 "github.com/Wynn-hub/kubesentry/internal/api/v1alpha1"
	"github.com/Wynn-hub/kubesentry/test/e2e/helpers"
	admregv1 "k8s.io/api/admissionregistration/v1"
	"k8s.io/apimachinery/pkg/types"
)

func TestT2_PolicyGroupsSynced(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	for _, group := range []string{"security", "efficiency", "reliability"} {
		var pg v1alpha1.PolicyGroup
		if err := k8sClient.Get(ctx, types.NamespacedName{Name: group}, &pg); err != nil {
			t.Errorf("PolicyGroup %s not found: %v", group, err)
		}
	}
}

func TestT2_AllPoliciesReady(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	if err := helpers.WaitForAllPoliciesReady(ctx, k8sClient, 90*time.Second); err != nil {
		var list v1alpha1.PolicyList
		_ = k8sClient.List(ctx, &list)
		for _, p := range list.Items {
			if p.Status.Phase != v1alpha1.PhaseReady {
				t.Logf("Policy %s is in phase %s", p.Name, p.Status.Phase)
			}
		}
		t.Fatalf("not all policies reached Ready phase within 90s: %v", err)
	}
}

func TestT2_PolicyVersionsCreated(t *testing.T) {
	ctx := context.Background()
	var list v1alpha1.PolicyList
	if err := k8sClient.List(ctx, &list); err != nil {
		t.Fatalf("list policies: %v", err)
	}
	for _, p := range list.Items {
		count, err := helpers.CountPolicyVersions(ctx, k8sClient, p.Name)
		if err != nil {
			t.Errorf("policy %s: count versions: %v", p.Name, err)
			continue
		}
		if count == 0 {
			t.Errorf("policy %s: expected at least 1 PolicyVersion, got 0", p.Name)
		}
	}
}

func TestT2_VWCRulesPopulated(t *testing.T) {
	ctx := context.Background()
	var vwc admregv1.ValidatingWebhookConfiguration
	if err := k8sClient.Get(ctx, types.NamespacedName{Name: "kubesentry"}, &vwc); err != nil {
		t.Fatalf("get VWC: %v", err)
	}
	if len(vwc.Webhooks) == 0 {
		t.Fatal("VWC has no webhooks")
	}
	if len(vwc.Webhooks[0].Rules) == 0 {
		t.Fatal("VWC webhook[0].Rules is empty — operator may not have synced policies yet")
	}
}
