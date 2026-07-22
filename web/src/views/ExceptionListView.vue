<template>
  <PageHeader :title="$t('exception.title')" :subtitle="$t('exception.subtitle')">
    <el-button type="primary" @click="$router.push('/exceptions/new')">
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
    <el-table-column :label="$t('common.name')" prop="name" sortable="custom" :sort-orders="sortOrders" />
    <el-table-column :label="$t('policy.phase')" prop="phase" sortable="custom" :sort-orders="sortOrders" width="120">
      <template #default="{ row }"><PhaseTag :phase="row.phase" /></template>
    </el-table-column>
    <el-table-column :label="$t('exception.target')" prop="targetSummary" sortable="custom" :sort-orders="sortOrders" />
    <el-table-column :label="$t('exception.duration')" prop="duration" sortable="custom" :sort-orders="sortOrders" width="110" />
    <el-table-column :label="$t('exception.expiresAt')" prop="expiresAt" sortable="custom" :sort-orders="sortOrders" width="200">
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
import { deleteException, listExceptions, type ExceptionListItem } from '../api/exception'
import PageHeader from '../components/PageHeader.vue'
import PhaseTag from '../components/PhaseTag.vue'
import PaginationBar from '../components/PaginationBar.vue'
import { listStateParams, useRouteListState, type SortOrder } from '../composables/useRouteListState'
import { useLatestRequest } from '../composables/useLatestRequest'

const sortOrders = ['ascending', 'descending'] as const

const { t } = useI18n()
const items = ref<ExceptionListItem[]>([])
const total = ref(0)
const { loading, run: runLatest } = useLatestRequest()
const routeList = useRouteListState({
  defaultSortBy: 'name',
  defaultSortOrder: 'asc',
  allowedSort: ['name', 'phase', 'targetSummary', 'duration', 'expiresAt'],
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
      () => listExceptions(requestParams.value),
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

</script>
