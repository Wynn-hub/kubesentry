package hub

import (
	"context"
	"crypto/subtle"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/Wynn-hub/kubesentry/internal/api/v1alpha1"
	"github.com/Wynn-hub/kubesentry/internal/hubpb"
)

// pushResync is the fallback interval for re-rendering a cluster's snapshot
// even without a change notification — a safety net against a missed/dropped
// Notify (e.g. a goroutine race at startup) rather than the primary trigger.
const pushResync = 60 * time.Second

// Server implements the hubpb.Hub gRPC service.
type Server struct {
	hubpb.UnimplementedHubServer
	client    client.Client
	namespace string
	hb        *HeartbeatTracker
	changes   *Notifier
}

func NewServer(c client.Client, namespace string, hb *HeartbeatTracker, changes *Notifier) *Server {
	return &Server{client: c, namespace: namespace, hb: hb, changes: changes}
}

func (s *Server) Register(ctx context.Context, req *hubpb.RegisterRequest) (*hubpb.RegisterResponse, error) {
	if req.ClusterName == "" || req.RegistrationToken == "" || req.ClusterUid == "" {
		return nil, status.Error(codes.InvalidArgument, "cluster_name, registration_token and cluster_uid are required")
	}

	var cl v1alpha1.Cluster
	if err := s.client.Get(ctx, types.NamespacedName{Name: req.ClusterName}, &cl); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, status.Error(codes.NotFound, "cluster not registered on hub")
		}
		return nil, status.Errorf(codes.Internal, "get cluster: %v", err)
	}

	// 校验一次性注册 token。
	var reg corev1.Secret
	if err := s.client.Get(ctx,
		types.NamespacedName{Namespace: s.namespace, Name: RegTokenSecretName(req.ClusterName)}, &reg); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, status.Error(codes.PermissionDenied, "no pending registration token for this cluster")
		}
		return nil, status.Errorf(codes.Internal, "get reg token: %v", err)
	}
	if subtle.ConstantTimeCompare(reg.Data[KeyToken], []byte(req.RegistrationToken)) != 1 {
		return nil, status.Error(codes.PermissionDenied, "invalid registration token")
	}

	// clusterUID 规则：跨 Cluster 唯一；重新接入必须 UID 一致。
	var all v1alpha1.ClusterList
	if err := s.client.List(ctx, &all); err != nil {
		return nil, status.Errorf(codes.Internal, "list clusters: %v", err)
	}
	for i := range all.Items {
		other := &all.Items[i]
		if other.Name != req.ClusterName && other.Status.ClusterUID == req.ClusterUid {
			return nil, status.Errorf(codes.AlreadyExists,
				"cluster UID already registered as %q", other.Name)
		}
	}
	if cl.Status.ClusterUID != "" && cl.Status.ClusterUID != req.ClusterUid {
		return nil, status.Error(codes.PermissionDenied,
			"cluster UID does not match previously registered identity")
	}

	// 签发长期 token：hub 只存哈希。
	token, err := NewToken()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "issue token: %v", err)
	}
	auth := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: s.namespace,
			Name:      AuthSecretName(req.ClusterName),
			Labels:    map[string]string{"kubesentry.io/cluster": req.ClusterName},
		},
		Data: map[string][]byte{
			KeyTokenHash:  []byte(HashToken(token)),
			KeyClusterUID: []byte(req.ClusterUid),
		},
	}
	if err := s.client.Create(ctx, auth); err != nil {
		if apierrors.IsAlreadyExists(err) {
			// 重新接入：轮换哈希。
			var existing corev1.Secret
			if err := s.client.Get(ctx, types.NamespacedName{Namespace: s.namespace, Name: auth.Name}, &existing); err != nil {
				return nil, status.Errorf(codes.Internal, "get auth secret: %v", err)
			}
			existing.Data = auth.Data
			if err := s.client.Update(ctx, &existing); err != nil {
				return nil, status.Errorf(codes.Internal, "rotate auth secret: %v", err)
			}
		} else {
			return nil, status.Errorf(codes.Internal, "create auth secret: %v", err)
		}
	}

	cl.Status.ClusterUID = req.ClusterUid
	cl.Status.OperatorVersion = req.OperatorVersion
	if cl.Status.Phase == "" || cl.Status.Phase == v1alpha1.ClusterPhaseRegistering {
		// 保持 Registering；Online 由首个心跳 flush 置位。
		cl.Status.Phase = v1alpha1.ClusterPhaseRegistering
	}
	if err := s.client.Status().Update(ctx, &cl); err != nil {
		return nil, status.Errorf(codes.Internal, "update cluster status: %v", err)
	}

	// 消费注册 token（一次性）：必须放在所有其他步骤成功之后再删除。
	// 如果先删 token 再提交 status，一旦 status 更新失败，token 已被
	// 消费但调用方从未拿到 ClusterToken —— 无法重试，只能等管理员手动
	// 干预。放在最后，即使 Delete 本身失败也只是残留一个失效 Secret，
	// 不影响已经成功的注册结果。
	if err := s.client.Delete(ctx, &reg); err != nil && !apierrors.IsNotFound(err) {
		return nil, status.Errorf(codes.Internal, "consume reg token: %v", err)
	}

	return &hubpb.RegisterResponse{ClusterToken: token}, nil
}

// validateSyncAuth 供 Task 6 的 Sync 使用：按 metadata 中的集群名 + token
// 校验长期凭据，返回集群名。
func (s *Server) validateSyncAuth(ctx context.Context, clusterName, token string) error {
	var auth corev1.Secret
	if err := s.client.Get(ctx,
		types.NamespacedName{Namespace: s.namespace, Name: AuthSecretName(clusterName)}, &auth); err != nil {
		return fmt.Errorf("cluster not registered")
	}
	if subtle.ConstantTimeCompare(auth.Data[KeyTokenHash], []byte(HashToken(token))) != 1 {
		return fmt.Errorf("invalid cluster token")
	}
	return nil
}

// syncSession serializes stream.Send between the recv loop's Acks and the
// pusher goroutine, and remembers the last pushed snapshot for StatusReport
// correlation.
type syncSession struct {
	mu      sync.Mutex
	stream  hubpb.Hub_SyncServer
	lastRes RenderResult
	hasLast bool
}

func (ss *syncSession) send(m *hubpb.HubMessage) error {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	return ss.stream.Send(m)
}

func (ss *syncSession) setLast(r RenderResult) {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	ss.lastRes, ss.hasLast = r, true
}

func (ss *syncSession) last() (RenderResult, bool) {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	return ss.lastRes, ss.hasLast
}

// renderFor lists the CRDs from the manager cache and renders one cluster's
// snapshot. Cluster-not-found renders the empty snapshot: a decommissioned
// cluster's managed objects cascade away.
func (s *Server) renderFor(ctx context.Context, cluster string) (RenderResult, error) {
	var cl v1alpha1.Cluster
	if err := s.client.Get(ctx, types.NamespacedName{Name: cluster}, &cl); err != nil {
		if !apierrors.IsNotFound(err) {
			return RenderResult{}, err
		}
		cl = v1alpha1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: cluster}}
	}
	var dists v1alpha1.PolicyDistributionList
	if err := s.client.List(ctx, &dists); err != nil {
		return RenderResult{}, err
	}
	var groups v1alpha1.PolicyGroupList
	if err := s.client.List(ctx, &groups); err != nil {
		return RenderResult{}, err
	}
	var policies v1alpha1.PolicyList
	if err := s.client.List(ctx, &policies); err != nil {
		return RenderResult{}, err
	}
	return Render(&cl, dists.Items, groups.Items, policies.Items), nil
}

// pushLoop renders and pushes a DesiredState snapshot immediately, then again
// whenever s.changes fires or pushResync elapses — but only sends when the
// content hash actually changed. Returns once the stream errors (the recv
// loop in Sync will observe the same broken stream and return too) or ctx is
// canceled (Sync's defer cancelPush on the way out).
func (s *Server) pushLoop(ctx context.Context, sess *syncSession, cluster string) {
	ticker := time.NewTicker(pushResync)
	defer ticker.Stop()
	for {
		if res, err := s.renderFor(ctx, cluster); err == nil {
			last, has := sess.last()
			if !has || last.Hash != res.Hash {
				if err := sess.send(&hubpb.HubMessage{Msg: &hubpb.HubMessage_DesiredState{
					DesiredState: &hubpb.DesiredState{VersionHash: res.Hash, Objects: ToProto(res.Objects)},
				}}); err != nil {
					return // stream already broken; recv loop will wind down too
				}
				sess.setLast(res)
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-s.changes.Wait():
		case <-ticker.C:
		}
	}
}

// Sync handles the long-lived member stream: auth from metadata once, then
// two concurrent halves share one stream — the recv loop below answers
// Heartbeat/StatusReport with an Ack, while a pushLoop goroutine renders and
// pushes DesiredState snapshots. Both funnel sends through sess.send's mutex
// since grpc.ServerStream.Send is not safe for concurrent use.
func (s *Server) Sync(stream hubpb.Hub_SyncServer) error {
	md, _ := metadata.FromIncomingContext(stream.Context())
	cluster := firstMD(md, MDClusterKey)
	token := strings.TrimPrefix(firstMD(md, MDAuthKey), "Bearer ")
	if cluster == "" || token == "" {
		return status.Error(codes.Unauthenticated, "missing cluster identity metadata")
	}
	if err := s.validateSyncAuth(stream.Context(), cluster, token); err != nil {
		return status.Error(codes.Unauthenticated, err.Error())
	}

	sess := &syncSession{stream: stream}
	pushCtx, cancelPush := context.WithCancel(stream.Context())
	defer cancelPush()
	go s.pushLoop(pushCtx, sess, cluster)

	for {
		msg, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		switch m := msg.Msg.(type) {
		case *hubpb.MemberMessage_Heartbeat:
			s.hb.MarkAlive(cluster, m.Heartbeat.OperatorVersion)
		case *hubpb.MemberMessage_StatusReport:
			// Task 6 在这里接 RecordStatus + markReported。
			_ = m
		}
		if err := sess.send(&hubpb.HubMessage{Msg: &hubpb.HubMessage_Ack{Ack: &hubpb.Ack{}}}); err != nil {
			return err
		}
	}
}

func firstMD(md metadata.MD, key string) string {
	if vs := md.Get(key); len(vs) > 0 {
		return vs[0]
	}
	return ""
}
