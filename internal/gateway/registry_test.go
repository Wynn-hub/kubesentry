package gateway

import (
	"context"
	"testing"

	"google.golang.org/grpc/metadata"

	"github.com/Wynn-hub/kubesentry/internal/gatewaypb"
)

// fakeConnectStream is the minimal gatewaypb.Hub_ConnectServer implementation
// needed to get two distinguishable pointer identities for
// connectRegistry's compare-and-delete test — no real gRPC transport
// involved.
type fakeConnectStream struct{ tag string }

func (f *fakeConnectStream) Recv() (*gatewaypb.Envelope, error) { return nil, nil }
func (f *fakeConnectStream) Send(*gatewaypb.Envelope) error     { return nil }
func (f *fakeConnectStream) SetHeader(metadata.MD) error        { return nil }
func (f *fakeConnectStream) SendHeader(metadata.MD) error       { return nil }
func (f *fakeConnectStream) SetTrailer(metadata.MD)             {}
func (f *fakeConnectStream) Context() context.Context           { return context.Background() }
func (f *fakeConnectStream) SendMsg(m any) error                { return nil }
func (f *fakeConnectStream) RecvMsg(m any) error                { return nil }

// TestSessionRegistryRemoveIsCompareAndDelete proves remove() no longer does
// an unconditioned delete-by-key: a stale session's cleanup must not evict a
// newer session that already replaced it under the same name. This is the
// scenario TestSessionRegistryRaceSafeUnderConcurrentChurn couldn't catch
// because its connect loop was sequential-with-blocking-Recv and never
// forced put(new) to land before remove(old).
func TestSessionRegistryRemoveIsCompareAndDelete(t *testing.T) {
	r := newSessionRegistry()
	oldSess := &syncSession{}
	newSess := &syncSession{}

	r.put("prod", "operator-0", "leader", oldSess)
	// Reconnect races in and replaces the entry before the old session's
	// deferred remove() runs.
	r.put("prod", "operator-0", "leader", newSess)

	// Old session's cleanup fires with a stale reference — must be a no-op.
	r.remove("prod", "operator-0", oldSess)
	got, ok := r.leader("prod")
	if !ok || got != newSess {
		t.Fatalf("remove(name, stale) evicted the live session: got=%v ok=%v, want newSess", got, ok)
	}

	// The current session's own cleanup still works.
	r.remove("prod", "operator-0", newSess)
	if _, ok := r.leader("prod"); ok {
		t.Fatal("remove(name, current) should have deleted the entry")
	}
}

func TestSessionRegistryStandbyDoesNotReplaceLeader(t *testing.T) {
	r := newSessionRegistry()
	leader := &syncSession{}
	standby := &syncSession{}
	r.put("prod", "operator-0", "leader", leader)
	r.put("prod", "operator-1", "standby", standby)

	got, ok := r.leader("prod")
	if !ok || got != leader {
		t.Fatalf("leader = %v, %v; want original leader", got, ok)
	}
}

func TestSessionRegistryNewestLeaderClaimWins(t *testing.T) {
	r := newSessionRegistry()
	oldLeader := &syncSession{}
	newLeader := &syncSession{}
	r.put("prod", "operator-0", "leader", oldLeader)
	demoted := r.put("prod", "operator-1", "leader", newLeader)

	got, ok := r.leader("prod")
	if !ok || got != newLeader {
		t.Fatalf("leader = %v, %v; want newest leader", got, ok)
	}
	if len(demoted) != 1 || demoted[0] != oldLeader {
		t.Fatalf("demoted = %+v, want old leader", demoted)
	}
}

// TestConnectRegistryUnregisterIsCompareAndDelete mirrors the sessionRegistry
// test for connectRegistry's unregister().
func TestConnectRegistryUnregisterIsCompareAndDelete(t *testing.T) {
	r := newConnectRegistry()
	oldStream := &fakeConnectStream{tag: "old"}
	newStream := &fakeConnectStream{tag: "new"}

	r.register("console-api", oldStream)
	r.register("console-api", newStream)

	r.unregister("console-api", oldStream)
	got, ok := r.get("console-api")
	if !ok || got != gatewaypb.Hub_ConnectServer(newStream) {
		t.Fatalf("unregister(id, stale) evicted the live stream: got=%v ok=%v, want newStream", got, ok)
	}

	r.unregister("console-api", newStream)
	if _, ok := r.get("console-api"); ok {
		t.Fatal("unregister(id, current) should have deleted the entry")
	}
}
