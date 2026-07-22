<template>
  <PageHeader :title="$t('distribution.title')" :subtitle="$t('distribution.subtitle')">
    <el-button type="primary" @click="$router.push('/distributions/new')">
      {{ $t('common.create') }}
    </el-button>
  </PageHeader>

  <el-table :data="items" v-loading="loading">
    <el-table-column :label="$t('common.name')" prop="name" sortable min-width="160" />
    <el-table-column :label="$t('distribution.policyGroups')" min-width="240">
      <template #default="{ row }">{{ row.policyGroups.join(', ') }}</template>
    </el-table-column>
    <el-table-column :label="$t('common.actions')" width="160">
      <template #default="{ row }">
        <el-button link type="primary" @click="$router.push(`/distributions/${row.name}/edit`)">
          {{ $t('common.edit') }}
        </el-button>
        <el-tooltip v-if="row.name === 'default'" :content="$t('distribution.deleteDefaultBlocked')">
          <el-button link type="danger" disabled>{{ $t('common.delete') }}</el-button>
        </el-tooltip>
        <el-button v-else link type="danger" @click="onDelete(row)">{{ $t('common.delete') }}</el-button>
      </template>
    </el-table-column>
  </el-table>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useI18n } from 'vue-i18n'
import { deleteDistribution, listDistributions, type DistributionListItem } from '../api/distribution'
import PageHeader from '../components/PageHeader.vue'

const { t } = useI18n()
const items = ref<DistributionListItem[]>([])
const loading = ref(false)

async function load() {
  loading.value = true
  try {
    items.value = (await listDistributions()).items
  } catch (e) {
    ElMessage.error((e as Error).message)
  } finally {
    loading.value = false
  }
}
onMounted(load)

async function onDelete(row: DistributionListItem) {
  try {
    await ElMessageBox.confirm(t('common.deleteConfirm', { name: row.name }))
    await deleteDistribution(row.name)
    ElMessage.success(t('common.deleted'))
    await load()
  } catch (e) {
    if (e !== 'cancel') ElMessage.error((e as Error).message)
  }
}
</script>
