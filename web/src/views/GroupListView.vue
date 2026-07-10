<template>
  <div style="display: flex; margin-bottom: 12px">
    <div style="flex: 1" />
    <el-button type="primary" @click="$router.push('/policygroups/new')">{{ $t('common.create') }}</el-button>
  </div>
  <el-table :data="items" v-loading="loading">
    <el-table-column :label="$t('common.name')" prop="name" sortable>
      <template #default="{ row }">
        <router-link :to="`/policygroups/${row.name}`">{{ row.name }}</router-link>
      </template>
    </el-table-column>
    <el-table-column :label="$t('group.displayName')" prop="displayName" sortable />
    <el-table-column :label="$t('group.enabled')" prop="enabled" sortable width="110">
      <template #default="{ row }">
        <el-switch
          :model-value="row.enabled"
          :loading="toggling === row.name"
          @change="onToggleEnabled(row)"
        />
      </template>
    </el-table-column>
    <el-table-column :label="$t('policy.phase')" prop="phase" sortable width="120">
      <template #default="{ row }"><PhaseTag :phase="row.phase" /></template>
    </el-table-column>
    <el-table-column :label="$t('group.resolvedCount')" prop="resolvedCount" sortable width="110" />
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
import { deleteGroup, listGroups, setGroupEnabled, type GroupListItem } from '../api/group'
import PhaseTag from '../components/PhaseTag.vue'

const { t } = useI18n()
const items = ref<GroupListItem[]>([])
const loading = ref(false)
const toggling = ref('')

async function onToggleEnabled(row: GroupListItem) {
  toggling.value = row.name
  try {
    const next = !row.enabled
    await setGroupEnabled(row.name, next)
    ElMessage.success(t(next ? 'group.enableDone' : 'group.disableDone', { name: row.name }))
    await load()
  } catch (e) {
    ElMessage.error((e as Error).message)
  } finally {
    toggling.value = ''
  }
}

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
