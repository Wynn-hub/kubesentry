<template>
  <PageHeader :title="$t('cluster.title')" :subtitle="$t('cluster.subtitle')">
    <el-button type="primary" @click="createDialogOpen = true">{{ $t('common.create') }}</el-button>
  </PageHeader>

  <el-tabs v-model="activeTab">
    <el-tab-pane :label="$t('cluster.title')" name="clusters">
      <el-table :data="items" v-loading="loading">
        <el-table-column :label="$t('cluster.name')" prop="name" sortable min-width="160">
          <template #default="{ row }">
            <router-link :to="`/clusters/${row.name}`">{{ row.name }}</router-link>
          </template>
        </el-table-column>
        <el-table-column :label="$t('cluster.phase')" prop="phase" sortable width="120">
          <template #default="{ row }"><ClusterPhaseTag :phase="row.phase" /></template>
        </el-table-column>
        <el-table-column :label="$t('cluster.operatorVersion')" prop="operatorVersion" width="140" />
        <el-table-column :label="$t('cluster.lastHeartbeatAt')" width="160">
          <template #default="{ row }">
            {{ row.lastHeartbeatAt ? relTime(row.lastHeartbeatAt, locale) : $t('cluster.neverHeartbeat') }}
          </template>
        </el-table-column>
        <el-table-column :label="$t('cluster.createdAt')" width="155">
          <template #default="{ row }">{{ new Date(row.createdAt).toLocaleString() }}</template>
        </el-table-column>
        <el-table-column :label="$t('common.actions')" width="130" fixed="right">
          <template #default="{ row }">
            <el-button link type="primary" @click="$router.push(`/clusters/${row.name}`)">{{ $t('common.view') }}</el-button>
            <el-tooltip v-if="row.phase === 'Online'" :content="$t('cluster.deleteOnlineBlocked')">
              <el-button link type="danger" disabled>{{ $t('common.delete') }}</el-button>
            </el-tooltip>
            <el-button v-else link type="danger" @click="onDelete(row)">{{ $t('common.delete') }}</el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-tab-pane>
    <el-tab-pane :label="$t('cluster.installRecords')" name="records" lazy>
      <el-table :data="installRecords" v-loading="recordsLoading">
        <el-table-column :label="$t('cluster.name')" prop="clusterName" min-width="140" />
        <el-table-column :label="$t('cluster.namespace')" prop="namespace" width="160" />
        <el-table-column :label="$t('cluster.webhookImage')" width="220">
          <template #default="{ row }">{{ row.webhookRepository }}:{{ row.webhookTag }}</template>
        </el-table-column>
        <el-table-column :label="$t('cluster.operatorImage')" width="220">
          <template #default="{ row }">{{ row.operatorRepository }}:{{ row.operatorTag }}</template>
        </el-table-column>
        <el-table-column :label="$t('cluster.gatewayAddress')" prop="gatewayAddress" width="160" />
        <el-table-column :label="$t('cluster.createdAt')" width="155">
          <template #default="{ row }">{{ new Date(row.createdAt).toLocaleString() }}</template>
        </el-table-column>
      </el-table>
    </el-tab-pane>
  </el-tabs>

  <el-dialog v-model="createDialogOpen" :title="$t('cluster.createTitle')" width="560px" @closed="resetCreateForm">
    <template v-if="createStep === 'basic'">
      <p style="color: var(--el-text-color-secondary); margin-top: 0">{{ $t('cluster.createHint') }}</p>
      <el-form :model="createForm" label-width="80px">
        <el-form-item :label="$t('cluster.name')" required>
          <el-input v-model="createForm.name" />
        </el-form-item>
        <el-form-item :label="$t('cluster.labels')">
          <ClusterLabelsEditor v-model="createForm.labels" />
        </el-form-item>
      </el-form>
    </template>
    <template v-else-if="createStep === 'install'">
      <InstallParamsForm v-model="installParams" />
    </template>
    <template v-else>
      <p>{{ $t('cluster.tokenTitle') }}</p>
      <el-input :model-value="generatedCommand" readonly type="textarea" :rows="8" style="font-family: var(--font-mono)" />
      <el-button style="margin-top: 8px" @click="copyCommand">{{ $t('common.copy') }}</el-button>
      <el-alert :title="$t('cluster.tokenHint')" type="warning" :closable="false" style="margin-top: 12px" />
    </template>
    <template #footer>
      <template v-if="createStep === 'basic'">
        <el-button @click="createDialogOpen = false">{{ $t('common.cancel') }}</el-button>
        <el-button type="primary" @click="createStep = 'install'">{{ $t('cluster.next') }}</el-button>
      </template>
      <template v-else-if="createStep === 'install'">
        <el-button @click="createStep = 'basic'">{{ $t('cluster.prev') }}</el-button>
        <el-button type="primary" :loading="creating" @click="onCreate">{{ $t('common.create') }}</el-button>
      </template>
      <template v-else>
        <el-button type="primary" @click="createDialogOpen = false">{{ $t('common.confirm') }}</el-button>
      </template>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useI18n } from 'vue-i18n'
import {
  createCluster, deleteCluster, listClusters, listInstallRecords,
  DEFAULT_INSTALL_PARAMS, type ClusterListItem, type InstallParams, type InstallRecordItem,
} from '../api/cluster'
import { relTime } from '../lib/relTime'
import PageHeader from '../components/PageHeader.vue'
import ClusterPhaseTag from '../components/ClusterPhaseTag.vue'
import ClusterLabelsEditor from '../components/ClusterLabelsEditor.vue'
import InstallParamsForm from '../components/InstallParamsForm.vue'

const { t, locale } = useI18n()
const activeTab = ref('clusters')
const items = ref<ClusterListItem[]>([])
const loading = ref(false)

async function load() {
  loading.value = true
  try {
    items.value = await listClusters()
  } catch (e) {
    ElMessage.error((e as Error).message)
  } finally {
    loading.value = false
  }
}
onMounted(load)

const installRecords = ref<InstallRecordItem[]>([])
const recordsLoading = ref(false)
async function loadInstallRecords() {
  recordsLoading.value = true
  try {
    installRecords.value = await listInstallRecords()
  } catch (e) {
    ElMessage.error((e as Error).message)
  } finally {
    recordsLoading.value = false
  }
}
// The "records" pane is `lazy`, so it never mounts (and never loads data)
// until the user actually clicks that tab — fire the load the first time
// that happens instead of on component mount.
watch(activeTab, (tab) => {
  if (tab === 'records' && installRecords.value.length === 0) loadInstallRecords()
})

async function onDelete(row: ClusterListItem) {
  try {
    await ElMessageBox.confirm(t('common.deleteConfirm', { name: row.name }))
    await deleteCluster(row.name)
    ElMessage.success(t('common.deleted'))
    await load()
  } catch (e) {
    if (e === 'cancel') return
    ElMessage.error((e as Error).message)
  }
}

const createDialogOpen = ref(false)
const creating = ref(false)
const createStep = ref<'basic' | 'install' | 'command'>('basic')
const generatedCommand = ref('')
const createForm = reactive({ name: '', labels: {} as Record<string, string> })
const installParams = reactive<InstallParams>(structuredClone(DEFAULT_INSTALL_PARAMS))

function resetCreateForm() {
  createStep.value = 'basic'
  createForm.name = ''
  createForm.labels = {}
  generatedCommand.value = ''
  Object.assign(installParams, structuredClone(DEFAULT_INSTALL_PARAMS))
}

async function onCreate() {
  if (!createForm.name) {
    ElMessage.error(t('cluster.name'))
    return
  }
  creating.value = true
  try {
    const res = await createCluster({ name: createForm.name, labels: createForm.labels, installParams })
    generatedCommand.value = res.command
    createStep.value = 'command'
    await load()
    // The records tab, if the user has already opened it once, should reflect
    // the new history row without requiring a manual switch away and back.
    if (activeTab.value === 'records') await loadInstallRecords()
  } catch (e) {
    ElMessage.error((e as Error).message)
  } finally {
    creating.value = false
  }
}

async function copyCommand() {
  await navigator.clipboard.writeText(generatedCommand.value)
  ElMessage.success(t('cluster.tokenCopied'))
}
</script>
