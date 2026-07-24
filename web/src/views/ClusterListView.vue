<template>
  <PageHeader :title="$t('cluster.title')" :subtitle="$t('cluster.subtitle')">
    <el-button type="primary" @click="createDialogOpen = true">{{ $t('common.create') }}</el-button>
  </PageHeader>

  <el-tabs v-model="activeTab">
    <el-tab-pane :label="$t('cluster.title')" name="clusters">
      <el-table
        :key="`${clustersState.sortBy}:${clustersState.sortOrder}`"
        :data="items"
        :default-sort="clustersTableSort"
        v-loading="clustersLoading"
        @sort-change="onClustersSortChange"
      >
        <el-table-column :label="$t('cluster.name')" prop="name" sortable="custom" :sort-orders="sortOrders" min-width="220" show-overflow-tooltip>
          <template #default="{ row }">
            <router-link :to="`/clusters/${row.name}`">{{ row.name }}</router-link>
          </template>
        </el-table-column>
        <el-table-column :label="$t('cluster.phase')" prop="phase" sortable="custom" :sort-orders="sortOrders" width="160">
          <template #default="{ row }"><ClusterPhaseTag :phase="row.phase" /></template>
        </el-table-column>
        <el-table-column :label="$t('cluster.operatorVersion')" prop="operatorVersion" sortable="custom" :sort-orders="sortOrders" width="170" show-overflow-tooltip />
        <el-table-column :label="$t('cluster.lastHeartbeatAt')" prop="lastHeartbeatAt" sortable="custom" :sort-orders="sortOrders" width="175">
          <template #default="{ row }">
            {{ row.lastHeartbeatAt ? relTime(row.lastHeartbeatAt, locale) : $t('cluster.neverHeartbeat') }}
          </template>
        </el-table-column>
        <el-table-column :label="$t('cluster.createdAt')" prop="createdAt" sortable="custom" :sort-orders="sortOrders" width="185">
          <template #default="{ row }">{{ new Date(row.createdAt).toLocaleString() }}</template>
        </el-table-column>
        <el-table-column :label="$t('common.actions')" width="126" fixed="right" align="right" class-name="cluster-actions">
          <template #default="{ row }">
            <el-button link type="primary" @click="$router.push(`/clusters/${row.name}`)">{{ $t('common.view') }}</el-button>
            <el-dropdown trigger="click" @command="(command: string) => onClusterAction(command, row)">
              <el-button link class="more-actions" :aria-label="$t('common.actions')">
                <el-icon><MoreFilled /></el-icon>
              </el-button>
              <template #dropdown>
                <el-dropdown-menu>
                  <el-dropdown-item v-if="row.phase === 'Online'" command="uninstall" class="warning-action">
                    {{ $t('cluster.uninstall') }}
                  </el-dropdown-item>
                  <el-tooltip v-if="row.phase === 'Online'" :content="$t('cluster.deleteOnlineBlocked')" placement="left">
                    <span class="disabled-menu-item">{{ $t('common.delete') }}</span>
                  </el-tooltip>
                  <el-dropdown-item v-else command="delete" divided class="danger-action">
                    {{ $t('common.delete') }}
                  </el-dropdown-item>
                </el-dropdown-menu>
              </template>
            </el-dropdown>
          </template>
        </el-table-column>
      </el-table>
      <PaginationBar
        :total="clustersTotal"
        :page="clustersState.page"
        :page-size="clustersState.pageSize"
        @update:page="(page) => clustersRouteList.replace({ page })"
        @update:page-size="(pageSize) => clustersRouteList.replace({ pageSize })"
      />
    </el-tab-pane>
    <el-tab-pane :label="$t('cluster.installRecords')" name="records" lazy>
      <el-table
        :key="`${recordsState.sortBy}:${recordsState.sortOrder}`"
        :data="installRecords"
        :default-sort="recordsTableSort"
        v-loading="recordsLoading"
        @sort-change="onRecordsSortChange"
      >
        <el-table-column :label="$t('cluster.name')" prop="clusterName" sortable="custom" :sort-orders="sortOrders" min-width="140" />
        <el-table-column :label="$t('cluster.namespace')" prop="namespace" sortable="custom" :sort-orders="sortOrders" width="160" />
        <el-table-column :label="$t('cluster.webhookImage')" width="220">
          <template #default="{ row }">{{ row.webhookRepository }}:{{ row.webhookTag }}</template>
        </el-table-column>
        <el-table-column :label="$t('cluster.operatorImage')" width="220">
          <template #default="{ row }">{{ row.operatorRepository }}:{{ row.operatorTag }}</template>
        </el-table-column>
        <el-table-column :label="$t('cluster.highAvailability')" width="130">
          <template #default="{ row }">
            <el-tag :type="row.highAvailability ? 'success' : 'info'" effect="plain">
              {{ row.highAvailability ? $t('common.enabled') : $t('common.disabled') }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column :label="$t('cluster.gatewayAddress')" prop="gatewayAddress" width="160" />
        <el-table-column :label="$t('cluster.createdAt')" prop="createdAt" sortable="custom" :sort-orders="sortOrders" width="155">
          <template #default="{ row }">{{ new Date(row.createdAt).toLocaleString() }}</template>
        </el-table-column>
      </el-table>
      <PaginationBar
        :total="recordsTotal"
        :page="recordsState.page"
        :page-size="recordsState.pageSize"
        @update:page="(page) => recordsRouteList.replace({ page })"
        @update:page-size="(pageSize) => recordsRouteList.replace({ pageSize })"
      />
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
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { MoreFilled } from '@element-plus/icons-vue'
import { useI18n } from 'vue-i18n'
import {
  createCluster, deleteCluster, listClusters, listInstallRecords, uninstallCluster,
  DEFAULT_INSTALL_PARAMS, type ClusterListItem, type InstallParams, type InstallRecordItem,
} from '../api/cluster'
import { relTime } from '../lib/relTime'
import PageHeader from '../components/PageHeader.vue'
import ClusterPhaseTag from '../components/ClusterPhaseTag.vue'
import ClusterLabelsEditor from '../components/ClusterLabelsEditor.vue'
import InstallParamsForm from '../components/InstallParamsForm.vue'
import PaginationBar from '../components/PaginationBar.vue'
import { listStateParams, useRouteListState, type SortOrder } from '../composables/useRouteListState'
import { useLatestRequest } from '../composables/useLatestRequest'

const sortOrders = ['ascending', 'descending'] as const

const { t, locale } = useI18n()
const route = useRoute()
const router = useRouter()
const activeTab = computed<'clusters' | 'records'>({
  get: () => route.query.tab === 'records' ? 'records' : 'clusters',
  set: (tab) => {
    void router.replace({ query: { ...route.query, tab } })
  },
})

watch(
  () => route.query.tab,
  (tab) => {
    if (tab !== 'clusters' && tab !== 'records') {
      void router.replace({ query: { ...route.query, tab: 'clusters' } })
    }
  },
  { immediate: true },
)

const items = ref<ClusterListItem[]>([])
const clustersTotal = ref(0)
const { loading: clustersLoading, run: runLatestClusters } = useLatestRequest()
const clustersRouteList = useRouteListState({
  prefix: 'clusters',
  defaultSortBy: 'name',
  defaultSortOrder: 'asc',
  allowedSort: ['name', 'phase', 'operatorVersion', 'lastHeartbeatAt', 'createdAt'],
})
const clustersState = clustersRouteList.state
const clustersParams = computed(() => listStateParams(clustersState.value))
const clustersTableSort = computed(() => ({
  prop: clustersState.value.sortBy,
  order: clustersState.value.sortOrder === 'asc' ? 'ascending' : 'descending',
}))

async function loadClusters() {
  try {
    await runLatestClusters(
      () => listClusters(clustersParams.value),
      (result) => {
        items.value = result.items
        clustersTotal.value = result.total
        const lastPage = Math.max(1, Math.ceil(result.total / clustersState.value.pageSize))
        if (clustersState.value.page > lastPage) void clustersRouteList.replace({ page: lastPage })
      },
    )
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}
watch(clustersParams, loadClusters, { immediate: true })

// Keep the member-cluster lifecycle visible without a browser reload. This
// only refreshes the cluster list request and is cleaned up when navigating
// away from the cluster management view.
let clusterRefreshTimer: ReturnType<typeof setInterval> | undefined
onMounted(() => {
  clusterRefreshTimer = setInterval(() => { void loadClusters() }, 5000)
})
onBeforeUnmount(() => {
  if (clusterRefreshTimer) clearInterval(clusterRefreshTimer)
})

function onClustersSortChange({ prop, order }: { prop: string; order: 'ascending' | 'descending' }) {
  if (!prop || !order) return
  void clustersRouteList.replace({
    sortBy: prop,
    sortOrder: (order === 'ascending' ? 'asc' : 'desc') as SortOrder,
  })
}

const installRecords = ref<InstallRecordItem[]>([])
const recordsTotal = ref(0)
const recordsLoaded = ref(false)
const { loading: recordsLoading, run: runLatestRecords } = useLatestRequest()
const recordsRouteList = useRouteListState({
  prefix: 'records',
  defaultSortBy: 'createdAt',
  defaultSortOrder: 'desc',
  allowedSort: ['clusterName', 'namespace', 'createdAt'],
})
const recordsState = recordsRouteList.state
const recordsParams = computed(() => listStateParams(recordsState.value))
const recordsTableSort = computed(() => ({
  prop: recordsState.value.sortBy,
  order: recordsState.value.sortOrder === 'asc' ? 'ascending' : 'descending',
}))

async function loadInstallRecords() {
  try {
    await runLatestRecords(
      () => listInstallRecords(recordsParams.value),
      (result) => {
        installRecords.value = result.items
        recordsTotal.value = result.total
        recordsLoaded.value = true
        const lastPage = Math.max(1, Math.ceil(result.total / recordsState.value.pageSize))
        if (recordsState.value.page > lastPage) void recordsRouteList.replace({ page: lastPage })
      },
    )
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}

watch(recordsParams, () => {
  if (activeTab.value === 'records' || recordsLoaded.value) void loadInstallRecords()
})
watch(activeTab, (tab) => {
  if (tab === 'records' && !recordsLoaded.value) void loadInstallRecords()
}, { immediate: true })

function onRecordsSortChange({ prop, order }: { prop: string; order: 'ascending' | 'descending' }) {
  if (!prop || !order) return
  void recordsRouteList.replace({
    sortBy: prop,
    sortOrder: (order === 'ascending' ? 'asc' : 'desc') as SortOrder,
  })
}

async function onDelete(row: ClusterListItem) {
  try {
    await ElMessageBox.confirm(t('common.deleteConfirm', { name: row.name }))
    await deleteCluster(row.name)
    ElMessage.success(t('common.deleted'))
    await loadClusters()
  } catch (e) {
    if (e === 'cancel') return
    ElMessage.error((e as Error).message)
  }
}

async function onUninstall(row: ClusterListItem) {
  try {
    const { value } = await ElMessageBox.prompt(
      t('cluster.uninstallConfirm', { name: row.name }),
      t('cluster.uninstall'),
      { inputPlaceholder: row.name, inputValidator: (input) => input === row.name || t('cluster.uninstallNameMismatch') },
    )
    if (value !== row.name) return
    await uninstallCluster(row.name)
    ElMessage.success(t('cluster.uninstallStarted'))
    await loadClusters()
  } catch (e) {
    if (e === 'cancel' || e === 'close') return
    ElMessage.error((e as Error).message)
  }
}

function onClusterAction(command: string, row: ClusterListItem) {
  if (command === 'uninstall') void onUninstall(row)
  if (command === 'delete' && row.phase !== 'Online') void onDelete(row)
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
    await loadClusters()
    // The records tab, if the user has already opened it once, should reflect
    // the new history row without requiring a manual switch away and back.
    if (recordsLoaded.value || activeTab.value === 'records') await loadInstallRecords()
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

<style scoped>
.more-actions {
  margin-left: 8px;
  min-width: 26px;
  padding: 4px;
}

:deep(.cluster-actions .cell) {
  display: flex;
  align-items: center;
  justify-content: flex-end;
}

:deep(.warning-action) { color: var(--el-color-warning); }
:deep(.danger-action) { color: var(--el-color-danger); }
.disabled-menu-item {
  display: block;
  padding: 5px 16px;
  color: var(--el-text-color-disabled);
  cursor: not-allowed;
}
</style>
