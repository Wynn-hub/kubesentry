<template>
  <div style="display: flex; margin-bottom: 12px">
    <div style="flex: 1" />
    <el-button type="primary" @click="$router.push('/exceptions/new')">{{ $t('common.create') }}</el-button>
  </div>
  <el-table :data="items" v-loading="loading">
    <el-table-column :label="$t('common.name')" prop="name" sortable />
    <el-table-column :label="$t('policy.phase')" prop="phase" sortable width="120">
      <template #default="{ row }"><PhaseTag :phase="row.phase" /></template>
    </el-table-column>
    <el-table-column :label="$t('exception.target')" prop="targetSummary" sortable />
    <el-table-column :label="$t('exception.duration')" prop="duration" sortable width="110" />
    <el-table-column :label="$t('exception.expiresAt')" prop="expiresAt" sortable width="200">
      <template #default="{ row }">{{ row.expiresAt ? new Date(row.expiresAt).toLocaleString() : '-' }}</template>
    </el-table-column>
    <el-table-column :label="$t('exception.reason')" prop="reason" />
    <el-table-column :label="$t('common.actions')" width="160">
      <template #default="{ row }">
        <el-button link type="primary" @click="$router.push(`/exceptions/${row.name}/edit`)">
          {{ $t('common.edit') }}
        </el-button>
        <el-button link type="danger" @click="onDelete(row.name)">{{ $t('common.delete') }}</el-button>
      </template>
    </el-table-column>
  </el-table>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useI18n } from 'vue-i18n'
import { deleteException, listExceptions, type ExceptionListItem } from '../api/exception'
import PhaseTag from '../components/PhaseTag.vue'

const { t } = useI18n()
const items = ref<ExceptionListItem[]>([])
const loading = ref(false)

async function load() {
  loading.value = true
  try {
    items.value = await listExceptions()
  } catch (e) {
    ElMessage.error((e as Error).message)
  } finally {
    loading.value = false
  }
}

async function onDelete(name: string) {
  try {
    await ElMessageBox.confirm(t('common.deleteConfirm', { name }))
    await deleteException(name)
    ElMessage.success(t('common.deleted'))
    await load()
  } catch (e) {
    if (e !== 'cancel') ElMessage.error((e as Error).message)
  }
}

onMounted(load)
</script>
