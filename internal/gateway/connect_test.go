package gateway

import (
	"context"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"github.com/Wynn-hub/kubesentry/internal/gatewaypb"
)

func TestNotifyChangeTriggersNotifier(t *testing.T) {
	repos := newTestRepos(t)
	notifier := NewNotifier()
	srv := NewServer(repos, gatewayNS, NewHeartbeatTracker(), notifier, "test-token")

	waitCh := notifier.Wait()
	if _, err := srv.NotifyChange(context.Background(), &gatewaypb.NotifyChangeRequest{Topic: "policy"}); err != nil {
		t.Fatalf("NotifyChange: %v", err)
	}
	select {
	case <-waitCh:
	default:
		t.Fatal("expected NotifyChange to fire the Notifier")
	}
}

func TestConnectRejectsBadToken(t *testing.T) {
	repos := newTestRepos(t)
	client := startBufconnGateway(t, NewServer(repos, gatewayNS, NewHeartbeatTracker(), NewNotifier(), "right-token"))

	stream, err := client.Connect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	payload, err := proto.Marshal(&gatewaypb.ConnectRequest{ComponentId: "console-api", SharedToken: "wrong-token"})
	if err != nil {
		t.Fatal(err)
	}
	if err := stream.Send(&gatewaypb.Envelope{Payload: payload}); err != nil {
		t.Fatal(err)
	}
	_, err = stream.Recv()
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("code = %v, want Unauthenticated", status.Code(err))
	}
}

func TestConnectAcceptsValidTokenAndRegisters(t *testing.T) {
	repos := newTestRepos(t)
	srv := NewServer(repos, gatewayNS, NewHeartbeatTracker(), NewNotifier(), "right-token")
	client := startBufconnGateway(t, srv)

	stream, err := client.Connect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	payload, err := proto.Marshal(&gatewaypb.ConnectRequest{ComponentId: "console-api", SharedToken: "right-token"})
	if err != nil {
		t.Fatal(err)
	}
	if err := stream.Send(&gatewaypb.Envelope{Payload: payload}); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		if _, ok := srv.connected.get("console-api"); ok {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("console-api never registered in connectRegistry")
		}
		time.Sleep(10 * time.Millisecond)
	}

	if err := stream.CloseSend(); err != nil {
		t.Fatal(err)
	}
}
