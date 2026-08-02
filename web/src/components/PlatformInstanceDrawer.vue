<template>
  <el-drawer v-model="open" :title="$t('platformObservability.instanceDetail')" size="min(860px, 94vw)" destroy-on-close>
    <template v-if="target">
      <h3 class="instance-title">{{ target.instance.instance }}</h3>
      <p class="instance-sub">{{ componentLabel }}<template v-if="target.cluster"> · {{ target.cluster }}</template></p>

      <el-alert
        v-if="target.instance.healthReason"
        :type="target.instance.health === 'ok' ? 'info' : 'warning'"
        :closable="false"
        show-icon
        class="reason"
        :title="$t(`platformObservability.reason_${target.instance.healthReason}`)"
        :description="$t(`platformObservability.advice_${target.instance.healthReason}`)"
      />

      <el-descriptions :column="2" border class="summary">
        <el-descriptions-item :label="$t('platformObservability.role')">{{ role }}</el-descriptions-item>
        <el-descriptions-item :label="$t('platformObservability.health')">
          {{ $t(`platformObservability.instance_${target.instance.health}`) }}
        </el-descriptions-item>
        <el-descriptions-item :label="$t('platformObservability.uptime')">
          {{ stale ? '—' : duration(target.instance.uptimeSeconds) }}
        </el-descriptions-item>
        <el-descriptions-item :label="$t('platformObservability.restarts')">
          {{ target.instance.restartCount }}
        </el-descriptions-item>
        <el-descriptions-item :label="$t('platformObservability.lastObserved')">
          {{ new Date(target.instance.lastObservedAt).toLocaleString() }}
        </el-descriptions-item>
        <el-descriptions-item v-if="!isPostgres" :label="$t('platformObservability.memory')">
          {{ stale ? '—' : memory }}
        </el-descriptions-item>
      </el-descriptions>

      <!-- postgres runs an upstream image and console-web is nginx: neither can
           execute our collector, so Go runtime metrics simply do not exist for
           them. Show what each one is actually judged on instead of empty rows. -->
      <template v-if="isPostgres">
        <h4 class="panel-title">{{ $t('platformObservability.databaseStats') }}</h4>
        <el-descriptions :column="2" border class="summary">
          <el-descriptions-item :label="$t('platformObservability.connections')">
            {{ stale ? '—' : ratio('connections', 'maxConnections') }}
          </el-descriptions-item>
          <el-descriptions-item :label="$t('platformObservability.cacheHit')">
            {{ stale ? '—' : percent(stat('cacheHitRatio')) }}
          </el-descriptions-item>
          <el-descriptions-item :label="$t('platformObservability.databaseSize')">
            {{ stale ? '—' : bytes(stat('databaseBytes')) }}
          </el-descriptions-item>
          <el-descriptions-item :label="$t('platformObservability.connectionPool')">
            {{ stale ? '—' : pool }}
          </el-descriptions-item>
          <el-descriptions-item :label="$t('platformObservability.transactions')">
            {{ stale ? '—' : platformMetricText(stat('transactions')) }}
          </el-descriptions-item>
          <el-descriptions-item :label="$t('platformObservability.rollbacks')">
            {{ stale ? '—' : platformMetricText(stat('rollbacks')) }}
          </el-descriptions-item>
          <el-descriptions-item :label="$t('platformObservability.poolWaits')">
            {{ stale ? '—' : platformMetricText(stat('poolWaitCount')) }}
          </el-descriptions-item>
          <el-descriptions-item :label="$t('platformObservability.poolWaitTime')">
            {{ stale ? '—' : seconds(stat('poolWaitSeconds')) }}
          </el-descriptions-item>
        </el-descriptions>
        <p class="hint">{{ $t('platformObservability.postgresProbeHint') }}</p>
      </template>

      <template v-else-if="isConsoleWeb">
        <h4 class="panel-title">{{ $t('platformObservability.probeStatus') }}</h4>
        <el-descriptions :column="1" border class="summary">
          <el-descriptions-item :label="$t('platformObservability.httpProbe')">
            <el-tag :type="probeFailed ? 'danger' : 'success'" size="small" effect="dark">
              {{ probeFailed ? $t('platformObservability.probeFailed') : $t('platformObservability.probeOk') }}
            </el-tag>
          </el-descriptions-item>
          <el-descriptions-item :label="$t('platformObservability.collectedBy')">
            {{ $t('platformObservability.consoleWebProxyHint') }}
          </el-descriptions-item>
        </el-descriptions>
      </template>

      <template v-else>
        <h4 class="panel-title">{{ $t('platformObservability.runtimeMetrics') }}</h4>
        <el-descriptions :column="2" border class="summary">
          <el-descriptions-item :label="$t('platformObservability.goroutines')">
            {{ stale ? '—' : platformMetricText(target.instance.goroutines) }}
          </el-descriptions-item>
          <el-descriptions-item :label="$t('platformObservability.cpu')">
            {{ stale ? '—' : platformMetricText(target.instance.cpuMillis, 'm') }}
          </el-descriptions-item>
          <el-descriptions-item :label="$t('platformObservability.heapAlloc')">
            {{ stale ? '—' : bytes(target.instance.heapAllocBytes) }}
          </el-descriptions-item>
          <el-descriptions-item :label="$t('platformObservability.heapSys')">
            {{ stale ? '—' : bytes(target.instance.heapSysBytes) }}
          </el-descriptions-item>
        </el-descriptions>

        <div class="trend-toolbar">
          <h4 class="panel-title">{{ $t('platformObservability.instanceTrend') }}</h4>
          <div>
            <el-select v-model="range" size="small" style="width:150px">
              <el-option :label="$t('admissionEvent.last1h')" value="1h" />
              <el-option :label="$t('admissionEvent.last24h')" value="24h" />
              <el-option :label="$t('admissionEvent.last7d')" value="7d" />
            </el-select>
            <span class="hint-inline">{{ $t('platformObservability.trendRefresh') }}</span>
          </div>
        </div>
        <div v-loading="loading" class="trend-panel">
          <PlatformTrendChart :points="trend?.items ?? []" />
        </div>
      </template>
    </template>
  </el-drawer>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { useI18n } from 'vue-i18n'
import type { PlatformInstance, PlatformTrend } from '../api/platform'
import { getPlatformInstanceTrend } from '../api/platform'
import PlatformTrendChart from './PlatformTrendChart.vue'
import { platformMetricsStale, platformMetricText } from './platformTrend'

export interface PlatformInstanceTarget {
  component: string
  cluster?: string
  instance: PlatformInstance
}

const props = defineProps<{ modelValue: boolean; target?: PlatformInstanceTarget }>()
const emit = defineEmits<{ 'update:modelValue': [value: boolean] }>()
const { t } = useI18n()
const open = computed({
  get: () => props.modelValue,
  set: value => emit('update:modelValue', value),
})
const trend = ref<PlatformTrend>()
const loading = ref(false)
const range = ref('1h')
let timer: number | undefined

const isPostgres = computed(() => props.target?.component === 'postgres')
const isConsoleWeb = computed(() => props.target?.component === 'console-web')
const componentLabel = computed(() => props.target
  ? t(`platformObservability.component_${props.target.component.replace('-', '_')}`)
  : '')
const stale = computed(() => !!props.target && platformMetricsStale(props.target.instance))
const probeFailed = computed(() => props.target?.instance.healthReason === 'console_web_probe_failed')
const role = computed(() => props.target?.instance.role === 'leader'
  ? t('platformObservability.leader')
  : props.target?.instance.role === 'standby' ? t('platformObservability.standby') : '—')
const memory = computed(() => {
  const instance = props.target?.instance
  if (!instance) return '—'
  return instance.memoryLimitBytes > 0
    ? `${bytes(instance.memoryBytes)} / ${bytes(instance.memoryLimitBytes)}`
    : bytes(instance.memoryBytes)
})
const stat = (key: string) => props.target?.instance.dbStats?.[key] ?? -1
const ratio = (value: string, maximum: string) => stat(maximum) >= 0
  ? `${platformMetricText(stat(value))} / ${platformMetricText(stat(maximum))}`
  : platformMetricText(stat(value))
const pool = computed(() => `${platformMetricText(stat('poolInUse'))} / ${platformMetricText(stat('poolIdle'))}`)

const span = () => ({ '1h': 36e5, '24h': 864e5, '7d': 6048e5 }[range.value] ?? 36e5)
async function load() {
  // postgres and console-web have no runtime series to plot, so skip the query
  // rather than rendering an empty chart.
  if (!props.target || document.hidden || isPostgres.value || isConsoleWeb.value) return
  loading.value = true
  try {
    const to = new Date()
    trend.value = await getPlatformInstanceTrend(props.target.component, props.target.instance.instance, {
      from: new Date(to.getTime() - span()).toISOString(),
      to: to.toISOString(),
      ...(props.target.cluster ? { cluster: props.target.cluster } : {}),
    })
  } catch (error) {
    ElMessage.error((error as Error).message)
  } finally {
    loading.value = false
  }
}
function start() {
  if (timer !== undefined) window.clearInterval(timer)
  if (open.value && !document.hidden) timer = window.setInterval(load, 60_000)
}
function visibility() {
  if (document.hidden) {
    if (timer !== undefined) window.clearInterval(timer)
    timer = undefined
  } else if (open.value) {
    void load()
    start()
  }
}
watch([open, range, () => props.target], async ([visible]) => {
  if (!visible) {
    if (timer !== undefined) window.clearInterval(timer)
    timer = undefined
    return
  }
  await load()
  start()
}, { deep: true })
document.addEventListener('visibilitychange', visibility)
onBeforeUnmount(() => {
  if (timer !== undefined) window.clearInterval(timer)
  document.removeEventListener('visibilitychange', visibility)
})
const bytes = (value: number) => value < 0 ? '—' : `${(value / 1024 / 1024).toFixed(1)} MiB`
const seconds = (value: number) => value < 0 ? '—' : `${value.toFixed(2)} s`
const percent = (value: number) => value < 0 ? '—' : `${(value * 100).toFixed(1)}%`
const duration = (value: number) => {
  if (value < 0) return '—'
  const days = Math.floor(value / 86400)
  const hours = Math.floor(value % 86400 / 3600)
  const minutes = Math.floor(value % 3600 / 60)
  return days > 0 ? `${days}d ${hours}h` : hours > 0 ? `${hours}h ${minutes}m` : `${minutes}m`
}
</script>

<style scoped>
.instance-title{margin:0;color:var(--paper-050);font:14px var(--font-mono);overflow-wrap:anywhere}
.instance-sub{margin:4px 0 16px;color:var(--steel-400);font-size:12px}
.reason{margin-bottom:16px}
.summary{margin-bottom:18px}
.panel-title{margin:0 0 10px;color:var(--paper-050);font-size:13px}
.trend-toolbar{display:flex;align-items:center;justify-content:space-between;gap:12px;flex-wrap:wrap;margin-bottom:10px}
.trend-toolbar>div{display:flex;align-items:center;gap:10px}
.hint-inline,.hint{color:var(--steel-400);font:10px var(--font-mono)}
.hint{margin:-8px 0 0;line-height:1.6}
.trend-panel{min-height:260px;padding:16px;border:1px solid var(--ink-600);border-radius:10px;background:var(--ink-850)}
</style>
