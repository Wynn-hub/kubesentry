package hub

import (
	"context"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/Wynn-hub/kubesentry/internal/api/v1alpha1"
	"github.com/Wynn-hub/kubesentry/internal/hubpb"
)

func startBufconnHub(t *testing.T, s *Server) hubpb.HubClient {
	t.Helper()
	lis := bufconn.Listen(1024 * 1024)
	gs := grpc.NewServer()
	hubpb.RegisterHubServer(gs, s)
	go func() { _ = gs.Serve(lis) }()
	t.Cleanup(gs.Stop)

	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return hubpb.NewHubClient(conn)
}

func TestSyncHeartbeatFlow(t *testing.T) {
	cl := &v1alpha1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "prod"}}
	c := newHubClient(t, cl, &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Namespace: hubNS, Name: AuthSecretName("prod")},
		Data: map[string][]byte{
			KeyTokenHash:  []byte(HashToken("secret-token")),
			KeyClusterUID: []byte("uid-1"),
		},
	})
	tr := NewHeartbeatTracker()
	client := startBufconnHub(t, NewServer(c, hubNS, tr, NewNotifier()))

	ctx := metadata.AppendToOutgoingContext(context.Background(),
		"x-kubesentry-cluster", "prod",
		"authorization", "Bearer secret-token")
	stream, err := client.Sync(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := stream.Send(&hubpb.MemberMessage{Msg: &hubpb.MemberMessage_Heartbeat{
		Heartbeat: &hubpb.Heartbeat{OperatorVersion: "v9"},
	}}); err != nil {
		t.Fatal(err)
	}
	// The pushLoop goroutine may deliver a DesiredState snapshot before or
	// interleaved with the recv loop's Ack — skip any DesiredState frames
	// and assert only on the Ack this test actually cares about.
	var reply *hubpb.HubMessage
	for {
		reply, err = stream.Recv()
		if err != nil {
			t.Fatal(err)
		}
		if reply.GetDesiredState() != nil {
			continue
		}
		break
	}
	if reply.GetAck() == nil {
		t.Fatalf("expected Ack, got %+v", reply)
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		if info, ok := tr.Snapshot()["prod"]; ok && info.OperatorVersion == "v9" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("heartbeat not recorded in tracker")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestSyncRejectsBadToken(t *testing.T) {
	c := newHubClient(t, &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Namespace: hubNS, Name: AuthSecretName("prod")},
		Data:       map[string][]byte{KeyTokenHash: []byte(HashToken("right"))},
	})
	client := startBufconnHub(t, NewServer(c, hubNS, NewHeartbeatTracker(), NewNotifier()))

	ctx := metadata.AppendToOutgoingContext(context.Background(),
		"x-kubesentry-cluster", "prod",
		"authorization", "Bearer wrong")
	stream, err := client.Sync(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = stream.Recv()
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("code = %v, want Unauthenticated", status.Code(err))
	}
}

// recvDesiredState drains the stream in a background goroutine until a
// DesiredState frame arrives (skipping Acks), or times out.
func recvDesiredState(t *testing.T, stream hubpb.Hub_SyncClient, timeout time.Duration) *hubpb.DesiredState {
	t.Helper()
	type item struct {
		ds  *hubpb.DesiredState
		err error
	}
	ch := make(chan item, 1)
	go func() {
		for {
			msg, err := stream.Recv()
			if err != nil {
				ch <- item{err: err}
				return
			}
			if ds := msg.GetDesiredState(); ds != nil {
				ch <- item{ds: ds}
				return
			}
		}
	}()
	select {
	case it := <-ch:
		if it.err != nil {
			t.Fatalf("recv: %v", it.err)
		}
		return it.ds
	case <-time.After(timeout):
		t.Fatal("no DesiredState received")
		return nil
	}
}

func TestSyncPushesSnapshotOnConnectAndChange(t *testing.T) {
	cl := &v1alpha1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "prod"}}
	pol := &v1alpha1.Policy{
		ObjectMeta: metav1.ObjectMeta{Name: "p1"},
		Spec:       v1alpha1.PolicySpec{EnforcementMode: v1alpha1.ModeAudit},
	}
	grp := &v1alpha1.PolicyGroup{
		ObjectMeta: metav1.ObjectMeta{Name: "g1"},
		Spec: v1alpha1.PolicyGroupSpec{
			Enabled:  true,
			Policies: v1alpha1.PolicyGroupPolicies{ByName: []v1alpha1.PolicyRef{{Name: "p1"}}},
		},
	}
	auth := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Namespace: hubNS, Name: AuthSecretName("prod")},
		Data:       map[string][]byte{KeyTokenHash: []byte(HashToken("tok")), KeyClusterUID: []byte("u")},
	}
	c := newHubClient(t, cl, pol, grp, auth)
	notifier := NewNotifier()
	client := startBufconnHub(t, NewServer(c, hubNS, NewHeartbeatTracker(), notifier))

	ctx := metadata.AppendToOutgoingContext(context.Background(),
		MDClusterKey, "prod", MDAuthKey, "Bearer tok")
	stream, err := client.Sync(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// 连接即收到空快照（无分发）。
	first := recvDesiredState(t, stream, 3*time.Second)
	if len(first.Objects) != 0 || first.VersionHash == "" {
		t.Fatalf("expected empty initial snapshot, got %+v", first)
	}

	// 建分发 → Notify → 收到含 2 对象的快照。
	dist := &v1alpha1.PolicyDistribution{
		ObjectMeta: metav1.ObjectMeta{Name: "d1"},
		Spec: v1alpha1.PolicyDistributionSpec{
			Clusters:     v1alpha1.DistributionClusters{ByName: []string{"prod"}},
			PolicyGroups: []string{"g1"},
		},
	}
	if err := c.Create(context.Background(), dist); err != nil {
		t.Fatal(err)
	}
	notifier.Notify()
	second := recvDesiredState(t, stream, 3*time.Second)
	if len(second.Objects) != 2 || second.VersionHash == first.VersionHash {
		t.Fatalf("expected 2-object snapshot with new hash, got %+v", second)
	}

	// 删分发 → 空快照（级联删除的协议表达）。
	if err := c.Delete(context.Background(), dist); err != nil {
		t.Fatal(err)
	}
	notifier.Notify()
	third := recvDesiredState(t, stream, 3*time.Second)
	if len(third.Objects) != 0 || third.VersionHash != first.VersionHash {
		t.Fatalf("expected empty snapshot again, got %+v", third)
	}
}
