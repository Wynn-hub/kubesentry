<template>
  <div v-if="tl">
    <el-alert v-if="tl.inFlight" :title="$t('timeline.inFlight')" type="warning" :closable="false" style="margin-bottom: 12px" />
    <el-timeline>
      <el-timeline-item
        v-for="v in tl.versions"
        :key="v.version"
        :timestamp="v.createdAt"
        :type="v.isCurrent ? 'primary' : undefined"
        :hollow="!v.isCurrent"
      >
        <div style="display: flex; align-items: center; gap: 8px">
          <b>{{ $t('timeline.version', { n: v.version }) }}</b>
          <PhaseTag :phase="v.phase" />
          <ModeTag :mode="v.enforcementMode" />
          <el-tag v-if="v.isCurrent" type="success" size="small">{{ $t('timeline.current') }}</el-tag>
          <el-button
            v-if="rollbackActionFor(v.version, tl) === 'prev'"
            size="small" type="warning" :loading="rolling" @click="doRollback('prev')"
          >{{ $t('timeline.rollbackPrev') }}</el-button>
          <el-button
            v-if="rollbackActionFor(v.version, tl) === 'next'"
            size="small" type="warning" :loading="rolling" @click="doRollback('next')"
          >{{ $t('timeline.rollbackNext') }}</el-button>
        </div>
        <el-collapse style="margin-top: 8px">
          <el-collapse-item :title="$t('policy.rego')">
            <pre style="font-family: monospace; white-space: pre-wrap">{{ v.rego }}</pre>
          </el-collapse-item>
        </el-collapse>
      </el-timeline-item>
    </el-timeline>
  </div>
</template>

<script setup lang="ts">
import { onMounted, onUnmounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { useI18n } from 'vue-i18n'
import { getTimeline, rollback, type VersionTimeline } from '../api/policy'
import { rollbackActionFor } from './timeline'
import ModeTag from './ModeTag.vue'
import PhaseTag from './PhaseTag.vue'

const props = defineProps<{ policyName: string }>()
const { t } = useI18n()
const tl = ref<VersionTimeline>()
const rolling = ref(false)
let pollTimer: ReturnType<typeof setInterval> | undefined

async function load() {
  try {
    tl.value = await getTimeline(props.policyName)
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}

async function doRollback(direction: 'prev' | 'next') {
  rolling.value = true
  try {
    await rollback(props.policyName, direction)
    // 轮询直到 operator 落盘（inFlight 消失且 currentVersion 前进）
    pollTimer = setInterval(async () => {
      await load()
      if (tl.value && !tl.value.inFlight) {
        clearInterval(pollTimer)
        rolling.value = false
        ElMessage.success(t('timeline.rollbackDone'))
      }
    }, 1000)
  } catch (e) {
    rolling.value = false
    ElMessage.error((e as Error).message)
  }
}

onMounted(load)
onUnmounted(() => clearInterval(pollTimer))
</script>
