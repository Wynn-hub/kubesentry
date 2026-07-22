<!-- web/src/views/OverviewView.vue -->
<template>
  <PageHeader :title="$t('overview.title')" :subtitle="$t('overview.subtitle')" />

  <div v-if="data">
    <div class="stat-grid">
      <div
        class="stat-card"
        style="--accent: var(--steel-300)"
        role="button"
        tabindex="0"
        @click="$router.push('/clusters')"
        @keydown.enter="$router.push('/clusters')"
      >
        <span class="stat-label">{{ $t('overview.cluster') }}</span>
        <span class="stat-value mono-num">{{ data.totals.clusters }}</span>
        <span class="cluster-phase-summary">
          <span v-if="data.clusterPhases.online">
            {{ $t('overview.clusterOnline', { n: data.clusterPhases.online }) }}
          </span>
          <span v-if="data.clusterPhases.offline">
            {{ $t('overview.clusterOffline', { n: data.clusterPhases.offline }) }}
          </span>
          <span v-if="data.clusterPhases.registering">
            {{ $t('overview.clusterRegistering', { n: data.clusterPhases.registering }) }}
          </span>
        </span>
      </div>
      <div
        class="stat-card"
        style="--accent: var(--accent-scan)"
        role="button"
        tabindex="0"
        @click="$router.push('/policies')"
        @keydown.enter="$router.push('/policies')"
      >
        <span class="stat-label">{{ $t('overview.policies') }}</span>
        <span class="stat-value mono-num">{{ data.totals.policies }}</span>
      </div>
      <div
        class="stat-card"
        style="--accent: var(--success)"
        role="button"
        tabindex="0"
        @click="$router.push('/policygroups')"
        @keydown.enter="$router.push('/policygroups')"
      >
        <span class="stat-label">{{ $t('overview.groups') }}</span>
        <span class="stat-value mono-num">{{ data.totals.policygroups }}</span>
      </div>
      <div
        class="stat-card"
        style="--accent: var(--accent-guard)"
        role="button"
        tabindex="0"
        @click="$router.push('/exceptions')"
        @keydown.enter="$router.push('/exceptions')"
      >
        <span class="stat-label">{{ $t('overview.exceptions') }}</span>
        <span class="stat-value mono-num">{{ data.totals.exceptions }}</span>
      </div>
    </div>

    <div class="overview-grid">
      <el-card>
        <template #header>{{ $t('overview.groupHealth') }}</template>
        <div v-for="g in groupsPage.items" :key="g.name" class="group-row">
          <router-link :to="`/policygroups/${g.name}`" class="group-name">
            {{ g.displayName || g.name }}
          </router-link>
          <span v-if="!g.enabled" class="status-pill is-info">{{ $t('overview.groupDisabled') }}</span>
          <span class="group-meta mono-num">
            {{ $t('overview.policies') }} ×{{ g.resolvedCount }} ·
            {{ $t('overview.enforceN', { n: g.enforceCount }) }} ·
            {{ $t('overview.auditN', { n: g.auditCount }) }}
          </span>
        </div>
        <PaginationBar
          :total="groupsPage.total"
          :page="groupsState.page"
          :page-size="groupsState.pageSize"
          @update:page="(page) => groupsRouteList.replace({ page })"
          @update:page-size="(pageSize) => groupsRouteList.replace({ pageSize })"
        />
      </el-card>

      <el-card>
        <template #header>{{ $t('overview.attention') }}</template>
        <template v-if="attentionPage.total">
          <p v-for="p in attentionPage.items" :key="p.name" class="attention-row">
            <router-link :to="`/policies/${p.name}`">{{ p.name }}</router-link>
            <PhaseTag :phase="p.phase" />
          </p>
        </template>
        <p v-else class="all-clear">
          <span class="status-pill is-success"><span class="status-dot" />{{ $t('overview.noIssues') }}</span>
        </p>
        <PaginationBar
          v-if="attentionPage.total"
          :total="attentionPage.total"
          :page="attentionState.page"
          :page-size="attentionState.pageSize"
          @update:page="(page) => attentionRouteList.replace({ page })"
          @update:page-size="(pageSize) => attentionRouteList.replace({ pageSize })"
        />
      </el-card>

      <el-card class="events-soon">
        <template #header>{{ $t('overview.eventsTrend') }}</template>
        <div class="events-soon-body">
          <svg aria-hidden="true" width="32" height="32" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
            <path d="M4 17l4-8 4 5 3-3 5 6" stroke-linecap="round" stroke-linejoin="round" />
            <path d="M3 21h18" stroke-linecap="round" />
          </svg>
          <span class="status-pill is-warning">{{ $t('nav.soon') }}</span>
          <p>{{ $t('overview.eventsSoon') }}</p>
        </div>
      </el-card>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { getSummary, type SummaryData } from '../api/summary'
import { listPolicies, type PolicyListItem } from '../api/policy'
import { collectAllPages } from '../api/pagination'
import PageHeader from '../components/PageHeader.vue'
import PhaseTag from '../components/PhaseTag.vue'
import PaginationBar from '../components/PaginationBar.vue'
import { useClientPagination } from '../composables/useClientPagination'
import { useRouteListState } from '../composables/useRouteListState'

const data = ref<SummaryData>()
const invalidPolicies = ref<PolicyListItem[]>([])
const groupsRouteList = useRouteListState({
  prefix: 'overviewGroups',
  defaultSortBy: 'name',
  defaultSortOrder: 'asc',
  allowedSort: ['name'],
})
const groupsState = groupsRouteList.state
const groupsPage = useClientPagination(
  () => data.value?.groups ?? [],
  groupsState,
  (a, b) => a.name.localeCompare(b.name),
)
const attentionRouteList = useRouteListState({
  prefix: 'overviewAttention',
  defaultSortBy: 'name',
  defaultSortOrder: 'asc',
  allowedSort: ['name'],
})
const attentionState = attentionRouteList.state
const attentionPage = useClientPagination(
  invalidPolicies,
  attentionState,
  (a, b) => a.name.localeCompare(b.name),
)

function correctPage(total: number, page: number, pageSize: number, replace: (patch: { page: number }) => Promise<unknown>) {
  const lastPage = Math.max(1, Math.ceil(total / pageSize))
  if (page > lastPage) void replace({ page: lastPage })
}

onMounted(async () => {
  try {
    const [summary, invalid] = await Promise.all([
      getSummary(),
      collectAllPages((params) => listPolicies({ ...params, phase: 'Invalid' })),
    ])
    data.value = summary
    invalidPolicies.value = invalid
    correctPage(summary.groups.length, groupsState.value.page, groupsState.value.pageSize, groupsRouteList.replace)
    correctPage(invalid.length, attentionState.value.page, attentionState.value.pageSize, attentionRouteList.replace)
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
})
</script>

<style scoped>
.group-row,
.attention-row {
  display: flex;
  align-items: center;
  gap: 10px;
  margin: 0 0 10px;
}
.group-row:last-child,
.attention-row:last-child {
  margin-bottom: 0;
}
.group-name {
  min-width: 110px;
  font-weight: 600;
}
.cluster-phase-summary {
  display: flex;
  flex-wrap: wrap;
  gap: 5px;
  min-height: 17px;
  color: var(--steel-300);
  font-size: 11.5px;
}
.cluster-phase-summary span + span::before {
  content: '·';
  margin-right: 5px;
}
.group-meta {
  font-size: 11.5px;
  color: var(--steel-300);
  margin-left: auto;
}
.all-clear {
  margin: 0;
  color: var(--steel-300);
}
.events-soon-body {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 10px;
  padding: 18px 0;
  color: var(--steel-400);
  text-align: center;
}
.events-soon-body p {
  margin: 0;
  font-size: 12.5px;
  color: var(--steel-300);
}
</style>
