<template>
  <div>
    <div class="trend-legend">
      <button
        v-for="series in seriesOptions"
        :key="series.key"
        class="legend"
        :class="{ muted: !active.includes(series.key), hit: series.key === hitKey }"
        type="button"
        @click="toggle(series.key)"
      >
        <span class="swatch" :style="{ background: series.color }" />
        <span>{{ $t(series.label) }}</span>
        <b>{{ formatSeriesValue(series.key, seriesPeak(points, series.key)) }}</b>
      </button>
    </div>
    <div v-if="points.length" class="chart-frame">
      <svg
        ref="plot"
        viewBox="0 0 900 250"
        role="img"
        :aria-label="$t('platformObservability.instanceTrend')"
        @pointermove="trackTime"
        @pointerleave="clear"
      >
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
            :stroke-width="series.key === hitKey ? 3.4 : 2.2"
            :opacity="hitKey && series.key !== hitKey ? 0.25 : 1"
            stroke-linecap="round"
            stroke-linejoin="round"
          />
        </g>
        <!-- Transparent wide strokes on top: the browser hit-tests these, so
             pointing at a line is decided by SVG z-order rather than by our own
             distance maths. Two flat lines at the same height stay separately
             selectable, which a nearest-distance search cannot do. -->
        <g class="hit-layer">
          <path
            v-for="target in hitTargets"
            :key="target.id"
            :d="target.d"
            fill="none"
            stroke="transparent"
            stroke-width="18"
            stroke-linecap="round"
            @pointerenter="hitKey = target.key"
            @pointermove="trackTime"
          />
        </g>
        <g v-if="hitKey && hovered >= 0" class="cursor">
          <line :x1="cursorX" :x2="cursorX" y1="8" y2="218" />
          <circle :cx="cursorX" :cy="hitY" r="4" :fill="hitColor" stroke="var(--ink-900)" stroke-width="1.5" />
        </g>
        <text x="24" y="242">{{ time(points[0]?.bucketStartedAt) }}</text>
        <text x="876" y="242" text-anchor="end">{{ time(points.at(-1)?.bucketStartedAt) }}</text>
      </svg>
      <dl v-if="hitKey && hovered >= 0" class="readout" :style="readoutStyle">
        <dt><span class="swatch" :style="{ background: hitColor }" />{{ $t(hitLabel) }}</dt>
        <dd class="readout-value">{{ formatSeriesValue(hitKey, points[hovered]?.[hitKey] ?? -1) }}</dd>
        <dd v-if="isFlat(hitKey)" class="readout-flat">{{ $t('platformObservability.flatSeries') }}</dd>
        <dd class="readout-time">{{ time(points[hovered]?.bucketStartedAt) }}</dd>
      </dl>
      <p class="hover-hint">{{ $t('platformObservability.hoverHint') }}</p>
    </div>
    <el-empty v-else :description="$t('platformObservability.noTrendData')" :image-size="72" />
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import type { PlatformTrendPoint } from '../api/platform'
import {
  formatSeriesValue,
  nearestPointIndex,
  platformLineSegments,
  seriesPeak,
  seriesY,
  type PlatformTrendKey,
} from './platformTrend'

const props = defineProps<{ points: PlatformTrendPoint[] }>()
const seriesOptions: { key: PlatformTrendKey; label: string; color: string }[] = [
  { key: 'memoryBytes', label: 'platformObservability.memory', color: '#33d6c0' },
  { key: 'cpuUsageMicros', label: 'platformObservability.cpu', color: '#7aa2ff' },
  { key: 'goroutines', label: 'platformObservability.goroutines', color: '#eab764' },
  { key: 'gcPauseMicros', label: 'platformObservability.gcPause', color: '#ff6b7a' },
]
const PLOT_LEFT = 24
const PLOT_WIDTH = 852
const PLOT_HEIGHT = 210
const PLOT_TOP = 8

const active = ref<PlatformTrendKey[]>(seriesOptions.map(series => series.key))
const hovered = ref(-1)
const hitKey = ref<PlatformTrendKey | null>(null)
const plot = ref<SVGSVGElement>()
const visible = computed(() => seriesOptions.filter(series => active.value.includes(series.key)))
const shift = (path: string) => path.replace(
  /([0-9.-]+) ([0-9.-]+)/g,
  (_, x, y) => `${Number(x) + PLOT_LEFT} ${Number(y) + PLOT_TOP}`,
)
const paths = (key: PlatformTrendKey) =>
  platformLineSegments(props.points, key, PLOT_WIDTH, PLOT_HEIGHT).map(shift)
// One hit target per drawn segment, keyed so Vue can reuse them across polls.
const hitTargets = computed(() => visible.value.flatMap(series =>
  paths(series.key).map((d, index) => ({ id: `${series.key}-${index}`, key: series.key, d })),
))
const isFlat = (key: PlatformTrendKey) => seriesPeak(props.points, key) <= 0

const toggle = (key: PlatformTrendKey) => {
  if (active.value.includes(key)) {
    if (active.value.length > 1) active.value = active.value.filter(value => value !== key)
  } else active.value = [...active.value, key]
  if (hitKey.value && !active.value.includes(hitKey.value)) clear()
}

// Which line is decided by the hit layer; this only resolves which bucket along
// that line the pointer is over.
function trackTime(event: PointerEvent) {
  const box = plot.value?.getBoundingClientRect()
  if (!box || !box.width) return
  const viewX = ((event.clientX - box.left) / box.width) * 900
  hovered.value = nearestPointIndex((viewX - PLOT_LEFT) / PLOT_WIDTH, props.points.length)
}
function clear() {
  hovered.value = -1
  hitKey.value = null
}

const cursorX = computed(() => {
  const denominator = Math.max(props.points.length - 1, 1)
  return PLOT_LEFT + (hovered.value / denominator) * PLOT_WIDTH
})
const hitY = computed(() => {
  if (!hitKey.value) return 0
  const y = seriesY(props.points, hitKey.value, hovered.value, PLOT_HEIGHT)
  return y === null ? 0 : y + PLOT_TOP
})
const hitColor = computed(() => seriesOptions.find(series => series.key === hitKey.value)?.color ?? '')
const hitLabel = computed(() => seriesOptions.find(series => series.key === hitKey.value)?.label ?? '')
// Flip the readout to the left half once the cursor passes the midpoint so it
// never runs off the panel.
const readoutStyle = computed(() => cursorX.value > 480
  ? { right: `${((900 - cursorX.value) / 900) * 100 + 2}%` }
  : { left: `${(cursorX.value / 900) * 100 + 2}%` })

const time = (value?: string) => value
  ? new Date(value).toLocaleString([], { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' })
  : ''
</script>

<style scoped>
.trend-legend{display:flex;flex-wrap:wrap;gap:8px;margin-bottom:12px}
.legend{display:flex;align-items:center;gap:7px;border:1px solid var(--ink-600);border-radius:999px;background:var(--ink-800);color:var(--paper-050);padding:5px 11px;font:11px var(--font-mono);cursor:pointer}
.legend b{color:var(--steel-300);font-weight:400}.legend.muted{opacity:.4}
.legend.hit{border-color:var(--accent-scan)}
.swatch{width:7px;height:7px;border-radius:50%;flex:none}
.chart-frame{position:relative;overflow-x:auto}svg{display:block;width:100%;min-width:620px;touch-action:none}
.grid line{stroke:var(--ink-600);stroke-dasharray:3 5}text{fill:var(--steel-400);font:10px var(--font-mono)}
.hit-layer path{cursor:crosshair}
.cursor line{stroke:var(--steel-300);stroke-width:1;stroke-dasharray:2 3}
.cursor,.cursor *{pointer-events:none}
.readout{position:absolute;top:8px;margin:0;padding:9px 11px;border:1px solid var(--ink-600);border-radius:8px;background:rgba(12,18,24,.97);pointer-events:none;min-width:132px}
.readout dt{display:flex;align-items:center;gap:6px;color:var(--steel-300);font-size:11px}
.readout-value{margin:5px 0 0;color:var(--paper-050);font:600 15px var(--font-mono)}
.readout-flat{margin:3px 0 0;color:var(--steel-400);font-size:10px}
.readout-time{margin:3px 0 0;color:var(--steel-400);font:10px var(--font-mono)}
.hover-hint{margin:8px 0 0;color:var(--steel-400);font:10px var(--font-mono)}
</style>
