<template>
  <PageHeader :title="$t('distribution.title')" :subtitle="$t('distribution.subtitle')">
    <el-button type="primary" @click="$router.push('/distributions/new')">
      {{ $t('common.create') }}
    </el-button>
  </PageHeader>

  <el-table
    :key="`${listState.sortBy}:${listState.sortOrder}`"
    :data="items"
    :default-sort="tableSort"
    v-loading="loading"
    @sort-change="onSortChange"
  >
    <el-table-column :label="$t('common.name')" prop="name" sortable="custom" :sort-orders="sortOrders" min-width="160" />
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
  <PaginationBar
    :total="total"
    :page="listState.page"
    :page-size="listState.pageSize"
    @update:page="(page) => routeList.replace({ page })"
    @update:page-size="(pageSize) => routeList.replace({ pageSize })"
  />
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useI18n } from 'vue-i18n'
import { deleteDistribution, listDistributions, type DistributionListItem } from '../api/distribution'
import PageHeader from '../components/PageHeader.vue'
import PaginationBar from '../components/PaginationBar.vue'
import { listStateParams, useRouteListState, type SortOrder } from '../composables/useRouteListState'
import { useLatestRequest } from '../composables/useLatestRequest'

const sortOrders = ['ascending', 'descending'] as const

const { t } = useI18n()
const items = ref<DistributionListItem[]>([])
const total = ref(0)
const { loading, run: runLatest } = useLatestRequest()
const routeList = useRouteListState({
  defaultSortBy: 'name',
  defaultSortOrder: 'asc',
  allowedSort: ['name'],
})
const listState = routeList.state
const tableSort = computed(() => ({
  prop: listState.value.sortBy,
  order: listState.value.sortOrder === 'asc' ? 'ascending' : 'descending',
}))
const requestParams = computed(() => listStateParams(listState.value))

async function load() {
  try {
    await runLatest(
      () => listDistributions(requestParams.value),
      (result) => {
        items.value = result.items
        total.value = result.total
        const lastPage = Math.max(1, Math.ceil(result.total / listState.value.pageSize))
        if (listState.value.page > lastPage) void routeList.replace({ page: lastPage })
      },
    )
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}

watch(requestParams, load, { immediate: true })

function onSortChange({ prop, order }: { prop: string; order: 'ascending' | 'descending' }) {
  if (!prop || !order) return
  void routeList.replace({ sortBy: prop, sortOrder: (order === 'ascending' ? 'asc' : 'desc') as SortOrder })
}

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
