package helpers

import (
	"context"
	"encoding/base64"
	"fmt"
	"path/filepath"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1alpha1 "github.com/Wynn-hub/kubesentry/internal/api/v1alpha1"
)

// RegisterLocalCluster runs the full self-registration flow: create the
// Cluster CR, harvest the one-time token and hub CA, re-point the member
// release at the hub, then wait for Online.
func RegisterLocalCluster(ctx context.Context, c client.Client) error {
	cl := &v1alpha1.Cluster{}
	cl.Name = "local"
	if err := c.Create(ctx, cl); err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("create Cluster: %w", err)
	}

	var token string
	if err := wait.PollUntilContextTimeout(ctx, 2*time.Second, time.Minute, false, func(ctx context.Context) (bool, error) {
		var sec corev1.Secret
		if err := c.Get(ctx, types.NamespacedName{Namespace: WebhookNamespace, Name: "kubesentry-reg-local"}, &sec); err != nil {
			return false, nil
		}
		token = string(sec.Data["token"])
		return token != "", nil
	}); err != nil {
		return fmt.Errorf("registration token not issued: %w", err)
	}

	var tlsSec corev1.Secret
	if err := c.Get(ctx, types.NamespacedName{Namespace: WebhookNamespace, Name: "kubesentry-hub-tls"}, &tlsSec); err != nil {
		return fmt.Errorf("read hub tls secret: %w", err)
	}
	caB64 := base64.StdEncoding.EncodeToString(tlsSec.Data["ca.crt"])

	chartDir := filepath.Join("..", "..", "charts", "kubesentry")
	if err := HelmUpgradeMemberWithHub(ctx, chartDir, caB64, token); err != nil {
		return err
	}
	if err := WaitForDeploymentReady(ctx, c, WebhookNamespace, "kubesentry-operator", 3*time.Minute); err != nil {
		return err
	}

	return wait.PollUntilContextTimeout(ctx, 5*time.Second, 3*time.Minute, false, func(ctx context.Context) (bool, error) {
		var got v1alpha1.Cluster
		if err := c.Get(ctx, types.NamespacedName{Name: "local"}, &got); err != nil {
			return false, nil
		}
		return got.Status.Phase == v1alpha1.ClusterPhaseOnline, nil
	})
}
