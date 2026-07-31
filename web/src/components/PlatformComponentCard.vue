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
        <span class="instance-name" :title="instance.instance">{{ instance.instance }}</span>
        <span v-if="component.hasLeadership" class="role">{{ roleLabel(instance.role) }}</span>
        <span class="instance-health" :class="`health-${instance.health}`">
          {{ $t(`platformObservability.instance_${instance.health}`) }}
        </span>
        <span v-if="component.component !== 'postgres'" class="metric">
          {{ $t('platformObservability.memory') }} {{ memoryLabel(instance) }}
        </span>
        <span v-if="component.component !== 'postgres'" class="metric">
          {{ $t('platformObservability.cpu') }} {{ platformMetricText(instance.cpuMillis, 'm') }}
        </span>
        <template v-if="component.component === 'postgres'">
          <span class="metric">{{ $t('platformObservability.connections') }} {{ dbRatio(instance, 'connections', 'maxConnections') }}</span>
          <span class="metric">{{ $t('platformObservability.cacheHit') }} {{ percent(instance.dbStats?.cacheHitRatio) }}</span>
        </template>
      </button>
    </div>
    <el-empty v-else :description="$t('platformObservability.noInstanceData')" :image-size="46" />
    <p v-if="consoleWebNoData" class="proxy-hint">
      {{ $t('platformObservability.consoleWebProxyHint') }}
    </p>

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
import { platformMetricText } from './platformTrend'

const props = defineProps<{ component: PlatformComponent }>()
defineEmits<{ select: [instance: PlatformInstance]; history: [] }>()
const { t } = useI18n()

const componentLabel = computed(() => t(`platformObservability.component_${props.component.component.replace('-', '_')}`))
const healthTag = computed(() => props.component.health === 'ok' ? 'success' : props.component.health === 'degraded' ? 'warning' : 'info')
const consoleWebNoData = computed(() =>
  props.component.component === 'console-web' &&
  props.component.instances.some(instance => instance.health === 'no_data'))
const roleLabel = (role: string) => role === 'leader'
  ? t('platformObservability.leader')
  : role === 'standby' ? t('platformObservability.standby') : '—'
const mib = (value: number) => value < 0 ? '—' : `${(value / 1024 / 1024).toFixed(value < 10 * 1024 * 1024 ? 1 : 0)} MiB`
const memoryLabel = (instance: PlatformInstance) => instance.memoryLimitBytes > 0
  ? `${mib(instance.memoryBytes)} / ${mib(instance.memoryLimitBytes)}`
  : mib(instance.memoryBytes)
const dbRatio = (instance: PlatformInstance, value: string, maximum: string) => {
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
.component-card.health-degraded::before{background:#eab764}.component-card.health-unavailable::before{background:var(--steel-400)}
header{display:flex;align-items:center;justify-content:space-between;gap:8px;margin-bottom:12px}header>div{display:flex;align-items:center;gap:8px;min-width:0}
header strong{color:var(--paper-050);font-size:14px}.status-dot{width:8px;height:8px;border-radius:50%;background:var(--accent-scan)}
.health-degraded .status-dot{background:#eab764}.health-unavailable .status-dot{background:var(--steel-400)}
.instances{display:grid;gap:7px}.instance{display:grid;grid-template-columns:minmax(0,1fr) auto;gap:4px 8px;width:100%;padding:9px;border:1px solid var(--ink-700);border-radius:7px;background:var(--ink-800);color:var(--steel-300);text-align:left;cursor:pointer}
.instance:hover{border-color:var(--accent-scan)}.instance-name{overflow:hidden;text-overflow:ellipsis;color:var(--paper-050);font:11px var(--font-mono);white-space:nowrap}
.role{color:var(--accent-scan);font-size:11px}.instance-health{font-size:10px}.instance-health.health-degraded{color:#eab764}.instance-health.health-no_data{color:var(--steel-400)}
.metric{justify-self:end;color:var(--steel-400);font:10px var(--font-mono)}.history{margin-top:11px;padding:0;border:0;background:transparent;color:var(--accent-scan);font-size:11px;cursor:pointer}
.proxy-hint{margin:10px 0 0;color:#eab764;font-size:10px;line-height:1.45}
</style>
