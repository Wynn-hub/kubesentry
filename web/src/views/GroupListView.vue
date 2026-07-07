<template>
  <div style="display: flex; margin-bottom: 12px">
    <div style="flex: 1" />
    <el-button type="primary" @click="$router.push('/policygroups/new')">{{ $t('common.create') }}</el-button>
  </div>
  <el-table :data="items" v-loading="loading">
    <el-table-column :label="$t('common.name')">
      <template #default="{ row }">
        <router-link :to="`/policygroups/${row.name}`">{{ row.name }}</router-link>
      </template>
    </el-table-column>
    <el-table-column :label="$t('group.displayName')" prop="displayName" />
    <el-table-column :label="$t('group.enabled')" width="90">
      <template #default="{ row }">
        <el-tag :type="row.enabled ? 'success' : 'info'" size="small">{{ row.enabled }}</el-tag>
      </template>
    </el-table-column>
    <el-table-column :label="$t('policy.phase')" width="110">
      <template #default="{ row }"><PhaseTag :phase="row.phase" /></template>
    </el-table-column>
    <el-table-column :label="$t('group.resolvedCount')" prop="resolvedCount" width="90" />
    <el-table-column :label="$t('common.actions')" width="160">
      <template #default="{ row }">
        <el-button link type="primary" @click="$router.push(`/policygroups/${row.name}/edit`)">
          {{ $t('common.edit') }}
        </el-button>
        <el-button link type="danger" @click="onDelete(row)">{{ $t('common.delete') }}</el-button>
      </template>
    </el-table-column>
  </el-table>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useI18n } from 'vue-i18n'
import { deleteGroup, listGroups, type GroupListItem } from '../api/group'
import PhaseTag from '../components/PhaseTag.vue'

const { t } = useI18n()
const items = ref<GroupListItem[]>([])
const loading = ref(false)

async function load() {
  loading.value = true
  try {
    items.value = await listGroups()
  } catch (e) {
    ElMessage.error((e as Error).message)
  } finally {
    loading.value = false
  }
}

async function onDelete(row: GroupListItem) {
  try {
    await ElMessageBox.confirm(t('common.deleteConfirm', { name: row.name }))
    if (row.source === 'builtin') await ElMessageBox.confirm(t('common.builtinWarning'))
    await deleteGroup(row.name)
    ElMessage.success(t('common.deleted'))
    await load()
  } catch (e) {
    if (e !== 'cancel') ElMessage.error((e as Error).message)
  }
}

onMounted(load)
</script>
