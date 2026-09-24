<template>
  <div v-if="d">
    <PageHeader show-back :title="d.name">
      <el-tag size="small" :type="d.source === 'builtin' ? 'info' : 'primary'">{{ d.source }}</el-tag>
      <PhaseTag :phase="d.phase" kind="policy" />
      <el-button @click="$router.push(`/policies/${d.name}/edit`)">{{ $t('common.edit') }}</el-button>
    </PageHeader>

    <el-tabs v-model="activeTab">
      <el-tab-pane :label="$t('policy.tabOverview')" name="overview">
        <el-descriptions :column="2" border>
          <el-descriptions-item :label="$t('common.description')">{{ d.description }}</el-descriptions-item>
          <el-descriptions-item :label="$t('policy.mode')"><ModeTag :mode="d.enforcementMode" /></el-descriptions-item>
          <el-descriptions-item :label="$t('policy.referencedBy')">{{ (d.referencedBy ?? []).join(', ') }}</el-descriptions-item>
          <el-descriptions-item :label="$t('policy.operations')">{{ d.match.operations.join(', ') }}</el-descriptions-item>
          <el-descriptions-item :label="$t('policy.resources')">
            {{ d.match.resources.map((r) => r.resources.join('/')).join('; ') }}
          </el-descriptions-item>
        </el-descriptions>
        <el-alert v-if="d.compileError" :title="d.compileError" type="error" :closable="false" style="margin-top: 12px" />
      </el-tab-pane>

      <el-tab-pane :label="$t('policy.tabRego')" name="rego">
        <el-button size="small" style="margin-bottom: 8px" @click="copyRego">{{ $t('policy.copyRego') }}</el-button>
        <pre
          style="
            font-family: var(--font-mono);
            background: var(--ink-900);
            border: 1px solid var(--ink-700);
            border-radius: 8px;
            padding: 14px;
            white-space: pre-wrap;
          "
          >{{ d.rego }}</pre
        >
      </el-tab-pane>

      <el-tab-pane :label="$t('policy.tabVersions')" name="versions" lazy>
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
import PageHeader from '../components/PageHeader.vue'

const route = useRoute()
const { t } = useI18n()
const d = ref<PolicyDetail>()
const activeTab = ref(route.query.tab === 'versions' ? 'versions' : 'overview')

async function copyRego() {
  if (!d.value) return
  await navigator.clipboard.writeText(d.value.rego)
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
