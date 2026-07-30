<template>
  <div class="trend">
    <div class="trend-legend">
      <button
        v-for="series in seriesOptions"
        :key="series.key"
        type="button"
        class="legend-button"
        :class="{ muted: !active.includes(series.key) }"
        @click="toggle(series.key)"
      >
        <span class="legend-dot" :style="{ background: series.color }" />{{ $t(series.label) }}
      </button>
    </div>
    <div v-if="points.length" class="chart-frame">
      <svg :viewBox="`0 0 ${width + 58} ${height + 34}`" role="img" :aria-label="$t('observability.trend')">
        <g class="grid">
          <line v-for="line in 5" :key="line" x1="46" :x2="width + 46" :y1="gridY(line)" :y2="gridY(line)" />
          <text v-for="line in 5" :key="`label-${line}`" x="38" :y="gridY(line) + 4" text-anchor="end">{{ gridLabel(line) }}</text>
        </g>
        <g v-for="series in visibleSeries" :key="series.key">
          <path
            v-for="(path, index) in paths(series.key)"
            :key="index"
            :d="path"
            fill="none"
            :stroke="series.color"
            stroke-width="2.25"
            stroke-linecap="round"
            stroke-linejoin="round"
            vector-effect="non-scaling-stroke"
          />
        </g>
        <text x="46" :y="height + 27" class="axis-time">{{ timeLabel(points[0]?.bucketStartedAt) }}</text>
        <text :x="width + 46" :y="height + 27" text-anchor="end" class="axis-time">{{ timeLabel(points.at(-1)?.bucketStartedAt) }}</text>
      </svg>
    </div>
    <el-empty v-else :description="$t('observability.noTrendData')" :image-size="72" />
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import type { ObservationTrendPoint } from '../api/observability'
import { observationLineSegments, observationTrendMax, type ObservationTrendKey } from './observationTrend'

const props = defineProps<{ points: ObservationTrendPoint[] }>()
const width = 840
const height = 220
const seriesOptions: { key: ObservationTrendKey; label: string; color: string }[] = [
  { key: 'generated', label: 'observability.generated', color: '#33d6c0' },
  { key: 'written', label: 'observability.written', color: '#7aa2ff' },
  { key: 'faults', label: 'observability.faults', color: '#ff6b7a' },
  { key: 'policyFiltered', label: 'observability.policyFiltered', color: '#eab764' },
]
const active = ref<ObservationTrendKey[]>(['generated', 'written', 'faults'])
const visibleSeries = computed(() => seriesOptions.filter(series => active.value.includes(series.key)))
const maximum = computed(() => observationTrendMax(props.points, active.value))
const paths = (key: ObservationTrendKey) =>
  observationLineSegments(props.points, key, maximum.value, width, height).map(path => path.replace(/([ML]) /g, '$1 ').replace(/([0-9.-]+) ([0-9.-]+)/g, (_, x, y) => `${Number(x) + 46} ${y}`))
const toggle = (key: ObservationTrendKey) => {
  if (active.value.includes(key)) {
    if (active.value.length > 1) active.value = active.value.filter(item => item !== key)
  } else active.value = [...active.value, key]
}
const gridY = (line: number) => ((line - 1) / 4) * height
const gridLabel = (line: number) => Math.round(maximum.value * (1 - (line - 1) / 4)).toLocaleString()
const timeLabel = (value?: string) => value ? new Date(value).toLocaleString([], { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' }) : ''
</script>

<style scoped>
.trend-legend{display:flex;flex-wrap:wrap;gap:8px;margin-bottom:14px}
.legend-button{display:inline-flex;align-items:center;gap:7px;border:1px solid var(--ink-600);border-radius:999px;background:var(--ink-800);color:var(--paper-050);padding:5px 10px;font:12px var(--font-mono);cursor:pointer}
.legend-button.muted{opacity:.42}
.legend-dot{width:7px;height:7px;border-radius:50%}
.chart-frame{width:100%;overflow-x:auto}
svg{display:block;width:100%;min-width:620px;overflow:visible}
.grid line{stroke:var(--ink-600);stroke-width:1;stroke-dasharray:3 5}
.grid text,.axis-time{fill:var(--steel-400);font:10px var(--font-mono)}
</style>
