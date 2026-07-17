package helpers

import (
	"context"
	"encoding/base64"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1alpha1 "github.com/Wynn-hub/kubesentry/internal/api/v1alpha1"
)

// HarvestRegistration creates the "local" Cluster CR on an already-running
// gateway and returns the one-time registration token and base64 gateway CA
// bundle needed to install the member chart pre-pointed at the gateway. Must
// run after the gateway chart is installed and before the member chart is
// installed — gateway.address/caBundle/clusterName/registrationToken are
// chart-required, so there is no "install member bare, then helm upgrade
// with real values" path anymore.
func HarvestRegistration(ctx context.Context, c client.Client) (token, caBundleB64 string, err error) {
	cl := &v1alpha1.Cluster{}
	cl.Name = "local"
	if err := c.Create(ctx, cl); err != nil && !apierrors.IsAlreadyExists(err) {
		return "", "", fmt.Errorf("create Cluster: %w", err)
	}

	if err := wait.PollUntilContextTimeout(ctx, 2*time.Second, time.Minute, false, func(ctx context.Context) (bool, error) {
		var sec corev1.Secret
		if err := c.Get(ctx, types.NamespacedName{Namespace: WebhookNamespace, Name: "kubesentry-reg-local"}, &sec); err != nil {
			return false, nil
		}
		token = string(sec.Data["token"])
		return token != "", nil
	}); err != nil {
		return "", "", fmt.Errorf("registration token not issued: %w", err)
	}

	var tlsSec corev1.Secret
	if err := c.Get(ctx, types.NamespacedName{Namespace: WebhookNamespace, Name: "kubesentry-gateway-tls"}, &tlsSec); err != nil {
		return "", "", fmt.Errorf("read gateway tls secret: %w", err)
	}
	caBundleB64 = base64.StdEncoding.EncodeToString(tlsSec.Data["ca.crt"])
	return token, caBundleB64, nil
}

// WaitClusterOnline polls until the named Cluster reaches Online phase.
func WaitClusterOnline(ctx context.Context, c client.Client, name string, timeout time.Duration) error {
	return wait.PollUntilContextTimeout(ctx, 5*time.Second, timeout, false, func(ctx context.Context) (bool, error) {
		var got v1alpha1.Cluster
		if err := c.Get(ctx, types.NamespacedName{Name: name}, &got); err != nil {
			return false, nil
		}
		return got.Status.Phase == v1alpha1.ClusterPhaseOnline, nil
	})
}
