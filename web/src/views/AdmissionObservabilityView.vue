<template>
  <section v-loading="loading">
    <PageHeader :title="$t('observability.title')" :subtitle="$t('observability.subtitle')">
      <el-button @click="settingsOpen = true"><el-icon><Setting /></el-icon>{{ $t('observability.settings') }}</el-button>
      <el-button type="primary" plain @click="refreshAll"><el-icon><Refresh /></el-icon>{{ $t('common.refresh') }}</el-button>
    </PageHeader>

    <div class="observation-toolbar">
      <el-select v-model="cluster" clearable filterable :placeholder="$t('observability.allClusters')" style="width:220px">
        <el-option v-for="name in clusterOptions" :key="name" :label="name" :value="name" />
      </el-select>
      <el-select v-model="rangeKey" style="width:175px">
        <el-option :label="$t('admissionEvent.last1h')" value="1h" />
        <el-option :label="$t('admissionEvent.last24h')" value="24h" />
        <el-option :label="$t('admissionEvent.last7d')" value="7d" />
        <el-option :label="$t('admissionEvent.last30d')" value="30d" />
        <el-option :label="$t('admissionEvent.customRange')" value="custom" />
      </el-select>
      <el-date-picker
        v-if="rangeKey === 'custom'"
        v-model="customRange"
        type="datetimerange"
        :teleported="false"
        style="width:360px"
      />
      <span class="poll-note"><span class="live-dot" />{{ $t('observability.liveRefresh') }}</span>
    </div>

    <template v-if="overview">
      <el-alert
        v-if="overview.platformHealth.health === 'warning'"
        type="error"
        :closable="false"
        show-icon
        class="platform-alert"
        :title="$t('observability.platformWarning')"
        :description="platformWarning"
      />
      <div v-if="notices.length" class="notice-stack">
        <el-alert v-for="notice in notices" :key="notice" type="warning" :closable="false" show-icon :title="notice" />
      </div>

      <div class="summary-grid">
        <article class="summary-card" :class="`health-${overview.health}`">
          <span class="summary-kicker">{{ $t('observability.linkHealth') }}</span>
          <strong>{{ healthLabel(overview.health) }}</strong>
          <span>{{ healthSummary }}</span>
        </article>
        <article class="summary-card">
          <span class="summary-kicker">{{ $t('observability.webhookGenerated') }}</span>
          <strong class="mono-num">{{ number(overview.generated) }}</strong>
          <span>{{ $t('observability.selectedWindow') }}</span>
        </article>
        <article class="summary-card">
          <span class="summary-kicker">{{ $t('observability.gatewayWritten') }}</span>
          <strong class="mono-num">{{ number(overview.written) }}</strong>
          <span>{{ $t('observability.selectedWindow') }}</span>
        </article>
        <article class="summary-card fault-card">
          <span class="summary-kicker">{{ $t('observability.faultTotal') }}</span>
          <strong class="mono-num">{{ number(overview.windowFaultTotal) }}</strong>
          <span>{{ $t('observability.historicalFaultHint') }}</span>
        </article>
      </div>

      <div class="panel link-panel">
        <div class="panel-heading">
          <div><h2>{{ $t('observability.linkFlow') }}</h2><p>{{ $t('observability.stageVolumeHint') }}</p></div>
          <div class="link-side-metrics">
            <span>{{ $t('observability.retryCount') }} <b>{{ number(totalRecord(overview.retryCount)) }}</b></span>
            <span>{{ $t('observability.dropped') }} <b class="danger">{{ number(totalFault('dropped')) }}</b></span>
          </div>
        </div>
        <div class="funnel">
          <div class="stage"><span>Webhook</span><b>{{ number(overview.generated) }}</b><small>{{ $t('observability.generated') }}</small></div>
          <div class="flow-arrow"><span>→</span></div>
          <div class="stage"><span>Operator</span><b>{{ number(overview.operatorReceived) }}</b><small>{{ $t('observability.received') }}</small></div>
          <div class="flow-arrow"><span>→</span></div>
          <div class="stage"><span>Gateway</span><b>{{ number(overview.gatewayReceived) }}</b><small>{{ $t('observability.received') }}</small></div>
          <div class="flow-arrow"><span>→</span></div>
          <div class="stage database"><span>PostgreSQL</span><b>{{ number(overview.written) }}</b><small>{{ $t('observability.written') }}</small></div>
        </div>
        <div class="branches">
          <span>{{ $t('observability.memberFiltered') }} <b>{{ number(overview.policyFilteredMember) }}</b></span>
          <span>{{ $t('observability.gatewayFiltered') }} <b>{{ number(overview.policyFilteredGateway) }}</b></span>
          <span>{{ $t('observability.retentionElapsed') }} <b>{{ number(overview.retentionElapsed) }}</b></span>
          <span>{{ $t('observability.duplicate') }} <b>{{ number(overview.duplicate) }}</b></span>
        </div>
      </div>

      <div class="analytics-grid">
        <div class="panel trend-panel">
          <div class="panel-heading"><div><h2>{{ $t('observability.trend') }}</h2><p>{{ $t('observability.trendHint') }}</p></div></div>
          <ObservationTrendChart :points="trend?.items ?? []" />
        </div>
        <div class="panel reasons-panel">
          <div class="panel-heading"><div><h2>{{ $t('observability.faultReasons') }}</h2><p>{{ $t('observability.faultReasonsHint') }}</p></div></div>
          <ol v-if="overview.faultReasons.length" class="reason-list">
            <li v-for="reason in overview.faultReasons.slice(0, 8)" :key="`${reason.component}-${reason.reason}`">
              <div><span class="component-pill">{{ reason.component }}</span><code>{{ reason.reason }}</code></div>
              <strong class="mono-num">{{ number(reason.count) }}</strong>
            </li>
          </ol>
          <el-empty v-else :description="$t('observability.noFaults')" :image-size="72" />
        </div>
      </div>

      <div class="panel cluster-panel">
        <div class="panel-heading">
          <div><h2>{{ $t('observability.clusterDrilldown') }}</h2><p>{{ $t('observability.clusterDrilldownHint') }}</p></div>
          <el-select v-model="healthFilter" clearable :placeholder="$t('observability.allHealth')" style="width:190px">
            <el-option v-for="health in healthOptions" :key="health" :label="healthLabel(health)" :value="health" />
          </el-select>
        </div>
        <el-table :data="clusterPage?.items ?? []" row-key="name">
          <el-table-column prop="name" :label="$t('cluster.name')" min-width="150">
            <template #default="{ row }"><el-button link type="primary" @click="cluster = row.name">{{ row.name }}</el-button></template>
          </el-table-column>
          <el-table-column :label="$t('observability.health')" width="150">
            <template #default="{ row }"><el-tag :type="healthTag(row.health)" effect="dark">{{ healthLabel(row.health) }}</el-tag></template>
          </el-table-column>
          <el-table-column :label="$t('observability.webhookFreshness')" min-width="175">
            <template #default="{ row }">{{ freshnessLabel(row.freshness.webhook) }}</template>
          </el-table-column>
          <el-table-column :label="$t('observability.operatorFreshness')" min-width="175">
            <template #default="{ row }">{{ freshnessLabel(row.freshness.operator) }}</template>
          </el-table-column>
          <el-table-column :label="$t('observability.queuePeak')" min-width="150">
            <template #default="{ row }">{{ queueLabel(row.queues.operator ?? row.queues.webhook) }}</template>
          </el-table-column>
          <el-table-column :label="$t('observability.windowFaults')" width="130">
            <template #default="{ row }"><span :class="{ danger: faultTotal(row.windowFaults) > 0 }">{{ number(faultTotal(row.windowFaults)) }}</span></template>
          </el-table-column>
          <el-table-column :label="$t('observability.mainReason')" min-width="210">
            <template #default="{ row }"><code v-if="row.majorFaultReasons.length">{{ row.majorFaultReasons[0].reason }}</code><span v-else>—</span></template>
          </el-table-column>
        </el-table>
        <PaginationBar
          :total="clusterPage?.total ?? 0"
          :page="page"
          :page-size="pageSize"
          @update:page="value => { page = value; loadClusters() }"
          @update:page-size="value => { pageSize = value; page = 1; loadClusters() }"
        />
      </div>
    </template>

    <ObservationSettingsDrawer v-model="settingsOpen" />
  </section>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { Refresh, Setting } from '@element-plus/icons-vue'
import { useI18n } from 'vue-i18n'
import PageHeader from '../components/PageHeader.vue'
import PaginationBar from '../components/PaginationBar.vue'
import ObservationTrendChart from '../components/ObservationTrendChart.vue'
import ObservationSettingsDrawer from '../components/ObservationSettingsDrawer.vue'
import { createObservationPoller } from './observationPolling'
import { collectAllPages } from '../api/pagination'
import { listClusters } from '../api/cluster'
import {
  getAdmissionObservationOverview,
  getAdmissionObservationTrend,
  listAdmissionObservationClusters,
  type AdmissionObservationOverview,
  type ObservationClusterPage,
  type ObservationFaults,
  type ObservationFreshness,
  type ObservationHealth,
  type ObservationQueue,
  type ObservationTrend,
} from '../api/observability'

const { t } = useI18n()
const overview = ref<AdmissionObservationOverview>()
const trend = ref<ObservationTrend>()
const clusterPage = ref<ObservationClusterPage>()
const clusterOptions = ref<string[]>([])
const cluster = ref('')
const rangeKey = ref('24h')
const customRange = ref<[Date, Date] | null>(null)
const healthFilter = ref('')
const page = ref(1)
const pageSize = ref(20)
const loading = ref(true)
const settingsOpen = ref(false)
const healthOptions: ObservationHealth[] = ['warning', 'partial_no_data', 'no_data', 'not_onboarded', 'normal', 'offline']

function rangeQuery() {
  if (rangeKey.value === 'custom') {
    if (!customRange.value) return {}
    return { from: customRange.value[0].toISOString(), to: customRange.value[1].toISOString() }
  }
  const spans: Record<string, number> = { '1h': 36e5, '24h': 864e5, '7d': 6048e5, '30d': 2592e6 }
  const span = spans[rangeKey.value]
  const now = Date.now()
  return { from: new Date(now - span).toISOString(), to: new Date(now).toISOString() }
}
const scopedQuery = () => ({ ...(cluster.value ? { cluster: cluster.value } : {}), ...rangeQuery() })

async function loadWindow() {
  overview.value = await getAdmissionObservationOverview(scopedQuery())
}
async function loadCurrent() {
  const current = await getAdmissionObservationOverview({ ...(cluster.value ? { cluster: cluster.value } : {}), currentOnly: 'true' })
  if (!overview.value) { overview.value = current; return }
  overview.value = {
    ...overview.value,
    health: current.health,
    platformHealth: current.platformHealth,
    freshness: current.freshness,
    clusterCounts: current.clusterCounts,
    observabilityOnboarded: current.observabilityOnboarded,
  }
}
async function loadTrend() { trend.value = await getAdmissionObservationTrend(scopedQuery()) }
async function loadClusters() {
  clusterPage.value = await listAdmissionObservationClusters({
    ...rangeQuery(),
    page: String(page.value),
    pageSize: String(pageSize.value),
    ...(healthFilter.value ? { health: healthFilter.value } : {}),
  })
}
async function safe(run: () => Promise<void>) {
  try { await run() } catch (error) { ElMessage.error((error as Error).message) }
}
async function refreshAll() {
  await Promise.all([safe(loadWindow), safe(loadTrend), safe(loadClusters)])
}
const poller = createObservationPoller([
  { interval: 10_000, run: () => { safe(loadCurrent) } },
  { interval: 10_000, run: () => { safe(loadClusters) } },
  { interval: 30_000, run: () => { safe(loadWindow) } },
  { interval: 60_000, run: () => { safe(loadTrend) } },
])
function startPolling() { poller.start(document.hidden) }
function stopPolling() { poller.stop() }
function visibilityChanged() {
  if (document.hidden) stopPolling()
  else { refreshAll(); startPolling() }
}

watch([cluster, rangeKey, customRange], async () => {
  if (rangeKey.value === 'custom' && !customRange.value) return
  page.value = 1
  await refreshAll()
}, { deep: true })
watch(healthFilter, () => { page.value = 1; safe(loadClusters) })

const number = (value: number) => value.toLocaleString()
const totalRecord = (values: Record<string, number>) => Object.values(values).reduce((sum, value) => sum + value, 0)
const totalFault = (key: keyof ObservationFaults) =>
  Object.values(overview.value?.windowFaults ?? {}).reduce((sum, faults) => sum + faults[key], 0)
const faultTotal = (faults: ObservationFaults) => faults.dropped + faults.batchFailure + faults.authFailure + faults.ackTimeout + faults.reportDropped
const healthLabel = (health: ObservationHealth) => t(`observability.health_${health}`)
const healthTag = (health: ObservationHealth) => health === 'normal' ? 'success' : health === 'offline' || health === 'no_data' ? 'info' : health === 'warning' ? 'danger' : 'warning'
const freshnessLabel = (freshness?: ObservationFreshness) => freshness?.lastObservedAt
  ? `${freshness.fresh ? t('observability.fresh') : t('observability.stale')} · ${new Date(freshness.lastObservedAt).toLocaleString()}`
  : t('observability.neverReported')
const queueLabel = (queue?: ObservationQueue) => queue?.capacity ? `${number(queue.depthMax)} / ${number(queue.capacity)}` : '—'
const healthSummary = computed(() => {
  const counts = overview.value?.clusterCounts
  if (!counts) return ''
  return t('observability.healthCounts', { normal: counts.normal, warning: counts.warning, unavailable: counts.partialNoData + counts.noData })
})
const platformWarning = computed(() => {
  const health = overview.value?.platformHealth
  if (!health) return ''
  return [
    health.cleanupFailure ? t('observability.cleanupFailures', { count: health.cleanupFailure }) : '',
    health.observationWriteFailed ? t('observability.writeFailures', { count: health.observationWriteFailed }) : '',
    health.authFailure ? t('observability.authFailures', { count: health.authFailure }) : '',
  ].filter(Boolean).join(' · ')
})
const notices = computed(() => {
  if (!overview.value) return []
  const result: string[] = []
  if (overview.value.health === 'partial_no_data') {
    const missing = ['webhook', 'operator'].filter(component => !overview.value?.freshness[component]?.fresh)
    result.push(t('observability.componentInterrupted', { component: missing.join(', ') }))
  }
  if (overview.value.dataIncomplete) result.push(t('observability.dataIncomplete'))
  if (overview.value.baselineReset) result.push(t('observability.baselineReset'))
  if ((overview.value.seriesStarted.operator ?? 0) > 0) result.push(t('observability.operatorRestarted'))
  if (overview.value.clusterCounts.notOnboarded > 0) result.push(t('observability.notOnboardedCount', { count: overview.value.clusterCounts.notOnboarded }))
  return result
})

onMounted(async () => {
  try { clusterOptions.value = (await collectAllPages(listClusters)).map(item => item.name) } catch { /* overview remains usable */ }
  await refreshAll()
  loading.value = false
  document.addEventListener('visibilitychange', visibilityChanged)
  startPolling()
})
onBeforeUnmount(() => {
  stopPolling()
  document.removeEventListener('visibilitychange', visibilityChanged)
})
</script>

<style scoped>
.observation-toolbar{display:flex;align-items:center;flex-wrap:wrap;gap:9px;margin-bottom:14px}
.poll-note{margin-left:auto;display:flex;align-items:center;gap:7px;color:var(--steel-400);font:11px var(--font-mono)}
.live-dot{width:7px;height:7px;border-radius:50%;background:var(--accent-scan);box-shadow:0 0 0 4px rgba(51,214,192,.09)}
.platform-alert,.notice-stack{margin-bottom:12px}.notice-stack{display:grid;gap:8px}
.summary-grid{display:grid;grid-template-columns:repeat(4,minmax(0,1fr));gap:12px;margin:14px 0}
.summary-card,.panel{border:1px solid var(--ink-600);background:linear-gradient(145deg,rgba(30,42,52,.94),rgba(18,27,35,.96));border-radius:10px}
.summary-card{position:relative;overflow:hidden;display:flex;flex-direction:column;gap:6px;padding:16px 18px}
.summary-card::after{content:"";position:absolute;inset:auto 0 0;height:2px;background:var(--accent-scan);opacity:.55}
.summary-card strong{font:700 27px var(--font-mono);color:var(--paper-050)}.summary-card>span:last-child{color:var(--steel-400);font-size:11px}
.summary-kicker{text-transform:uppercase;letter-spacing:.07em;color:var(--steel-300);font:10px var(--font-mono)}
.health-warning::after,.fault-card::after{background:#ff6b7a}.health-partial_no_data::after,.health-not_onboarded::after{background:#eab764}.health-no_data::after,.health-offline::after{background:var(--steel-400)}
.panel{padding:18px;margin-bottom:14px}.panel-heading{display:flex;align-items:flex-start;justify-content:space-between;gap:16px;margin-bottom:16px}
.panel-heading h2{margin:0 0 5px;font-size:15px;color:var(--paper-050)}.panel-heading p{margin:0;color:var(--steel-400);font-size:11px;line-height:1.5}
.link-side-metrics{display:flex;gap:16px;color:var(--steel-300);font:11px var(--font-mono)}.link-side-metrics b{color:var(--paper-050)}
.funnel{display:grid;grid-template-columns:minmax(130px,1fr) 48px minmax(130px,1fr) 48px minmax(130px,1fr) 48px minmax(130px,1fr);align-items:center}
.stage{display:flex;flex-direction:column;align-items:center;gap:4px;padding:15px 10px;border:1px solid var(--ink-600);background:var(--ink-800);border-radius:8px}.stage>span{font:11px var(--font-mono);color:var(--accent-scan)}.stage b{font:700 23px var(--font-mono);color:var(--paper-050)}.stage small{color:var(--steel-400)}
.stage.database{border-color:rgba(122,162,255,.45)}.stage.database>span{color:#7aa2ff}.flow-arrow{text-align:center;color:var(--steel-400);font:22px var(--font-mono)}
.branches{display:flex;justify-content:center;flex-wrap:wrap;gap:10px 24px;margin-top:13px;color:var(--steel-300);font:11px var(--font-mono)}.branches b{color:#eab764}
.analytics-grid{display:grid;grid-template-columns:minmax(0,2fr) minmax(260px,1fr);gap:14px}.trend-panel,.reasons-panel{min-width:0}
.reason-list{list-style:none;padding:0;margin:0;display:grid;gap:2px}.reason-list li{display:flex;align-items:center;justify-content:space-between;gap:12px;padding:9px 4px;border-bottom:1px solid var(--ink-700)}.reason-list li>div{display:flex;align-items:center;gap:8px;min-width:0}.reason-list code{overflow:hidden;text-overflow:ellipsis;color:var(--steel-200)}
.component-pill{padding:2px 6px;border:1px solid var(--ink-600);border-radius:4px;color:var(--accent-scan);font:9px var(--font-mono);text-transform:uppercase}
.danger{color:#ff6b7a!important}code{font-family:var(--font-mono);font-size:11px}
@media(max-width:1100px){.summary-grid{grid-template-columns:repeat(2,1fr)}.analytics-grid{grid-template-columns:1fr}.funnel{grid-template-columns:1fr}.flow-arrow{transform:rotate(90deg);height:34px}.poll-note{margin-left:0;width:100%}}
@media(max-width:650px){.summary-grid{grid-template-columns:1fr}.panel-heading{flex-direction:column}.link-side-metrics{flex-wrap:wrap}}
</style>
