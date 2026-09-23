package gateway

import (
	"context"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	dbrepo "github.com/Wynn-hub/kubesentry/internal/database/repository"
	"github.com/Wynn-hub/kubesentry/internal/gatewayauth"
	"github.com/Wynn-hub/kubesentry/internal/gatewaypb"
)

func TestDispatchToOfflineClusterReturnsUnavailable(t *testing.T) {
	repos := newTestRepos(t)
	srv := NewServer(repos, gatewayNS, NewHeartbeatTracker(), NewNotifier(), "test-token")

	_, err := srv.Dispatch(context.Background(), &gatewaypb.DispatchRequest{To: "not-connected", Topic: "task"})
	if status.Code(err) != codes.Unavailable {
		t.Fatalf("expected Unavailable for offline target, got %v", err)
	}
}

func TestDispatchRequiresTo(t *testing.T) {
	repos := newTestRepos(t)
	srv := NewServer(repos, gatewayNS, NewHeartbeatTracker(), NewNotifier(), "test-token")

	_, err := srv.Dispatch(context.Background(), &gatewaypb.DispatchRequest{Topic: "task"})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("expected InvalidArgument for empty to, got %v", err)
	}
}

// TestDispatchRoundTripsViaCorrelationID simulates a member replying on the
// Sync stream's MemberMessage.Envelope branch and confirms Dispatch's
// pending-correlation bookkeeping in syncSession.dispatch actually
// delivers that reply back to the caller.
func TestDispatchRoundTripsViaCorrelationID(t *testing.T) {
	repos := newTestRepos(t)
	mustCreateCluster(t, repos, &dbrepo.Cluster{Name: "prod", AuthTokenHash: gatewayauth.HashToken("tok"), ClusterUID: "u"})
	srv := NewServer(repos, gatewayNS, NewHeartbeatTracker(), NewNotifier(), "test-token")
	client := startBufconnGateway(t, srv)

	ctx := metadata.AppendToOutgoingContext(context.Background(),
		gatewaypb.MDClusterKey, "prod", gatewaypb.MDAuthKey, "Bearer tok")
	stream, err := client.Sync(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := stream.Send(&gatewaypb.MemberMessage{Msg: &gatewaypb.MemberMessage_Identity{
		Identity: &gatewaypb.MemberIdentity{Instance: "operator-0", Role: "leader"},
	}}); err != nil {
		t.Fatal(err)
	}

	// Member-side echo loop: whenever an Envelope arrives, reply with the
	// same correlation id and a fixed payload.
	go func() {
		for {
			msg, err := stream.Recv()
			if err != nil {
				return
			}
			env := msg.GetEnvelope()
			if env == nil {
				continue
			}
			_ = stream.Send(&gatewaypb.MemberMessage{Msg: &gatewaypb.MemberMessage_Envelope{
				Envelope: &gatewaypb.Envelope{CorrelationId: env.CorrelationId, Payload: []byte("pong")},
			}})
		}
	}()

	deadline := time.Now().Add(2 * time.Second)
	for {
		if _, ok := srv.sessions.get("prod"); ok {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("prod session never registered")
		}
		time.Sleep(10 * time.Millisecond)
	}

	resp, err := srv.Dispatch(context.Background(), &gatewaypb.DispatchRequest{To: "prod", Payload: []byte("ping")})
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if string(resp.Payload) != "pong" {
		t.Fatalf("resp.Payload = %q, want pong", resp.Payload)
	}
}

// TestSessionRegistryRaceSafeUnderConcurrentChurn exercises concurrent Sync
// connect/disconnect churn for the same cluster name alongside concurrent
// Dispatch calls — run with -race to catch any unsynchronized access to
// sessionRegistry or syncSession.pending.
func TestSessionRegistryRaceSafeUnderConcurrentChurn(t *testing.T) {
	repos := newTestRepos(t)
	mustCreateCluster(t, repos, &dbrepo.Cluster{Name: "prod", AuthTokenHash: gatewayauth.HashToken("tok"), ClusterUID: "u"})
	srv := NewServer(repos, gatewayNS, NewHeartbeatTracker(), NewNotifier(), "test-token")
	client := startBufconnGateway(t, srv)

	const iterations = 20
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
			sctx := metadata.AppendToOutgoingContext(ctx, gatewaypb.MDClusterKey, "prod", gatewaypb.MDAuthKey, "Bearer tok")
			stream, err := client.Sync(sctx)
			if err == nil {
				_, _ = stream.Recv() // let the session register before tearing down
			}
			cancel()
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
			_, _ = srv.Dispatch(ctx, &gatewaypb.DispatchRequest{To: "prod", Payload: []byte("x")})
			cancel()
		}
	}()

	wg.Wait()
}
