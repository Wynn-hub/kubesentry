<!-- web/src/views/ClusterDetailView.vue -->
<template>
  <div v-if="d">
    <PageHeader show-back :title="d.name">
      <ClusterPhaseTag :phase="d.phase" />
      <el-button v-if="can('platform.manage') && d.phase === 'Registering'" @click="onReissue">{{ $t('cluster.reissueToken') }}</el-button>
      <el-tooltip v-if="can('platform.manage') && d.phase === 'Online'" :content="$t('cluster.deleteOnlineBlocked')">
        <el-button type="danger" plain disabled>{{ $t('common.delete') }}</el-button>
      </el-tooltip>
      <el-button v-else-if="can('platform.manage')" type="danger" plain @click="onDelete">{{ $t('common.delete') }}</el-button>
    </PageHeader>

    <el-descriptions :column="2" border style="margin-bottom: 16px">
      <el-descriptions-item :label="$t('cluster.clusterUid')">{{ d.clusterUid || '-' }}</el-descriptions-item>
      <el-descriptions-item :label="$t('cluster.operatorVersion')">{{ d.operatorVersion || '-' }}</el-descriptions-item>
      <el-descriptions-item :label="$t('cluster.kubernetesVersion')">{{ d.kubernetesVersion || '-' }}</el-descriptions-item>
      <el-descriptions-item :label="$t('cluster.nodes')">
        <span v-if="d.nodeCount > 0">
          {{ $t('cluster.nodesReadyOfTotal', { ready: d.readyNodeCount, total: d.nodeCount }) }}
        </span>
        <span v-else>-</span>
      </el-descriptions-item>
      <el-descriptions-item :label="$t('cluster.apiServerAddress')" :span="2">{{ d.apiServerAddress || '-' }}</el-descriptions-item>
      <el-descriptions-item :label="$t('cluster.lastHeartbeatAt')">
        {{ d.lastHeartbeatAt ? relTime(d.lastHeartbeatAt, locale) : $t('cluster.neverHeartbeat') }}
      </el-descriptions-item>
      <el-descriptions-item :label="$t('cluster.createdAt')">{{ new Date(d.createdAt).toLocaleString() }}</el-descriptions-item>
      <el-descriptions-item :label="$t('cluster.updatedAt')" :span="2">{{ new Date(d.updatedAt).toLocaleString() }}</el-descriptions-item>
      <el-descriptions-item v-if="can('platform.manage')" :label="$t('cluster.tenant')" :span="2">
        <el-select :model-value="d.tenantId" clearable :placeholder="$t('cluster.unassigned')" @change="onTenantChange">
          <el-option v-for="tenant in tenants" :key="tenant.id" :value="tenant.id!" :label="tenant.name" />
        </el-select>
      </el-descriptions-item>
    </el-descriptions>

    <h3>{{ $t('cluster.labels') }}</h3>
    <ClusterLabelsEditor v-if="can('platform.manage')" v-model="labelsDraft" @update:model-value="onLabelsChange" />
    <el-tag v-for="(value, key) in labelsDraft" v-else :key="key" style="margin-right: 8px">{{ key }}={{ value }}</el-tag>

    <section v-if="uninstallTasks.length" style="margin-top: 24px">
      <h3>{{ $t('cluster.uninstallTasks') }}</h3>
      <el-table :data="uninstallTasks" size="small">
        <el-table-column prop="id" label="ID" width="90" />
        <el-table-column prop="status" :label="$t('cluster.phase')" width="130" />
        <el-table-column prop="error" :label="$t('cluster.uninstallFailureReason')" min-width="360" show-overflow-tooltip>
          <template #default="{ row }">{{ row.error || '-' }}</template>
        </el-table-column>
        <el-table-column :label="$t('common.actions')" width="110">
          <template #default="{ row }">
            <el-button v-if="can('platform.manage') && row.status === 'Failed'" link type="warning" @click="onRetryUninstall(row.id)">{{ $t('cluster.retryUninstall') }}</el-button>
          </template>
        </el-table-column>
      </el-table>
    </section>

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
import { can } from '../auth/session'
import { onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useI18n } from 'vue-i18n'
import {
  assignClusterTenant, deleteCluster, getCluster, getLatestInstallParams, listUninstallTasks, reissueToken, retryUninstallTask, updateClusterLabels,
  DEFAULT_INSTALL_PARAMS, type ClusterDetail, type InstallParams, type UninstallTask,
} from '../api/cluster'
import { listTenants, type Tenant } from '../api/identity'
import { relTime } from '../lib/relTime'
import ClusterPhaseTag from '../components/ClusterPhaseTag.vue'
import ClusterLabelsEditor from '../components/ClusterLabelsEditor.vue'
import InstallParamsForm from '../components/InstallParamsForm.vue'
import PageHeader from '../components/PageHeader.vue'

const route = useRoute()
const router = useRouter()
const { t, locale } = useI18n()

const d = ref<ClusterDetail>()
const labelsDraft = ref<Record<string, string>>({})
const uninstallTasks = ref<UninstallTask[]>([])
const tenants = ref<Tenant[]>([])
let refreshTimer: ReturnType<typeof setInterval> | undefined

async function load() {
  try {
    d.value = await getCluster(String(route.params.name))
    labelsDraft.value = d.value.labels ?? {}
    uninstallTasks.value = await listUninstallTasks(d.value.name)
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}
onMounted(async () => {
  if (can('platform.manage')) {
    try { tenants.value = (await listTenants() ?? []).filter(tenant => tenant.status === 'active') }
    catch (e) { ElMessage.error((e as Error).message) }
  }
  await load()
  refreshTimer = setInterval(() => { void load() }, 5000)
})
onBeforeUnmount(() => { if (refreshTimer) clearInterval(refreshTimer) })

async function onTenantChange(tenantId?: number) {
  if (!d.value) return
  try { await assignClusterTenant(d.value.name, tenantId); await load() }
  catch (e) { ElMessage.error((e as Error).message) }
}

async function onRetryUninstall(taskID: number) {
  if (!d.value) return
  try {
    await retryUninstallTask(d.value.name, String(taskID))
    ElMessage.success(t('cluster.uninstallStarted'))
    await load()
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}

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
