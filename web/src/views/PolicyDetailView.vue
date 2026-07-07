<template>
  <div v-if="d">
    <div style="display: flex; align-items: center; gap: 12px; margin-bottom: 12px">
      <h2 style="margin: 0">{{ d.name }}</h2>
      <el-tag size="small" :type="d.source === 'builtin' ? 'info' : 'primary'">{{ d.source }}</el-tag>
      <PhaseTag :phase="d.status.phase" />
      <div style="flex: 1" />
      <el-button @click="$router.push(`/policies/${d.name}/edit`)">{{ $t('common.edit') }}</el-button>
      <el-button @click="$router.back()">{{ $t('common.back') }}</el-button>
    </div>

    <el-tabs>
      <el-tab-pane :label="$t('policy.tabOverview')">
        <el-descriptions :column="2" border>
          <el-descriptions-item :label="$t('common.description')">{{ d.spec.description }}</el-descriptions-item>
          <el-descriptions-item :label="$t('policy.mode')"><ModeTag :mode="d.spec.enforcementMode" /></el-descriptions-item>
          <el-descriptions-item :label="$t('policy.currentVersion')">v{{ d.status.currentVersion }}</el-descriptions-item>
          <el-descriptions-item :label="$t('policy.referencedBy')">{{ (d.status.referencedBy ?? []).join(', ') }}</el-descriptions-item>
          <el-descriptions-item :label="$t('policy.operations')">{{ d.spec.match.operations.join(', ') }}</el-descriptions-item>
          <el-descriptions-item :label="$t('policy.resources')">
            {{ d.spec.match.resources.map((r) => r.resources.join('/')).join('; ') }}
          </el-descriptions-item>
        </el-descriptions>
        <el-alert v-if="d.status.message" :title="d.status.message" type="error" :closable="false" style="margin-top: 12px" />
      </el-tab-pane>

      <el-tab-pane :label="$t('policy.tabRego')">
        <el-button size="small" style="margin-bottom: 8px" @click="copyRego">{{ $t('policy.copyRego') }}</el-button>
        <pre style="font-family: monospace; background: #f5f7fa; padding: 12px; white-space: pre-wrap">{{ d.spec.rego }}</pre>
      </el-tab-pane>

      <el-tab-pane :label="$t('policy.tabVersions')" lazy>
        <VersionTimeline :policy-name="d.name" />
      </el-tab-pane>
    </el-tabs>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { ElMessage } from 'element-plus'
import { useI18n } from 'vue-i18n'
import { getPolicy, type PolicyDetail } from '../api/policy'
import ModeTag from '../components/ModeTag.vue'
import PhaseTag from '../components/PhaseTag.vue'
import VersionTimeline from '../components/VersionTimeline.vue'

const route = useRoute()
const { t } = useI18n()
const d = ref<PolicyDetail>()

async function copyRego() {
  if (!d.value) return
  await navigator.clipboard.writeText(d.value.spec.rego)
  ElMessage.success(t('policy.copied'))
}

onMounted(async () => {
  try {
    d.value = await getPolicy(String(route.params.name))
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
})
</script>
