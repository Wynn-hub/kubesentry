<template>
  <article class="component-card" :class="`health-${component.health}`">
    <header>
      <div>
        <span class="status-dot" />
        <strong>{{ componentLabel }}</strong>
      </div>
      <el-tag :type="healthTag" size="small" effect="dark">{{ $t(`platformObservability.health_${component.health}`) }}</el-tag>
    </header>

    <div v-if="component.instances.length" class="instances">
      <button
        v-for="instance in component.instances"
        :key="instance.instance"
        class="instance"
        type="button"
        @click="$emit('select', instance)"
      >
        <el-tooltip :content="instance.instance" placement="top" :show-after="150">
          <!-- Pod names differ only in the trailing hash, so clipping the head
               keeps the part that identifies the replica. -->
          <span class="instance-name" dir="rtl">{{ instance.instance }}</span>
        </el-tooltip>
        <span v-if="component.hasLeadership" class="role" :class="{ leader: instance.role === 'leader' }">
          {{ roleLabel(instance.role) }}
        </span>
        <span class="instance-health" :class="`health-${instance.health}`">
          {{ $t(`platformObservability.instance_${instance.health}`) }}
        </span>
        <template v-if="component.component === 'postgres'">
          <span class="metric">{{ $t('platformObservability.connections') }} {{ metricsStale(instance) ? '—' : dbRatio(instance, 'connections', 'maxConnections') }}</span>
          <span class="metric">{{ $t('platformObservability.cacheHit') }} {{ metricsStale(instance) ? '—' : percent(instance.dbStats?.cacheHitRatio) }}</span>
        </template>
        <template v-else-if="component.component === 'console-web'">
          <span class="metric">{{ $t('platformObservability.memory') }} {{ metricsStale(instance) ? '—' : memoryLabel(instance) }}</span>
          <span class="metric">{{ $t('platformObservability.httpProbe') }} {{ probeLabel(instance) }}</span>
        </template>
        <template v-else>
          <span class="metric">{{ $t('platformObservability.memory') }} {{ metricsStale(instance) ? '—' : memoryLabel(instance) }}</span>
          <span class="metric">{{ $t('platformObservability.cpu') }} {{ metricsStale(instance) ? '—' : platformMetricText(instance.cpuMillis, 'm') }}</span>
        </template>
        <span v-if="instance.health !== 'ok' && instance.healthReason" class="instance-reason">
          {{ $t(`platformObservability.reason_${instance.healthReason}`) }}
        </span>
      </button>
    </div>
    <el-empty v-else :description="$t('platformObservability.noInstanceData')" :image-size="46" />

    <!-- A degraded card must always say why. The reason is carried up from the
         component so the operator does not have to open each instance. -->
    <p v-if="pending" class="card-hint">{{ $t('platformObservability.reason_component_pending') }}</p>
    <p v-else-if="component.health !== 'ok' && component.healthReason" class="card-reason">
      <span class="reason-icon">!</span>{{ $t(`platformObservability.reason_${component.healthReason}`) }}
    </p>
    <p v-if="component.singleReplica" class="card-hint">{{ $t('platformObservability.singleReplicaHint') }}</p>
    <p v-if="consoleWebNoData" class="card-hint">{{ $t('platformObservability.consoleWebProxyHint') }}</p>

    <button
      v-if="component.hasLeadership && component.lastLeadershipChange"
      class="history"
      type="button"
      @click="$emit('history')"
    >
      {{ $t('platformObservability.changedAt', { time: relative(component.lastLeadershipChange) }) }} ›
    </button>
  </article>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { PlatformComponent, PlatformInstance } from '../api/platform'
import { platformMetricsStale, platformMetricText } from './platformTrend'

const props = defineProps<{ component: PlatformComponent }>()
defineEmits<{ select: [instance: PlatformInstance]; history: [] }>()
const { t } = useI18n()

const componentLabel = computed(() => t(`platformObservability.component_${props.component.component.replace('-', '_')}`))
const healthTag = computed(() => props.component.health === 'ok'
  ? 'success'
  : props.component.health === 'degraded' ? 'warning' : 'info')
// Pending means "no first sample yet", so the card must not look like a fault.
const pending = computed(() => props.component.health === 'pending')
const consoleWebNoData = computed(() =>
  props.component.component === 'console-web' &&
  props.component.instances.some(instance => instance.health === 'no_data'))
const metricsStale = platformMetricsStale
const roleLabel = (role: string) => role === 'leader'
  ? t('platformObservability.leader')
  : role === 'standby' ? t('platformObservability.standby') : '—'
const mib = (value: number) => value < 0 ? '—' : `${(value / 1024 / 1024).toFixed(value < 10 * 1024 * 1024 ? 1 : 0)} MiB`
const memoryLabel = (instance: PlatformInstance) => instance.memoryLimitBytes > 0
  ? `${mib(instance.memoryBytes)} / ${mib(instance.memoryLimitBytes)}`
  : mib(instance.memoryBytes)
const probeLabel = (instance: PlatformInstance) => instance.healthReason === 'console_web_probe_failed'
  ? t('platformObservability.probeFailed')
  : t('platformObservability.probeOk')
type DatabaseStatKey = keyof NonNullable<PlatformInstance['dbStats']>
const dbRatio = (instance: PlatformInstance, value: DatabaseStatKey, maximum: DatabaseStatKey) => {
  const current = instance.dbStats?.[value] ?? -1
  const limit = instance.dbStats?.[maximum] ?? -1
  return limit >= 0 ? `${platformMetricText(current)} / ${platformMetricText(limit)}` : platformMetricText(current)
}
const percent = (value?: number) => value === undefined || value < 0 ? '—' : `${(value * 100).toFixed(1)}%`
const relative = (value: string) => {
  const minutes = Math.max(1, Math.round((Date.now() - new Date(value).getTime()) / 60000))
  const [amount, unit] = minutes >= 1440
    ? [Math.round(minutes / 1440), 'day']
    : minutes >= 60 ? [Math.round(minutes / 60), 'hour'] : [minutes, 'minute']
  return new Intl.RelativeTimeFormat(undefined, { numeric: 'auto' })
    .format(-amount, unit as Intl.RelativeTimeFormatUnit)
}
</script>

<style scoped>
.component-card{position:relative;min-width:0;padding:16px;border:1px solid var(--ink-600);border-radius:10px;background:linear-gradient(145deg,rgba(30,42,52,.94),rgba(18,27,35,.96));overflow:hidden}
.component-card::before{content:"";position:absolute;inset:0 auto 0 0;width:2px;background:var(--accent-scan)}
.component-card.health-degraded::before{background:#eab764}
.component-card.health-unavailable::before,.component-card.health-pending::before{background:var(--steel-400)}
header{display:flex;align-items:center;justify-content:space-between;gap:8px;margin-bottom:12px}header>div{display:flex;align-items:center;gap:8px;min-width:0}
header strong{color:var(--paper-050);font-size:14px}.status-dot{width:8px;height:8px;border-radius:50%;background:var(--accent-scan);flex:none}
.health-degraded .status-dot{background:#eab764}
.health-unavailable .status-dot,.health-pending .status-dot{background:var(--steel-400)}
.instances{display:grid;gap:7px}
.instance{display:grid;grid-template-columns:minmax(0,1fr) auto;gap:4px 8px;width:100%;padding:9px;border:1px solid var(--ink-700);border-radius:7px;background:var(--ink-800);color:var(--steel-300);text-align:left;cursor:pointer}
.instance:hover{border-color:var(--accent-scan)}
.instance-name{grid-column:1/-1;overflow:hidden;text-overflow:ellipsis;color:var(--paper-050);font:11px var(--font-mono);white-space:nowrap;text-align:left;direction:rtl}
.role{color:var(--steel-400);font-size:11px}.role.leader{color:var(--accent-scan);font-weight:600}
.instance-health{justify-self:end;font-size:10px}.instance-health.health-degraded{color:#eab764}.instance-health.health-no_data{color:var(--steel-400)}
.metric{color:var(--steel-400);font:10px var(--font-mono)}.metric:nth-of-type(even){justify-self:end}
.instance-reason{grid-column:1/-1;color:#eab764;font-size:10px;line-height:1.45}
.card-hint{margin:10px 0 0;color:var(--steel-400);font-size:10px;line-height:1.5}
.card-reason{display:flex;align-items:flex-start;gap:6px;margin:11px 0 0;padding:7px 9px;border-radius:6px;background:rgba(234,183,100,.1);color:#eab764;font-size:11px;line-height:1.5}
.reason-icon{flex:none;width:14px;height:14px;border-radius:50%;background:#eab764;color:var(--ink-900);font:700 10px/14px var(--font-mono);text-align:center}
.health-unavailable .card-reason{background:rgba(148,163,184,.12);color:var(--steel-300)}
.health-unavailable .reason-icon{background:var(--steel-300)}
.history{margin-top:11px;padding:0;border:0;background:transparent;color:var(--accent-scan);font-size:11px;cursor:pointer}
</style>
