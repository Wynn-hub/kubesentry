package gateway

import (
	"context"
	"testing"

	"github.com/Wynn-hub/kubesentry/internal/api/v1alpha1"
	"github.com/Wynn-hub/kubesentry/internal/dbrepo"
	"github.com/Wynn-hub/kubesentry/internal/gatewaypb"
)

// getClusterStatus resolves distribution+cluster names to IDs (mirroring
// what markPushed/markReported do internally) and returns the current
// per-cluster status row, or nil if none exists yet.
func getClusterStatus(t *testing.T, repos Repos, distName, clusterName string) *dbrepo.DistributionClusterStatus {
	t.Helper()
	ctx := context.Background()
	d, err := repos.Distributions.GetByName(ctx, distName)
	if err != nil {
		t.Fatal(err)
	}
	cl, err := repos.Clusters.GetByName(ctx, clusterName)
	if err != nil {
		t.Fatal(err)
	}
	e, err := repos.Distributions.GetClusterStatus(ctx, d.ID, cl.ID)
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func TestMarkPushedAndReportedLifecycle(t *testing.T) {
	repos := newTestRepos(t)
	ctx := context.Background()
	mustCreateCluster(t, repos, &dbrepo.Cluster{Name: "c1"})
	if err := repos.Distributions.Create(ctx, &dbrepo.PolicyDistribution{Name: "d1"}); err != nil {
		t.Fatal(err)
	}
	s := NewServer(repos, gatewayNS, NewHeartbeatTracker(), NewNotifier(), "test-token")

	res := RenderResult{Hash: "h1", Matched: []string{"d1"}, Errors: map[string]string{}}
	s.markPushed(ctx, "c1", res)
	e := getClusterStatus(t, repos, "d1", "c1")
	if e == nil || e.Status != v1alpha1.DistributionPending {
		t.Fatalf("after push: %+v", e)
	}

	s.markReported(ctx, "c1", res, &gatewaypb.StatusReport{AppliedVersion: "h1"})
	e = getClusterStatus(t, repos, "d1", "c1")
	if e.Status != v1alpha1.DistributionSynced || e.AppliedVersionHash != "h1" {
		t.Fatalf("after report: %+v", e)
	}

	// 重复上报同 hash：这个 schema 没有独立的 lastSyncTime 列，等价断言是
	// 重复上报不改变行内容（setClusterStatus 的 before/after diff 会跳过写入）。
	s.markReported(ctx, "c1", res, &gatewaypb.StatusReport{AppliedVersion: "h1"})
	if got := getClusterStatus(t, repos, "d1", "c1"); got.Status != v1alpha1.DistributionSynced || got.AppliedVersionHash != "h1" {
		t.Fatalf("repeated report altered status: %+v", got)
	}

	// 相同 hash 且已 Synced → markPushed 不打回 Pending。
	s.markPushed(ctx, "c1", res)
	if getClusterStatus(t, repos, "d1", "c1").Status != v1alpha1.DistributionSynced {
		t.Fatal("markPushed demoted a synced entry")
	}

	// apply 失败 → Error。
	s.markReported(ctx, "c1", res, &gatewaypb.StatusReport{AppliedVersion: "h1", ApplyError: "boom"})
	if e := getClusterStatus(t, repos, "d1", "c1"); e.Status != v1alpha1.DistributionError || e.LastError != "boom" {
		t.Fatalf("apply error not recorded: %+v", e)
	}

	// 同一 hash 的下一次 push 周期（resync ticker 或无关变更触发）不得把
	// Error 打回 Pending，否则一个持续冲突的 override 会被误报为"处理中"。
	s.markPushed(ctx, "c1", res)
	if e := getClusterStatus(t, repos, "d1", "c1"); e.Status != v1alpha1.DistributionError || e.LastError != "boom" {
		t.Fatalf("markPushed demoted an unresolved error entry: %+v", e)
	}

	// 陈旧上报被忽略。
	s.markReported(ctx, "c1", res, &gatewaypb.StatusReport{AppliedVersion: "old"})
	if getClusterStatus(t, repos, "d1", "c1").Status != v1alpha1.DistributionError {
		t.Fatal("stale report mutated status")
	}

	// 渲染错误 → Error(message)。
	resErr := RenderResult{Hash: "h2", Matched: []string{"d1"}, Errors: map[string]string{"d1": "policygroup \"ghost\" not found"}}
	s.markPushed(ctx, "c1", resErr)
	if e := getClusterStatus(t, repos, "d1", "c1"); e.Status != v1alpha1.DistributionError || e.LastError == "" {
		t.Fatalf("render error not recorded: %+v", e)
	}

	// 集群移出目标 → 条目删除。
	s.markPushed(ctx, "c1", RenderResult{Hash: "h3", Errors: map[string]string{}})
	if got := getClusterStatus(t, repos, "d1", "c1"); got != nil {
		t.Fatalf("entry not removed: %+v", got)
	}
}
