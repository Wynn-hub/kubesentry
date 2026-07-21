<template>
  <PageHeader :title="$t('cluster.title')" :subtitle="$t('cluster.subtitle')">
    <el-button type="primary" @click="createDialogOpen = true">{{ $t('common.create') }}</el-button>
  </PageHeader>

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

  <el-dialog v-model="createDialogOpen" :title="$t('cluster.createTitle')" width="520px" @closed="resetCreateForm">
    <template v-if="!createdToken">
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
    <template v-else>
      <p>{{ $t('cluster.tokenTitle') }}</p>
      <el-input :model-value="createdToken" readonly style="font-family: var(--font-mono)">
        <template #append>
          <el-button @click="copyToken">{{ $t('common.copy') }}</el-button>
        </template>
      </el-input>
      <el-alert :title="$t('cluster.tokenHint')" type="warning" :closable="false" style="margin-top: 12px" />
    </template>
    <template #footer>
      <template v-if="!createdToken">
        <el-button @click="createDialogOpen = false">{{ $t('common.cancel') }}</el-button>
        <el-button type="primary" :loading="creating" @click="onCreate">{{ $t('common.create') }}</el-button>
      </template>
      <template v-else>
        <el-button type="primary" @click="createDialogOpen = false">{{ $t('common.confirm') }}</el-button>
      </template>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useI18n } from 'vue-i18n'
import { createCluster, deleteCluster, listClusters, type ClusterListItem } from '../api/cluster'
import { relTime } from '../lib/relTime'
import PageHeader from '../components/PageHeader.vue'
import ClusterPhaseTag from '../components/ClusterPhaseTag.vue'
import ClusterLabelsEditor from '../components/ClusterLabelsEditor.vue'

const { t, locale } = useI18n()
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
const createdToken = ref('')
const createForm = reactive({ name: '', labels: {} as Record<string, string> })

function resetCreateForm() {
  createForm.name = ''
  createForm.labels = {}
  createdToken.value = ''
}

async function onCreate() {
  if (!createForm.name) {
    ElMessage.error(t('cluster.name'))
    return
  }
  creating.value = true
  try {
    const res = await createCluster({ name: createForm.name, labels: createForm.labels })
    createdToken.value = res.token
    await load()
  } catch (e) {
    ElMessage.error((e as Error).message)
  } finally {
    creating.value = false
  }
}

async function copyToken() {
  await navigator.clipboard.writeText(createdToken.value)
  ElMessage.success(t('cluster.tokenCopied'))
}
</script>
