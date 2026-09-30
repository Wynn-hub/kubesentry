<!-- web/src/views/PolicyListView.vue -->
<template>
  <PageHeader :title="$t('policy.title')" :subtitle="$t('policy.subtitle')">
    <el-button v-if="can('resource.write')" type="primary" @click="$router.push('/policies/new')">
      {{ $t('common.create') }}
    </el-button>
  </PageHeader>

  <div class="list-toolbar">
    <el-select
      v-model="sourceFilter"
      multiple
      collapse-tags
      clearable
      :placeholder="$t('policy.allSources')"
      style="width: 170px"
    >
      <el-option :label="$t('policy.builtin')" value="builtin" />
      <el-option :label="$t('policy.custom')" value="custom" />
    </el-select>
    <el-select
      v-model="phaseFilter"
      multiple
      collapse-tags
      clearable
      :placeholder="$t('policy.allPhases')"
      style="width: 170px"
    >
      <el-option v-for="p in POLICY_PHASES" :key="p" :label="$t(`phase.policy.${p}`)" :value="p" />
    </el-select>
    <el-select
      v-model="modeFilter"
      multiple
      collapse-tags
      clearable
      :placeholder="$t('policy.allModes')"
      style="width: 170px"
    >
      <el-option :label="$t('policy.enforce')" value="enforce" />
      <el-option :label="$t('policy.audit')" value="audit" />
    </el-select>
    <el-input v-model="keywordInput" :placeholder="$t('common.search')" style="width: 200px" clearable />
  </div>

  <el-table
    :key="`${listState.sortBy}:${listState.sortOrder}`"
    :data="items"
    :default-sort="tableSort"
    v-loading="loading"
    @sort-change="onSortChange"
  >
    <el-table-column :label="$t('common.name')" prop="name" sortable="custom" :sort-orders="sortOrders" min-width="180" show-overflow-tooltip>
      <template #default="{ row }">
        <router-link :to="`/policies/${row.name}`">{{ row.name }}</router-link>
      </template>
    </el-table-column>
    <el-table-column :label="$t('policy.source')" prop="source" sortable="custom" :sort-orders="sortOrders" width="100">
      <template #default="{ row }">
        <el-tag size="small" :type="row.source === 'builtin' ? 'info' : 'primary'">{{ $t(`policy.${row.source}`) }}</el-tag>
      </template>
    </el-table-column>
    <el-table-column :label="$t('policy.owner')" width="120"><template #default="{ row }">{{ row.ownerType === 'system' ? $t('policy.systemOwner') : $t('policy.tenantOwner') }}</template></el-table-column>
    <el-table-column :label="$t('policy.mode')" prop="enforcementMode" sortable="custom" :sort-orders="sortOrders" width="115">
      <template #default="{ row }"><ModeTag :mode="row.enforcementMode" /></template>
    </el-table-column>
    <el-table-column :label="$t('policy.phase')" prop="phase" sortable="custom" :sort-orders="sortOrders" width="100">
      <template #default="{ row }"><PhaseTag :phase="row.phase" kind="policy" /></template>
    </el-table-column>
    <el-table-column :label="$t('policy.createdAt')" prop="createdAt" sortable="custom" :sort-orders="sortOrders" width="155">
      <template #default="{ row }">
        {{ row.createdAt ? new Date(row.createdAt).toLocaleString() : '-' }}
      </template>
    </el-table-column>
    <el-table-column :label="$t('policy.updatedAt')" prop="updatedAt" sortable="custom" :sort-orders="sortOrders" width="155">
      <template #default="{ row }">
        {{ row.updatedAt ? new Date(row.updatedAt).toLocaleString() : '-' }}
      </template>
    </el-table-column>
    <el-table-column v-if="can('resource.write')" :label="$t('common.actions')" width="130" fixed="right">
      <template #default="{ row }">
        <el-button v-if="canEditPolicy(row)" link type="primary" @click="$router.push(`/policies/${row.name}/edit`)">
          {{ $t('common.edit') }}
        </el-button>
        <el-button v-if="canEditPolicy(row)" link type="danger" @click="onDelete(row)">{{ $t('common.delete') }}</el-button>
      </template>
    </el-table-column>
  </el-table>
  <PaginationBar
    :total="total"
    :page="listState.page"
    :page-size="listState.pageSize"
    @update:page="onPageChange"
    @update:page-size="onPageSizeChange"
  />
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useI18n } from 'vue-i18n'
import { ApiError } from '../api/http'
import { deletePolicy, listPolicies, POLICY_PHASES, type ListPoliciesQuery, type PolicyListItem } from '../api/policy'
import PageHeader from '../components/PageHeader.vue'
import ModeTag from '../components/ModeTag.vue'
import PhaseTag from '../components/PhaseTag.vue'
import PaginationBar from '../components/PaginationBar.vue'
import { listStateParams, useRouteListState, type SortOrder } from '../composables/useRouteListState'
import { useLatestRequest } from '../composables/useLatestRequest'
import { authState, can } from '../auth/session'

const KEYWORD_DEBOUNCE_MS = 300
const canEditPolicy = (policy: PolicyListItem) => can('platform.manage') || (can('resource.write') && policy.ownerType === 'tenant' && policy.tenantId === authState.me?.tenantId)
const sortOrders = ['ascending', 'descending'] as const

const { t } = useI18n()
const route = useRoute()
const items = ref<PolicyListItem[]>([])
const total = ref(0)
const { loading, run: runLatest } = useLatestRequest()
const routeList = useRouteListState({
  defaultSortBy: 'name',
  defaultSortOrder: 'asc',
  allowedSort: ['name', 'source', 'enforcementMode', 'phase', 'createdAt', 'updatedAt'],
})
const listState = routeList.state

function queryText(value: unknown): string {
  if (Array.isArray(value)) return queryText(value[0])
  return typeof value === 'string' ? value : ''
}

function queryValues(value: unknown): string[] {
  return queryText(value).split(',').map((part) => part.trim()).filter(Boolean)
}

function filterModel(key: 'source' | 'phase' | 'mode') {
  return computed<string[]>({
    get: () => queryValues(route.query[key]),
    set: (values) => {
      void routeList.replace({}, { resetPage: true, extra: { [key]: values.length ? values.join(',') : undefined } })
    },
  })
}

const sourceFilter = filterModel('source')
const phaseFilter = filterModel('phase')
const modeFilter = filterModel('mode')
const keywordInput = ref(queryText(route.query.keyword))

watch(
  () => queryText(route.query.keyword),
  (keyword) => {
    if (keywordInput.value !== keyword) keywordInput.value = keyword
  },
)

let keywordTimer: ReturnType<typeof setTimeout> | undefined
watch(keywordInput, (keyword) => {
  if (keyword === queryText(route.query.keyword)) return
  clearTimeout(keywordTimer)
  keywordTimer = setTimeout(() => {
    void routeList.replace({}, { resetPage: true, extra: { keyword: keyword || undefined } })
  }, KEYWORD_DEBOUNCE_MS)
})
onBeforeUnmount(() => clearTimeout(keywordTimer))

const requestParams = computed(() => {
  const params = listStateParams<ListPoliciesQuery>(listState.value)
  if (sourceFilter.value.length) params.source = sourceFilter.value as NonNullable<ListPoliciesQuery['source']>
  if (phaseFilter.value.length) params.phase = phaseFilter.value as NonNullable<ListPoliciesQuery['phase']>
  if (modeFilter.value.length) params.mode = modeFilter.value as NonNullable<ListPoliciesQuery['mode']>
  const keyword = queryText(route.query.keyword)
  if (keyword) params.keyword = keyword
  return params
})

const tableSort = computed(() => ({
  prop: listState.value.sortBy,
  order: listState.value.sortOrder === 'asc' ? 'ascending' : 'descending',
}))

async function load() {
  try {
    await runLatest(
      () => listPolicies(requestParams.value),
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

function onPageChange(page: number) {
  void routeList.replace({ page })
}

function onPageSizeChange(pageSize: number) {
  void routeList.replace({ pageSize })
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

</script>

<style scoped>
/* Wrap instead of squeezing the fixed-width filter controls on narrow screens. */
.list-toolbar {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
  margin-bottom: 12px;
}

.list-toolbar > * {
  flex-shrink: 0;
}
</style>
