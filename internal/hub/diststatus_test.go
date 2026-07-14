package hub

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/Wynn-hub/kubesentry/internal/api/v1alpha1"
	"github.com/Wynn-hub/kubesentry/internal/hubpb"
)

func distFixture(name string) *v1alpha1.PolicyDistribution {
	return &v1alpha1.PolicyDistribution{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: v1alpha1.PolicyDistributionSpec{
			Clusters:     v1alpha1.DistributionClusters{ByName: []string{"c1"}},
			PolicyGroups: []string{"g1"},
		},
	}
}

func getDist(t *testing.T, s *Server, name string) v1alpha1.PolicyDistribution {
	t.Helper()
	var d v1alpha1.PolicyDistribution
	if err := s.client.Get(context.Background(), types.NamespacedName{Name: name}, &d); err != nil {
		t.Fatal(err)
	}
	return d
}

func TestMarkPushedAndReportedLifecycle(t *testing.T) {
	c := newHubClient(t, distFixture("d1"))
	s := NewServer(c, hubNS, NewHeartbeatTracker(), NewNotifier())
	ctx := context.Background()

	res := RenderResult{Hash: "h1", Matched: []string{"d1"}, Errors: map[string]string{}}
	s.markPushed(ctx, "c1", res)
	d := getDist(t, s, "d1")
	if len(d.Status.Clusters) != 1 || d.Status.Clusters[0].State != v1alpha1.DistributionPending {
		t.Fatalf("after push: %+v", d.Status.Clusters)
	}

	s.markReported(ctx, "c1", res, &hubpb.StatusReport{AppliedVersion: "h1"})
	d = getDist(t, s, "d1")
	e := d.Status.Clusters[0]
	if e.State != v1alpha1.DistributionSynced || e.AppliedVersion != "h1" || e.LastSyncTime == nil {
		t.Fatalf("after report: %+v", e)
	}

	// 重复上报同 hash 不刷新 lastSyncTime。
	prev := *e.LastSyncTime
	s.markReported(ctx, "c1", res, &hubpb.StatusReport{AppliedVersion: "h1"})
	if got := *getDist(t, s, "d1").Status.Clusters[0].LastSyncTime; !got.Equal(&prev) {
		t.Fatal("lastSyncTime churned on periodic report")
	}

	// 相同 hash 且已 Synced → markPushed 不打回 Pending。
	s.markPushed(ctx, "c1", res)
	if getDist(t, s, "d1").Status.Clusters[0].State != v1alpha1.DistributionSynced {
		t.Fatal("markPushed demoted a synced entry")
	}

	// apply 失败 → Error。
	s.markReported(ctx, "c1", res, &hubpb.StatusReport{AppliedVersion: "h1", ApplyError: "boom"})
	if e := getDist(t, s, "d1").Status.Clusters[0]; e.State != v1alpha1.DistributionError || e.Message != "boom" {
		t.Fatalf("apply error not recorded: %+v", e)
	}

	// 同一 hash 的下一次 push 周期（resync ticker 或无关变更触发）不得把
	// Error 打回 Pending，否则一个持续冲突的 override 会被误报为"处理中"。
	s.markPushed(ctx, "c1", res)
	if e := getDist(t, s, "d1").Status.Clusters[0]; e.State != v1alpha1.DistributionError || e.Message != "boom" {
		t.Fatalf("markPushed demoted an unresolved error entry: %+v", e)
	}

	// 陈旧上报被忽略。
	s.markReported(ctx, "c1", res, &hubpb.StatusReport{AppliedVersion: "old"})
	if getDist(t, s, "d1").Status.Clusters[0].State != v1alpha1.DistributionError {
		t.Fatal("stale report mutated status")
	}

	// 渲染错误 → Error(message)。
	resErr := RenderResult{Hash: "h2", Matched: []string{"d1"}, Errors: map[string]string{"d1": "policygroup \"ghost\" not found"}}
	s.markPushed(ctx, "c1", resErr)
	if e := getDist(t, s, "d1").Status.Clusters[0]; e.State != v1alpha1.DistributionError || e.Message == "" {
		t.Fatalf("render error not recorded: %+v", e)
	}

	// 集群移出目标 → 条目删除。
	s.markPushed(ctx, "c1", RenderResult{Hash: "h3", Errors: map[string]string{}})
	if got := getDist(t, s, "d1").Status.Clusters; len(got) != 0 {
		t.Fatalf("entry not removed: %+v", got)
	}
}
