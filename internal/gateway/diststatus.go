package gateway

import (
	"context"
	"log/slog"

	"github.com/Wynn-hub/kubesentry/internal/api/v1alpha1"
	"github.com/Wynn-hub/kubesentry/internal/dbrepo"
	"github.com/Wynn-hub/kubesentry/internal/gatewaypb"
)

// markPushed reconciles every distribution's per-cluster status row with the
// freshly rendered snapshot: render errors → Error, awaiting the member ack →
// Pending (Synced- or Error-at-current-hash rows stay put — an unresolved
// apply conflict must not be reported back to Pending on every resync tick
// just because Render() itself has no way to know a given hash already
// failed on the member), no longer targeted → row removed. Writes only when
// the row actually changes (setClusterStatus's before/after diff).
func (s *Server) markPushed(ctx context.Context, cluster string, res RenderResult) {
	cl, err := s.repos.Clusters.GetByName(ctx, cluster)
	if err != nil {
		slog.Error("get cluster for status update", "cluster", cluster, "error", err)
		return
	}
	matched := map[string]bool{}
	for _, n := range res.Matched {
		matched[n] = true
	}
	dists, err := s.repos.Distributions.List(ctx)
	if err != nil {
		slog.Error("list distributions for status", "error", err)
		return
	}
	for i := range dists {
		d := &dists[i]
		switch {
		case matched[d.Name] && res.Errors[d.Name] != "":
			s.setClusterStatus(ctx, d.ID, cl.ID, cluster, func(e *dbrepo.DistributionClusterStatus) {
				e.Status = v1alpha1.DistributionError
				e.LastError = res.Errors[d.Name]
			})
		case matched[d.Name]:
			s.setClusterStatus(ctx, d.ID, cl.ID, cluster, func(e *dbrepo.DistributionClusterStatus) {
				if e.AppliedVersionHash == res.Hash &&
					(e.Status == v1alpha1.DistributionSynced || e.Status == v1alpha1.DistributionError) {
					return // already resolved (success or failure) for this exact snapshot
				}
				e.Status = v1alpha1.DistributionPending
				e.LastError = ""
			})
		default:
			if err := s.repos.Distributions.DeleteClusterStatus(ctx, d.ID, cl.ID); err != nil {
				slog.Error("remove distribution status entry", "distribution", d.Name, "cluster", cluster, "error", err)
			}
		}
	}
}

// markReported applies a member StatusReport to every matched distribution.
// Stale reports (hash ≠ last pushed) are ignored.
func (s *Server) markReported(ctx context.Context, cluster string, last RenderResult, rep *gatewaypb.StatusReport) {
	if rep.AppliedVersion == "" || rep.AppliedVersion != last.Hash {
		return
	}
	cl, err := s.repos.Clusters.GetByName(ctx, cluster)
	if err != nil {
		slog.Error("get cluster for status report", "cluster", cluster, "error", err)
		return
	}
	for _, name := range last.Matched {
		if last.Errors[name] != "" {
			continue // render error owns this entry
		}
		d, err := s.repos.Distributions.GetByName(ctx, name)
		if err != nil {
			slog.Error("get distribution for status report", "distribution", name, "error", err)
			continue
		}
		if rep.ApplyError != "" {
			s.setClusterStatus(ctx, d.ID, cl.ID, cluster, func(e *dbrepo.DistributionClusterStatus) {
				e.Status = v1alpha1.DistributionError
				e.AppliedVersionHash = rep.AppliedVersion
				e.LastError = rep.ApplyError
			})
			continue
		}
		s.setClusterStatus(ctx, d.ID, cl.ID, cluster, func(e *dbrepo.DistributionClusterStatus) {
			e.Status = v1alpha1.DistributionSynced
			e.AppliedVersionHash = rep.AppliedVersion
			e.LastError = ""
		})
	}
}

// setClusterStatus mutates one (distribution, cluster) status row, skipping
// the write when the mutator leaves the row unchanged (matches the previous
// CRD-status equalEntry no-op guard; there's no separate lastSyncTime column
// in this schema, so "no write" is the observable equivalent of "no churn").
//
// ponytail: no retry/lock around the read-then-write — markPushed (pushLoop)
// and markReported (Sync's recv loop) can race on the same row from two
// goroutines, same as the previous k8s-object version's reliance on
// RetryOnConflict for serialization. Every mutator here is idempotent and
// convergent (a later render or report always supersedes a stale one), so a
// lost update just gets corrected on the next push/report cycle. Add a
// per-row mutex or SELECT ... FOR UPDATE if strict linearizability is ever
// required.
func (s *Server) setClusterStatus(ctx context.Context, distID, clusterID int64, clusterName string, mutate func(*dbrepo.DistributionClusterStatus)) {
	cur, err := s.repos.Distributions.GetClusterStatus(ctx, distID, clusterID)
	if err != nil {
		slog.Error("get distribution cluster status", "cluster", clusterName, "error", err)
		return
	}
	entry := dbrepo.DistributionClusterStatus{DistributionID: distID, ClusterID: clusterID}
	if cur != nil {
		entry = *cur
	}
	before := entry
	mutate(&entry)
	if before.Status == entry.Status && before.AppliedVersionHash == entry.AppliedVersionHash && before.LastError == entry.LastError {
		return // no-op
	}
	if err := s.repos.Distributions.UpsertClusterStatus(ctx, &entry); err != nil {
		slog.Error("update distribution cluster status", "cluster", clusterName, "error", err)
	}
}
