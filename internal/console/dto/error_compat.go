package dto

// ErrorResponse is shared by all generated operations. ogen normally emits a
// distinct error type per response branch; these aliases keep the uniform
// envelope usable by the in-process compatibility adapters during migration.
func (*ErrorResponse) createClusterRes()                {}
func (*ErrorResponse) getClusterRes()                   {}
func (*ErrorResponse) updateClusterRes()                {}
func (*ErrorResponse) deleteClusterRes()                {}
func (*ErrorResponse) reissueClusterTokenRes()          {}
func (*ErrorResponse) getLatestInstallParamsRes()       {}
func (*ErrorResponse) createUninstallTaskRes()          {}
func (*ErrorResponse) listUninstallTasksRes()           {}
func (*ErrorResponse) retryUninstallTaskRes()           {}
func (*ErrorResponse) createDistributionRes()           {}
func (*ErrorResponse) getDistributionRes()              {}
func (*ErrorResponse) updateDistributionRes()           {}
func (*ErrorResponse) deleteDistributionRes()           {}
func (*ErrorResponse) getPlatformInstanceTrendRes()     {}
func (*ErrorResponse) getPlatformOverviewRes()          {}
func (*ErrorResponse) listPlatformLeadershipEventsRes() {}
func (*ErrorResponse) updatePlatformSettingsRes()       {}
