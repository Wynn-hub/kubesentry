import { describe, expect, it } from 'vitest'
import tenantView from '../TenantListView.vue?raw'
import clusterView from '../ClusterListView.vue?raw'
import tenantDialog from '../../components/TenantClusterDialog.vue?raw'
import clusterDialog from '../../components/ClusterTenantDialog.vue?raw'

describe('tenant cluster assignment', () => {
  it('offers assignment from both management surfaces', () => {
    expect(tenantView).toContain('TenantClusterDialog')
    expect(tenantView).toContain('row.clusterNames')
    expect(tenantView).toContain("'tenant.disable'")
    expect(clusterView).toContain('ClusterTenantDialog')
    expect(clusterView).toContain('row.tenantName')
    expect(clusterView).toContain("command=\"tenant\"")
  })

  it('supports bulk synchronization with explicit transfer handling', () => {
    expect(tenantDialog).toContain('syncTenantClusters')
    expect(tenantDialog).toContain('transfers.value.length > 0')
    expect(clusterDialog).toContain('assignClusterTenant')
  })
})
