package gateway

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/Wynn-hub/kubesentry/internal/api/v1alpha1"
	"github.com/Wynn-hub/kubesentry/internal/gatewaypb"
)

// markPushed reconciles every distribution's per-cluster status entry with
// the freshly rendered snapshot: render errors → Error, awaiting the member
// ack → Pending (Synced- or Error-at-current-hash entries stay put — an
// unresolved apply conflict must not be reported back to Pending on every
// resync tick just because Render() itself has no way to know a given hash
// already failed on the member), no longer targeted → entry removed. Writes
// only when the entry actually changes.
func (s *Server) markPushed(ctx context.Context, cluster string, res RenderResult) {
	matched := map[string]bool{}
	for _, n := range res.Matched {
		matched[n] = true
	}
	var dists v1alpha1.PolicyDistributionList
	if err := s.client.List(ctx, &dists); err != nil {
		log.FromContext(ctx).Error(err, "list distributions for status")
		return
	}
	for i := range dists.Items {
		name := dists.Items[i].Name
		switch {
		case matched[name] && res.Errors[name] != "":
			s.setClusterEntry(ctx, name, cluster, func(e *v1alpha1.DistributionClusterStatus) {
				e.State = v1alpha1.DistributionError
				e.Message = res.Errors[name]
			})
		case matched[name]:
			s.setClusterEntry(ctx, name, cluster, func(e *v1alpha1.DistributionClusterStatus) {
				if e.AppliedVersion == res.Hash &&
					(e.State == v1alpha1.DistributionSynced || e.State == v1alpha1.DistributionError) {
					return // already resolved (success or failure) for this exact snapshot
				}
				e.State = v1alpha1.DistributionPending
				e.Message = ""
			})
		default:
			s.removeClusterEntry(ctx, name, cluster)
		}
	}
}

// markReported applies a member StatusReport to every matched distribution.
// Stale reports (hash ≠ last pushed) are ignored.
func (s *Server) markReported(ctx context.Context, cluster string, last RenderResult, rep *gatewaypb.StatusReport) {
	if rep.AppliedVersion == "" || rep.AppliedVersion != last.Hash {
		return
	}
	now := metav1.Now()
	for _, name := range last.Matched {
		if last.Errors[name] != "" {
			continue // render error owns this entry
		}
		if rep.ApplyError != "" {
			s.setClusterEntry(ctx, name, cluster, func(e *v1alpha1.DistributionClusterStatus) {
				e.State = v1alpha1.DistributionError
				e.AppliedVersion = rep.AppliedVersion
				e.Message = rep.ApplyError
			})
			continue
		}
		s.setClusterEntry(ctx, name, cluster, func(e *v1alpha1.DistributionClusterStatus) {
			if e.State == v1alpha1.DistributionSynced && e.AppliedVersion == rep.AppliedVersion {
				return // avoid lastSyncTime churn on periodic reports
			}
			e.State = v1alpha1.DistributionSynced
			e.AppliedVersion = rep.AppliedVersion
			e.Message = ""
			e.LastSyncTime = &now
		})
	}
}

// setClusterEntry mutates one cluster's entry under RetryOnConflict,
// skipping the write when the mutator leaves the entry unchanged.
func (s *Server) setClusterEntry(ctx context.Context, dist, cluster string, mutate func(*v1alpha1.DistributionClusterStatus)) {
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		var d v1alpha1.PolicyDistribution
		if err := s.client.Get(ctx, types.NamespacedName{Name: dist}, &d); err != nil {
			return client.IgnoreNotFound(err)
		}
		idx := -1
		for i := range d.Status.Clusters {
			if d.Status.Clusters[i].Name == cluster {
				idx = i
				break
			}
		}
		if idx == -1 {
			d.Status.Clusters = append(d.Status.Clusters, v1alpha1.DistributionClusterStatus{Name: cluster})
			idx = len(d.Status.Clusters) - 1
		}
		before := d.Status.Clusters[idx] // plain value copy: no slice fields, LastSyncTime ptr not compared
		mutate(&d.Status.Clusters[idx])
		if equalEntry(before, d.Status.Clusters[idx]) {
			return nil // no-op
		}
		return s.client.Status().Update(ctx, &d)
	})
	if err != nil {
		log.FromContext(ctx).Error(err, "update distribution status", "distribution", dist, "cluster", cluster)
	}
}

func (s *Server) removeClusterEntry(ctx context.Context, dist, cluster string) {
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		var d v1alpha1.PolicyDistribution
		if err := s.client.Get(ctx, types.NamespacedName{Name: dist}, &d); err != nil {
			return client.IgnoreNotFound(err)
		}
		kept := d.Status.Clusters[:0]
		removed := false
		for _, e := range d.Status.Clusters {
			if e.Name == cluster {
				removed = true
				continue
			}
			kept = append(kept, e)
		}
		if !removed {
			return nil
		}
		d.Status.Clusters = kept
		return s.client.Status().Update(ctx, &d)
	})
	if err != nil {
		log.FromContext(ctx).Error(err, "remove distribution status entry", "distribution", dist, "cluster", cluster)
	}
}

func equalEntry(a, b v1alpha1.DistributionClusterStatus) bool {
	return a.State == b.State && a.AppliedVersion == b.AppliedVersion && a.Message == b.Message
}
