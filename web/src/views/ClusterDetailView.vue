<!-- web/src/views/ClusterDetailView.vue -->
<template>
  <div v-if="d">
    <div style="display: flex; align-items: center; gap: 12px; margin-bottom: 12px">
      <h2 style="margin: 0">{{ d.name }}</h2>
      <ClusterPhaseTag :phase="d.phase" />
      <div style="flex: 1" />
      <el-button v-if="d.phase === 'Registering'" @click="onReissue">{{ $t('cluster.reissueToken') }}</el-button>
      <el-tooltip v-if="d.phase === 'Online'" :content="$t('cluster.deleteOnlineBlocked')">
        <el-button type="danger" plain disabled>{{ $t('common.delete') }}</el-button>
      </el-tooltip>
      <el-button v-else type="danger" plain @click="onDelete">{{ $t('common.delete') }}</el-button>
      <el-button @click="$router.back()">{{ $t('common.back') }}</el-button>
    </div>

    <el-descriptions :column="2" border style="margin-bottom: 16px">
      <el-descriptions-item :label="$t('cluster.clusterUid')">{{ d.clusterUid || '-' }}</el-descriptions-item>
      <el-descriptions-item :label="$t('cluster.operatorVersion')">{{ d.operatorVersion || '-' }}</el-descriptions-item>
      <el-descriptions-item :label="$t('cluster.lastHeartbeatAt')">
        {{ d.lastHeartbeatAt ? relTime(d.lastHeartbeatAt, locale) : $t('cluster.neverHeartbeat') }}
      </el-descriptions-item>
      <el-descriptions-item :label="$t('cluster.createdAt')">{{ new Date(d.createdAt).toLocaleString() }}</el-descriptions-item>
      <el-descriptions-item :label="$t('cluster.updatedAt')" :span="2">{{ new Date(d.updatedAt).toLocaleString() }}</el-descriptions-item>
    </el-descriptions>

    <h3>{{ $t('cluster.labels') }}</h3>
    <ClusterLabelsEditor v-model="labelsDraft" @update:model-value="onLabelsChange" />

    <el-dialog v-model="reissueDialogOpen" :title="$t('cluster.reissueToken')" width="560px">
      <template v-if="!generatedCommand">
        <InstallParamsForm v-model="installParams" />
      </template>
      <template v-else>
        <p>{{ $t('cluster.tokenTitle') }}</p>
        <el-input :model-value="generatedCommand" readonly type="textarea" :rows="8" style="font-family: var(--font-mono)" />
        <el-button style="margin-top: 8px" @click="copyCommand">{{ $t('common.copy') }}</el-button>
        <el-alert :title="$t('cluster.tokenHint')" type="warning" :closable="false" style="margin-top: 12px" />
      </template>
      <template #footer>
        <template v-if="!generatedCommand">
          <el-button @click="reissueDialogOpen = false">{{ $t('common.cancel') }}</el-button>
          <el-button type="primary" :loading="reissuing" @click="onSubmitReissue">{{ $t('common.confirm') }}</el-button>
        </template>
        <template v-else>
          <el-button type="primary" @click="reissueDialogOpen = false">{{ $t('common.confirm') }}</el-button>
        </template>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useI18n } from 'vue-i18n'
import {
  deleteCluster, getCluster, getLatestInstallParams, reissueToken, updateClusterLabels,
  DEFAULT_INSTALL_PARAMS, type ClusterDetail, type InstallParams,
} from '../api/cluster'
import { relTime } from '../lib/relTime'
import ClusterPhaseTag from '../components/ClusterPhaseTag.vue'
import ClusterLabelsEditor from '../components/ClusterLabelsEditor.vue'
import InstallParamsForm from '../components/InstallParamsForm.vue'

const route = useRoute()
const router = useRouter()
const { t, locale } = useI18n()

const d = ref<ClusterDetail>()
const labelsDraft = ref<Record<string, string>>({})

async function load() {
  try {
    d.value = await getCluster(String(route.params.name))
    labelsDraft.value = d.value.labels ?? {}
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}
onMounted(load)

async function onLabelsChange(labels: Record<string, string>) {
  if (!d.value) return
  try {
    await updateClusterLabels(d.value.name, labels)
    ElMessage.success(t('common.saved'))
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}

async function onDelete() {
  if (!d.value) return
  try {
    await ElMessageBox.confirm(t('common.deleteConfirm', { name: d.value.name }))
    await deleteCluster(d.value.name)
    ElMessage.success(t('common.deleted'))
    router.push('/clusters')
  } catch (e) {
    if (e === 'cancel') return
    ElMessage.error((e as Error).message)
  }
}

const reissueDialogOpen = ref(false)
const reissuing = ref(false)
const generatedCommand = ref('')
const installParams = reactive<InstallParams>(structuredClone(DEFAULT_INSTALL_PARAMS))

async function onReissue() {
  if (!d.value) return
  generatedCommand.value = ''
  try {
    const latest = await getLatestInstallParams(d.value.name)
    Object.assign(installParams, latest ?? structuredClone(DEFAULT_INSTALL_PARAMS))
  } catch (e) {
    ElMessage.error((e as Error).message)
    return
  }
  reissueDialogOpen.value = true
}

async function onSubmitReissue() {
  if (!d.value) return
  try {
    await ElMessageBox.confirm(t('cluster.reissueConfirm', { name: d.value.name }))
  } catch {
    return
  }
  reissuing.value = true
  try {
    const res = await reissueToken(d.value.name, installParams)
    generatedCommand.value = res.command
  } catch (e) {
    ElMessage.error((e as Error).message)
  } finally {
    reissuing.value = false
  }
}

async function copyCommand() {
  await navigator.clipboard.writeText(generatedCommand.value)
  ElMessage.success(t('cluster.tokenCopied'))
}
</script>
