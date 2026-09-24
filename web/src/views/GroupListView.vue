<template>
  <PageHeader :title="$t('group.title')" :subtitle="$t('group.subtitle')">
    <el-button type="primary" @click="$router.push('/policygroups/new')">
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
    <el-table-column :label="$t('common.name')" prop="name" sortable="custom" :sort-orders="sortOrders">
      <template #default="{ row }">
        <router-link :to="`/policygroups/${row.name}`">{{ row.name }}</router-link>
      </template>
    </el-table-column>
    <el-table-column :label="$t('group.displayName')" prop="displayName" sortable="custom" :sort-orders="sortOrders" />
    <el-table-column :label="$t('group.enabled')" prop="enabled" sortable="custom" :sort-orders="sortOrders" width="110">
      <template #default="{ row }">
        <el-switch
          :model-value="row.enabled"
          :loading="toggling === row.name"
          @change="onToggleEnabled(row)"
        />
      </template>
    </el-table-column>
    <el-table-column :label="$t('policy.phase')" prop="phase" sortable="custom" :sort-orders="sortOrders" width="120">
      <template #default="{ row }"><PhaseTag :phase="row.phase" /></template>
    </el-table-column>
    <el-table-column :label="$t('group.resolvedCount')" prop="resolvedCount" sortable="custom" :sort-orders="sortOrders" width="110" />
    <el-table-column :label="$t('common.actions')" width="160">
      <template #default="{ row }">
        <el-button link type="primary" @click="$router.push(`/policygroups/${row.name}/edit`)">
          {{ $t('common.edit') }}
        </el-button>
        <el-button v-if="row.name !== 'default'" link type="danger" @click="onDelete(row)">{{ $t('common.delete') }}</el-button>
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
import { deleteGroup, listGroups, setGroupEnabled, type GroupListItem, type ListGroupsQuery } from '../api/group'
import PageHeader from '../components/PageHeader.vue'
import PhaseTag from '../components/PhaseTag.vue'
import PaginationBar from '../components/PaginationBar.vue'
import { listStateParams, useRouteListState, type SortOrder } from '../composables/useRouteListState'
import { useLatestRequest } from '../composables/useLatestRequest'

const sortOrders = ['ascending', 'descending'] as const

const { t } = useI18n()
const items = ref<GroupListItem[]>([])
const total = ref(0)
const { loading, run: runLatest } = useLatestRequest()
const toggling = ref('')
const routeList = useRouteListState({
  defaultSortBy: 'name',
  defaultSortOrder: 'asc',
  allowedSort: ['name', 'displayName', 'enabled', 'phase', 'resolvedCount'],
})
const listState = routeList.state
const tableSort = computed(() => ({
  prop: listState.value.sortBy,
  order: listState.value.sortOrder === 'asc' ? 'ascending' : 'descending',
}))
const requestParams = computed(() => listStateParams<ListGroupsQuery>(listState.value))

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
  try {
    await runLatest(
      () => listGroups(requestParams.value),
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

</script>
