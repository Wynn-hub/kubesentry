<template>
  <div v-if="d">
    <PageHeader show-back :title="d.spec.displayName || d.name">
      <el-tag size="small" :type="d.source === 'builtin' ? 'info' : 'primary'">{{ d.source }}</el-tag>
      <PhaseTag :phase="d.status.phase" kind="group" />
      <el-button @click="$router.push(`/policygroups/${d.name}/edit`)">{{ $t('common.edit') }}</el-button>
    </PageHeader>

    <el-descriptions :column="2" border style="margin-bottom: 16px">
      <el-descriptions-item :label="$t('common.name')">{{ d.name }}</el-descriptions-item>
      <el-descriptions-item :label="$t('group.enabled')">{{ d.spec.enabled }}</el-descriptions-item>
      <el-descriptions-item :label="$t('common.description')" :span="2">{{ d.spec.description }}</el-descriptions-item>
      <el-descriptions-item :label="$t('group.nsSelector')" :span="2">
        <code>{{ JSON.stringify(d.spec.namespaceSelector ?? {}) }}</code>
      </el-descriptions-item>
    </el-descriptions>

    <el-alert
      v-if="d.status.issues?.length"
      type="error"
      :title="$t('group.issues')"
      :description="d.status.issues.join('; ')"
      :closable="false"
      show-icon
      style="margin-bottom: 16px"
    />

    <h3>{{ $t('group.resolved') }} ({{ d.status.resolvedCount ?? 0 }})</h3>
    <el-table :data="membersPage.items">
      <el-table-column :label="$t('common.name')">
        <template #default="{ row }">
          <router-link :to="`/policies/${row.name}`">{{ row.name }}</router-link>
        </template>
      </el-table-column>
      <el-table-column :label="$t('group.memberSource')" prop="source" width="120" />
      <el-table-column :label="$t('group.effectiveMode')" width="120">
        <template #default="{ row }"><ModeTag :mode="row.enforcementMode" /></template>
      </el-table-column>
    </el-table>
    <PaginationBar
      :total="membersPage.total"
      :page="membersState.page"
      :page-size="membersState.pageSize"
      @update:page="(page) => membersRouteList.replace({ page })"
      @update:page-size="(pageSize) => membersRouteList.replace({ pageSize })"
    />

    <div v-if="excludedNames.length" style="margin-top: 16px">
      <h3>{{ $t('group.excluded') }} ({{ excludedNames.length }})</h3>
      <p style="color: var(--el-text-color-secondary); margin: 4px 0 12px">
        {{ $t('group.excludedHint') }}
      </p>
      <el-table :data="excludedPage.items">
        <el-table-column :label="$t('common.name')">
          <template #default="{ row }">
            <router-link :to="`/policies/${row.name}`">{{ row.name }}</router-link>
          </template>
        </el-table-column>
      </el-table>
      <PaginationBar
        :total="excludedPage.total"
        :page="excludedState.page"
        :page-size="excludedState.pageSize"
        @update:page="(page) => excludedRouteList.replace({ page })"
        @update:page-size="(pageSize) => excludedRouteList.replace({ pageSize })"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { ElMessage } from 'element-plus'
import { getGroup, type GroupDetail } from '../api/group'
import ModeTag from '../components/ModeTag.vue'
import PhaseTag from '../components/PhaseTag.vue'
import PageHeader from '../components/PageHeader.vue'
import PaginationBar from '../components/PaginationBar.vue'
import { useClientPagination } from '../composables/useClientPagination'
import { useRouteListState } from '../composables/useRouteListState'

const route = useRoute()
const d = ref<GroupDetail>()
const excludedNames = computed(() => d.value?.spec.policies?.exclude ?? [])
const membersRouteList = useRouteListState({
  prefix: 'members',
  defaultSortBy: 'name',
  defaultSortOrder: 'asc',
  allowedSort: ['name'],
})
const membersState = membersRouteList.state
const membersPage = useClientPagination(
  () => d.value?.status.resolvedPolicies ?? [],
  membersState,
  (a, b) => a.name.localeCompare(b.name),
)
const excludedRouteList = useRouteListState({
  prefix: 'excluded',
  defaultSortBy: 'name',
  defaultSortOrder: 'asc',
  allowedSort: ['name'],
})
const excludedState = excludedRouteList.state
const excludedItems = computed(() => excludedNames.value.map((name) => ({ name })))
const excludedPage = useClientPagination(
  excludedItems,
  excludedState,
  (a, b) => a.name.localeCompare(b.name),
)

function correctPage(total: number, state: { page: number; pageSize: number }, replace: (patch: { page: number }) => Promise<unknown>) {
  const lastPage = Math.max(1, Math.ceil(total / state.pageSize))
  if (state.page > lastPage) void replace({ page: lastPage })
}

onMounted(async () => {
  try {
    d.value = await getGroup(String(route.params.name))
    correctPage(d.value.status.resolvedPolicies?.length ?? 0, membersState.value, membersRouteList.replace)
    correctPage(excludedNames.value.length, excludedState.value, excludedRouteList.replace)
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
})
</script>
