<template>
  <el-drawer v-model="open" :title="title" size="min(860px, 94vw)" destroy-on-close>
    <template v-if="target">
      <div class="drawer-toolbar">
        <el-select v-model="range" style="width:160px">
          <el-option :label="$t('admissionEvent.last1h')" value="1h" />
          <el-option :label="$t('admissionEvent.last24h')" value="24h" />
          <el-option :label="$t('admissionEvent.last7d')" value="7d" />
        </el-select>
        <span>{{ $t('platformObservability.trendRefresh') }}</span>
      </div>
      <el-descriptions :column="2" border class="summary">
        <el-descriptions-item :label="$t('platformObservability.role')">{{ role }}</el-descriptions-item>
        <el-descriptions-item :label="$t('platformObservability.health')">{{ $t(`platformObservability.instance_${target.instance.health}`) }}</el-descriptions-item>
        <el-descriptions-item :label="$t('platformObservability.uptime')">{{ duration(target.instance.uptimeSeconds) }}</el-descriptions-item>
        <el-descriptions-item :label="$t('platformObservability.restarts')">{{ target.instance.restartCount }}</el-descriptions-item>
        <el-descriptions-item :label="$t('platformObservability.memory')">{{ bytes(target.instance.memoryBytes) }}</el-descriptions-item>
        <el-descriptions-item v-if="target.component !== 'console-web' && target.component !== 'postgres'" :label="$t('platformObservability.goroutines')">
          {{ platformMetricText(target.instance.goroutines) }}
        </el-descriptions-item>
      </el-descriptions>
      <div v-loading="loading" class="trend-panel">
        <PlatformTrendChart :points="trend?.items ?? []" />
      </div>
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
import { platformMetricText } from './platformTrend'

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

const title = computed(() => props.target ? `${props.target.component} · ${props.target.instance.instance}` : '')
const role = computed(() => props.target?.instance.role === 'leader'
  ? t('platformObservability.leader')
  : props.target?.instance.role === 'standby' ? t('platformObservability.standby') : '—')
const span = () => ({ '1h': 36e5, '24h': 864e5, '7d': 6048e5 }[range.value] ?? 36e5)
async function load() {
  if (!props.target || document.hidden) return
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
const duration = (seconds: number) => seconds < 0 ? '—' : `${Math.floor(seconds / 86400)}d ${Math.floor(seconds % 86400 / 3600)}h`
</script>

<style scoped>
.drawer-toolbar{display:flex;align-items:center;justify-content:space-between;gap:12px;margin-bottom:14px;color:var(--steel-400);font:11px var(--font-mono)}
.summary{margin-bottom:18px}.trend-panel{min-height:260px;padding:16px;border:1px solid var(--ink-600);border-radius:10px;background:var(--ink-850)}
</style>
