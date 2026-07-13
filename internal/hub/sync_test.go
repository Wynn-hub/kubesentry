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
	client := startBufconnHub(t, NewServer(c, hubNS, tr))

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
	reply, err := stream.Recv()
	if err != nil {
		t.Fatal(err)
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
	client := startBufconnHub(t, NewServer(c, hubNS, NewHeartbeatTracker()))

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
