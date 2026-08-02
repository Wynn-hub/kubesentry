package gateway

import (
	"context"
	"testing"
	"time"

	"github.com/Wynn-hub/kubesentry/internal/dbrepo"
	"github.com/Wynn-hub/kubesentry/internal/gatewaypb"
	"github.com/Wynn-hub/kubesentry/internal/platformobs"
)

func TestMemberPlatformSamplesUseAuthenticatedClusterAndSessionIdentity(t *testing.T) {
	repos := newTestRepos(t)
	cluster := &dbrepo.Cluster{Name: "member-a", Phase: "Online"}
	mustCreateCluster(t, repos, cluster)
	now := time.Now().UTC()
	sample := platformobs.UnavailableSample(platformobs.ComponentOperator, "spoofed", now)
	sample.ClusterID = 999
	sample.ClusterName = "spoofed"
	sample.Role = platformobs.RoleLeader
	sample.ProcessStartedAt = now.Add(-time.Hour)

	server := NewServer(repos, gatewayNS, NewHeartbeatTracker(), NewNotifier(), "token")
	batch := &gatewaypb.ComponentSampleBatch{Samples: []*gatewaypb.ComponentSample{platformobs.ToProto(sample)}}

	// The gateway is the backstop for members that have not yet received the
	// updated switch, so samples arriving while it is off must not be stored.
	if err := server.handleMemberComponentSamples(
		context.Background(), cluster.Name, "operator-1", "standby", batch); err != nil {
		t.Fatal(err)
	}
	rows, err := repos.Platform.LatestSamples(context.Background(), now.Add(-time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Fatalf("stored %d rows while platform observation was disabled, want 0", len(rows))
	}

	server.SetPlatformObservationEnabled(true)
	if err := server.handleMemberComponentSamples(
		context.Background(), cluster.Name, "operator-1", "standby", batch); err != nil {
		t.Fatal(err)
	}
	rows, err = repos.Platform.LatestSamples(context.Background(), now.Add(-time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	// The member's own cluster/instance/role fields are overwritten from the
	// authenticated session, so a member cannot report on another cluster's behalf.
	if len(rows) != 1 || rows[0].ClusterID != cluster.ID || rows[0].ClusterName != cluster.Name ||
		rows[0].Instance != "operator-1" || rows[0].Role != "standby" {
		t.Fatalf("stored rows = %+v", rows)
	}
}
