<!-- web/src/views/PolicyListView.vue -->
<template>
  <div style="display: flex; gap: 8px; margin-bottom: 12px">
    <el-select v-model="filters.source" clearable :placeholder="$t('policy.allSources')" style="width: 140px">
      <el-option :label="$t('policy.builtin')" value="builtin" />
      <el-option :label="$t('policy.custom')" value="custom" />
    </el-select>
    <el-select v-model="filters.phase" clearable :placeholder="$t('policy.allPhases')" style="width: 140px">
      <el-option v-for="p in ['Ready', 'Invalid', 'Syncing']" :key="p" :label="p" :value="p" />
    </el-select>
    <el-input v-model="filters.keyword" :placeholder="$t('common.search')" style="width: 200px" clearable />
    <el-button type="primary" @click="load">{{ $t('common.search') }}</el-button>
    <div style="flex: 1" />
    <el-button type="primary" @click="$router.push('/policies/new')">{{ $t('common.create') }}</el-button>
  </div>

  <el-table :data="items" v-loading="loading">
    <el-table-column :label="$t('common.name')" prop="name">
      <template #default="{ row }">
        <router-link :to="`/policies/${row.name}`">{{ row.name }}</router-link>
      </template>
    </el-table-column>
    <el-table-column :label="$t('policy.source')" width="100">
      <template #default="{ row }">
        <el-tag size="small" :type="row.source === 'builtin' ? 'info' : 'primary'">{{ row.source }}</el-tag>
      </template>
    </el-table-column>
    <el-table-column :label="$t('policy.mode')" width="100">
      <template #default="{ row }"><ModeTag :mode="row.enforcementMode" /></template>
    </el-table-column>
    <el-table-column :label="$t('policy.phase')" width="100">
      <template #default="{ row }"><PhaseTag :phase="row.phase" /></template>
    </el-table-column>
    <el-table-column :label="$t('policy.currentVersion')" width="100">
      <template #default="{ row }">
        <el-button link type="primary" @click="$router.push(`/policies/${row.name}?tab=versions`)">
          v{{ row.currentVersion }}
        </el-button>
      </template>
    </el-table-column>
    <el-table-column :label="$t('policy.referencedBy')">
      <template #default="{ row }">{{ (row.referencedBy ?? []).join(', ') }}</template>
    </el-table-column>
    <el-table-column :label="$t('common.actions')" width="160">
      <template #default="{ row }">
        <el-button link type="primary" @click="$router.push(`/policies/${row.name}/edit`)">
          {{ $t('common.edit') }}
        </el-button>
        <el-button link type="danger" @click="onDelete(row)">{{ $t('common.delete') }}</el-button>
      </template>
    </el-table-column>
  </el-table>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useI18n } from 'vue-i18n'
import { ApiError } from '../api/http'
import { deletePolicy, listPolicies, type PolicyListItem } from '../api/policy'
import ModeTag from '../components/ModeTag.vue'
import PhaseTag from '../components/PhaseTag.vue'

const { t } = useI18n()
const items = ref<PolicyListItem[]>([])
const loading = ref(false)
const filters = reactive({ source: '', phase: '', keyword: '' })

async function load() {
  loading.value = true
  try {
    const params: Record<string, string> = {}
    if (filters.source) params.source = filters.source
    if (filters.phase) params.phase = filters.phase
    if (filters.keyword) params.keyword = filters.keyword
    items.value = await listPolicies(params)
  } catch (e) {
    ElMessage.error((e as Error).message)
  } finally {
    loading.value = false
  }
}

async function onDelete(row: PolicyListItem) {
  try {
    await ElMessageBox.confirm(t('common.deleteConfirm', { name: row.name }))
    if (row.source === 'builtin') {
      await ElMessageBox.confirm(t('common.builtinWarning'))
    }
    await doDelete(row.name, false)
  } catch (e) {
    if (e === 'cancel') return
  }
}

async function doDelete(name: string, force: boolean) {
  try {
    await deletePolicy(name, force)
    ElMessage.success(t('common.deleted'))
    await load()
  } catch (e) {
    if (e instanceof ApiError && e.status === 409 && e.data) {
      const groups = ((e.data as { referencedBy?: string[] }).referencedBy ?? []).join(', ')
      try {
        await ElMessageBox.confirm(t('policy.referencedDeleteConfirm', { groups }))
        await doDelete(name, true)
      } catch {
        /* cancelled */
      }
      return
    }
    ElMessage.error((e as Error).message)
  }
}

onMounted(load)
</script>
