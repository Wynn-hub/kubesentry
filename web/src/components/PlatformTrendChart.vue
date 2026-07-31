<template>
  <div>
    <div class="trend-legend">
      <button
        v-for="series in seriesOptions"
        :key="series.key"
        class="legend"
        :class="{ muted: !active.includes(series.key) }"
        type="button"
        @click="toggle(series.key)"
      >
        <span :style="{ background: series.color }" />{{ $t(series.label) }}
      </button>
    </div>
    <div v-if="points.length" class="chart-frame">
      <svg viewBox="0 0 900 250" role="img" :aria-label="$t('platformObservability.instanceTrend')">
        <g class="grid">
          <line v-for="line in 5" :key="line" x1="24" x2="876" :y1="line * 42" :y2="line * 42" />
        </g>
        <g v-for="series in visible" :key="series.key">
          <path
            v-for="(path, index) in paths(series.key)"
            :key="index"
            :d="path"
            fill="none"
            :stroke="series.color"
            stroke-width="2.2"
            stroke-linecap="round"
            stroke-linejoin="round"
          />
        </g>
        <text x="24" y="242">{{ time(points[0]?.bucketStartedAt) }}</text>
        <text x="876" y="242" text-anchor="end">{{ time(points.at(-1)?.bucketStartedAt) }}</text>
      </svg>
    </div>
    <el-empty v-else :description="$t('platformObservability.noTrendData')" :image-size="72" />
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import type { PlatformTrendPoint } from '../api/platform'
import { platformLineSegments, type PlatformTrendKey } from './platformTrend'

const props = defineProps<{ points: PlatformTrendPoint[] }>()
const seriesOptions: { key: PlatformTrendKey; label: string; color: string }[] = [
  { key: 'memoryBytes', label: 'platformObservability.memory', color: '#33d6c0' },
  { key: 'cpuUsageMicros', label: 'platformObservability.cpu', color: '#7aa2ff' },
  { key: 'goroutines', label: 'platformObservability.goroutines', color: '#eab764' },
  { key: 'gcPauseMicros', label: 'platformObservability.gcPause', color: '#ff6b7a' },
]
const active = ref<PlatformTrendKey[]>(seriesOptions.map(series => series.key))
const visible = computed(() => seriesOptions.filter(series => active.value.includes(series.key)))
const paths = (key: PlatformTrendKey) => platformLineSegments(props.points, key, 852, 210)
  .map(path => path.replace(/([0-9.-]+) ([0-9.-]+)/g, (_, x, y) => `${Number(x) + 24} ${Number(y) + 8}`))
const toggle = (key: PlatformTrendKey) => {
  if (active.value.includes(key)) {
    if (active.value.length > 1) active.value = active.value.filter(value => value !== key)
  } else active.value = [...active.value, key]
}
const time = (value?: string) => value
  ? new Date(value).toLocaleString([], { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' })
  : ''
</script>

<style scoped>
.trend-legend{display:flex;flex-wrap:wrap;gap:8px;margin-bottom:12px}
.legend{display:flex;align-items:center;gap:7px;border:1px solid var(--ink-600);border-radius:999px;background:var(--ink-800);color:var(--paper-050);padding:5px 10px;font:11px var(--font-mono);cursor:pointer}
.legend span{width:7px;height:7px;border-radius:50%}.legend.muted{opacity:.4}
.chart-frame{overflow-x:auto}svg{display:block;width:100%;min-width:620px}
.grid line{stroke:var(--ink-600);stroke-dasharray:3 5}text{fill:var(--steel-400);font:10px var(--font-mono)}
</style>
