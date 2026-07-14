// Package hublink is the member-cluster side of the hub connection:
// one-time registration, then a heartbeat loop over the Sync stream with
// exponential-backoff reconnect. It runs as a manager Runnable inside the
// operator; when no hub address is configured the operator never starts it,
// which is the entire single-cluster compatibility story.
package hublink

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/Wynn-hub/kubesentry/internal/hubpb"
)

const (
	registerSecretName = "kubesentry-hub-register"
	authSecretName     = "kubesentry-hub-auth"
	keyToken           = "token"
	keyClusterToken    = "clusterToken"

	maxBackoff = 60 * time.Second
)

type Config struct {
	HubAddress      string // host:port；空 = 不启用（调用方保证）
	TLSCAFile       string // hub CA 证书路径；空 = 系统根证书
	TLSServerName   string // 证书 DNS 名覆盖（经 NodePort/LB 直连 IP 时必填）
	ClusterName     string
	Namespace       string // operator 自身 namespace（Secret 所在）
	OperatorVersion string
	Reader          client.Reader // 非缓存读（kube-system UID、Secret）
	Client          client.Client // 写成员侧 auth Secret
	HeartbeatEvery  time.Duration
	// DialOverride 仅测试注入 bufconn；nil 时走真实 TLS 拨号。
	DialOverride func(ctx context.Context) (*grpc.ClientConn, error)
}

type Link struct{ cfg Config }

func New(cfg Config) *Link {
	if cfg.HeartbeatEvery == 0 {
		cfg.HeartbeatEvery = 15 * time.Second
	}
	return &Link{cfg: cfg}
}

// NeedLeaderElection: only the elected operator holds the hub link.
func (l *Link) NeedLeaderElection() bool { return true }

func (l *Link) Start(ctx context.Context) error {
	backoff := time.Second
	for {
		connected, err := l.runOnce(ctx)
		if ctx.Err() != nil {
			return nil
		}
		if connected {
			// A session that reached "connected" was a healthy connection,
			// not a persistent failure — start the next retry fast.
			backoff = time.Second
		}
		slog.Warn("hub link disconnected, will retry", "error", err, "backoff", backoff)
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(backoff):
		}
		if backoff *= 2; backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

// runOnce dials, registers, and runs the heartbeat loop until it fails.
// connected reports whether the Sync stream was ever successfully opened,
// regardless of what happened afterwards — Start uses it to decide whether
// to reset the reconnect backoff.
func (l *Link) runOnce(ctx context.Context) (connected bool, err error) {
	conn, err := l.dial(ctx)
	if err != nil {
		return false, fmt.Errorf("dial hub: %w", err)
	}
	defer conn.Close()
	hc := hubpb.NewHubClient(conn)

	token, err := l.ensureRegistered(ctx, hc)
	if err != nil {
		return false, err
	}

	streamCtx := metadata.AppendToOutgoingContext(ctx,
		"x-kubesentry-cluster", l.cfg.ClusterName,
		"authorization", "Bearer "+token)
	stream, err := hc.Sync(streamCtx)
	if err != nil {
		l.clearCredentialIfUnauthenticated(ctx, err)
		return false, fmt.Errorf("open sync stream: %w", err)
	}
	slog.Info("hub link established", "hub", l.cfg.HubAddress, "cluster", l.cfg.ClusterName)

	ticker := time.NewTicker(l.cfg.HeartbeatEvery)
	defer ticker.Stop()
	for {
		if err := stream.Send(&hubpb.MemberMessage{Msg: &hubpb.MemberMessage_Heartbeat{
			Heartbeat: &hubpb.Heartbeat{OperatorVersion: l.cfg.OperatorVersion},
		}}); err != nil {
			l.clearCredentialIfUnauthenticated(ctx, err)
			return true, fmt.Errorf("send heartbeat: %w", err)
		}
		// Server-side Sync validates auth before entering its recv loop and
		// returns codes.Unauthenticated immediately on a stale/rotated
		// token — that rejection surfaces here, on the first Recv after a
		// successful Send, not from Sync() or Send() themselves.
		if _, err := stream.Recv(); err != nil {
			l.clearCredentialIfUnauthenticated(ctx, err)
			return true, fmt.Errorf("recv ack: %w", err)
		}
		select {
		case <-ctx.Done():
			return true, nil
		case <-ticker.C:
		}
	}
}

func (l *Link) dial(ctx context.Context) (*grpc.ClientConn, error) {
	if l.cfg.DialOverride != nil {
		return l.cfg.DialOverride(ctx)
	}
	creds, err := credentials.NewClientTLSFromFile(l.cfg.TLSCAFile, l.cfg.TLSServerName)
	if err != nil {
		return nil, fmt.Errorf("load hub ca: %w", err)
	}
	return grpc.NewClient(l.cfg.HubAddress, grpc.WithTransportCredentials(creds))
}

// ensureRegistered returns the long-lived cluster token, registering with
// the one-time token first if no stored credential exists yet.
func (l *Link) ensureRegistered(ctx context.Context, hc hubpb.HubClient) (string, error) {
	var auth corev1.Secret
	err := l.cfg.Reader.Get(ctx,
		types.NamespacedName{Namespace: l.cfg.Namespace, Name: authSecretName}, &auth)
	if err == nil && len(auth.Data[keyClusterToken]) > 0 {
		return string(auth.Data[keyClusterToken]), nil
	}
	if err != nil && !apierrors.IsNotFound(err) {
		return "", fmt.Errorf("read auth secret: %w", err)
	}

	var reg corev1.Secret
	if err := l.cfg.Reader.Get(ctx,
		types.NamespacedName{Namespace: l.cfg.Namespace, Name: registerSecretName}, &reg); err != nil {
		return "", fmt.Errorf("no stored credential and no registration token secret: %w", err)
	}

	uid, err := l.clusterUID(ctx)
	if err != nil {
		return "", err
	}
	resp, err := hc.Register(ctx, &hubpb.RegisterRequest{
		ClusterName:       l.cfg.ClusterName,
		RegistrationToken: string(reg.Data[keyToken]),
		ClusterUid:        uid,
		OperatorVersion:   l.cfg.OperatorVersion,
	})
	if err != nil {
		return "", fmt.Errorf("register: %w", err)
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Namespace: l.cfg.Namespace, Name: authSecretName},
		Data:       map[string][]byte{keyClusterToken: []byte(resp.ClusterToken)},
	}
	if err := l.cfg.Client.Create(ctx, secret); err != nil {
		if apierrors.IsAlreadyExists(err) {
			var existing corev1.Secret
			if err := l.cfg.Reader.Get(ctx, types.NamespacedName{Namespace: l.cfg.Namespace, Name: authSecretName}, &existing); err != nil {
				return "", err
			}
			existing.Data = secret.Data
			if err := l.cfg.Client.Update(ctx, &existing); err != nil {
				return "", fmt.Errorf("update auth secret: %w", err)
			}
		} else {
			return "", fmt.Errorf("store cluster token: %w", err)
		}
	}
	slog.Info("registered with hub", "cluster", l.cfg.ClusterName)
	return resp.ClusterToken, nil
}

// clearCredentialIfUnauthenticated deletes the stored hub-auth Secret when
// err is a definitive codes.Unauthenticated rejection from the hub — meaning
// the token itself was rejected (e.g. an admin rotated it via the documented
// re-registration recovery: delete the hub-side auth Secret). Clearing it
// lets the next retry's ensureRegistered fall through to the registration
// flow instead of looping forever presenting the same dead token. Other
// error types (network errors, context cancellation, etc.) don't imply an
// invalid token and are left untouched.
func (l *Link) clearCredentialIfUnauthenticated(ctx context.Context, err error) {
	if status.Code(err) != codes.Unauthenticated {
		return
	}
	if clearErr := l.clearStoredCredential(ctx); clearErr != nil {
		slog.Warn("failed to clear stale hub credential after unauthenticated rejection", "error", clearErr)
	}
}

// clearStoredCredential deletes the member-side auth Secret; already-gone is
// treated as success.
func (l *Link) clearStoredCredential(ctx context.Context) error {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Namespace: l.cfg.Namespace, Name: authSecretName},
	}
	if err := l.cfg.Client.Delete(ctx, secret); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("delete stale auth secret: %w", err)
	}
	slog.Info("cleared stale hub credential; will re-register on next retry", "cluster", l.cfg.ClusterName)
	return nil
}

// clusterUID is the kube-system namespace UID — the stable machine
// identity of this cluster (spec D6).
func (l *Link) clusterUID(ctx context.Context) (string, error) {
	var ns corev1.Namespace
	if err := l.cfg.Reader.Get(ctx, types.NamespacedName{Name: "kube-system"}, &ns); err != nil {
		return "", fmt.Errorf("read kube-system uid: %w", err)
	}
	return string(ns.UID), nil
}
