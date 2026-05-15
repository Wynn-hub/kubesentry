package helpers

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const WebhookNamespace = "kubesentry-system"

// GetWebhookPodName returns the name of the first running webhook pod.
func GetWebhookPodName(ctx context.Context, c client.Client) (string, error) {
	var podList corev1.PodList
	if err := c.List(ctx, &podList,
		client.InNamespace(WebhookNamespace),
		client.MatchingLabels{"app": "kubesentry-webhook"},
	); err != nil {
		return "", fmt.Errorf("list webhook pods: %w", err)
	}
	for _, p := range podList.Items {
		if p.Status.Phase == corev1.PodRunning {
			return p.Name, nil
		}
	}
	return "", fmt.Errorf("no running webhook pod found in %s", WebhookNamespace)
}

// GetWebhookLogs returns the webhook container logs since the given time.
func GetWebhookLogs(ctx context.Context, cs *kubernetes.Clientset, podName string, since time.Time) (string, error) {
	sinceTime := metav1.NewTime(since)
	req := cs.CoreV1().Pods(WebhookNamespace).GetLogs(podName, &corev1.PodLogOptions{
		Container: "webhook",
		SinceTime: &sinceTime,
	})
	rc, err := req.Stream(ctx)
	if err != nil {
		return "", fmt.Errorf("stream logs for pod %s: %w", podName, err)
	}
	defer rc.Close()
	b, err := io.ReadAll(rc)
	if err != nil {
		return "", fmt.Errorf("read logs for pod %s: %w", podName, err)
	}
	return string(b), nil
}

// WaitForLogEntry polls until the webhook logs contain keyword or the context is cancelled.
func WaitForLogEntry(ctx context.Context, cs *kubernetes.Clientset, podName, keyword string, since time.Time, timeout time.Duration) error {
	return wait.PollUntilContextTimeout(ctx, 2*time.Second, timeout, false, func(ctx context.Context) (bool, error) {
		logs, err := GetWebhookLogs(ctx, cs, podName, since)
		if err != nil {
			return false, nil
		}
		return strings.Contains(logs, keyword), nil
	})
}

// AssertNoLogEntry waits window duration and confirms keyword never appears in logs.
func AssertNoLogEntry(ctx context.Context, cs *kubernetes.Clientset, podName, keyword string, since time.Time, window time.Duration) error {
	deadline := time.Now().Add(window)
	for time.Now().Before(deadline) {
		logs, err := GetWebhookLogs(ctx, cs, podName, since)
		if err != nil {
			time.Sleep(time.Second)
			continue
		}
		if strings.Contains(logs, keyword) {
			return fmt.Errorf("unexpected log entry found: %q", keyword)
		}
		time.Sleep(time.Second)
	}
	return nil
}
