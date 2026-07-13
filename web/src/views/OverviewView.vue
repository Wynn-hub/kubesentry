<!-- web/src/views/OverviewView.vue -->
<template>
  <PageHeader :title="$t('overview.title')" :subtitle="$t('overview.subtitle')">
    <span v-if="data" class="status-pill" :class="webhookPillClass">
      <span class="status-dot" />{{ webhookText }}
    </span>
  </PageHeader>

  <div v-if="data">
    <div class="stat-grid">
      <div class="stat-card" style="--accent: var(--steel-300)">
        <span class="stat-label">{{ $t('overview.cluster') }}</span>
        <span class="stat-value mono-num">1</span>
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
        <span class="stat-sub">{{
          $t('overview.readyOfTotal', { ready: data.policyPhases.Ready ?? 0, total: data.totals.policies })
        }}</span>
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
      <div class="stat-card" style="--accent: var(--warning)">
        <span class="stat-label">{{ $t('overview.nsCoverage') }}</span>
        <span class="stat-value mono-num">
          {{ data.namespaceCoverage.protected }}<span class="stat-denom">/{{ data.namespaceCoverage.total }}</span>
        </span>
      </div>
    </div>

    <div class="overview-grid">
      <el-card>
        <template #header>{{ $t('overview.groupHealth') }}</template>
        <div v-for="g in data.groups" :key="g.name" class="group-row">
          <router-link :to="`/policygroups/${g.name}`" class="group-name">
            {{ g.displayName || g.name }}
          </router-link>
          <span v-if="!g.enabled" class="status-pill is-info">{{ $t('overview.groupDisabled') }}</span>
          <PhaseTag v-else :phase="g.phase" />
          <span class="group-meta mono-num">
            {{ $t('overview.enforceN', { n: g.enforceCount }) }} ·
            {{ $t('overview.auditN', { n: g.auditCount }) }} ·
            {{ $t('overview.coverNs', { n: g.matchedNamespaces }) }}
          </span>
        </div>
      </el-card>

      <el-card>
        <template #header>{{ $t('overview.attention') }}</template>
        <template v-if="invalidPolicies.length || degradedGroups.length">
          <p v-for="p in invalidPolicies" :key="p.name" class="attention-row">
            <router-link :to="`/policies/${p.name}`">{{ p.name }}</router-link>
            <PhaseTag :phase="p.phase" />
          </p>
          <p v-for="g in degradedGroups" :key="g.name" class="attention-row">
            <router-link :to="`/policygroups/${g.name}`">{{ g.name }}</router-link>
            <PhaseTag :phase="g.phase" />
          </p>
        </template>
        <p v-else class="all-clear">
          <span class="status-pill is-success"><span class="status-dot" />{{ $t('overview.noIssues') }}</span>
        </p>
      </el-card>

      <el-card>
        <template #header>{{ $t('overview.recentChanges') }}</template>
        <template v-if="data.recentVersions.length">
          <p v-for="v in data.recentVersions" :key="`${v.policy}-${v.version}`" class="change-row">
            <router-link :to="`/policies/${v.policy}`">{{ v.policy }}</router-link>
            <span class="status-pill is-scan">{{ $t('overview.versionTag', { n: v.version }) }}</span>
            <span class="change-time mono-num">{{ relTime(v.createdAt, String($i18n.locale)) }}</span>
          </p>
        </template>
        <p v-else class="all-clear">{{ $t('overview.noChanges') }}</p>
      </el-card>

      <el-card class="events-soon">
        <template #header>{{ $t('overview.eventsTrend') }}</template>
        <div class="events-soon-body">
          <svg width="32" height="32" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
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
import { computed, onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { useI18n } from 'vue-i18n'
import { getSummary, type SummaryData } from '../api/summary'
import { listPolicies, type PolicyListItem } from '../api/policy'
import { relTime } from '../lib/relTime'
import PageHeader from '../components/PageHeader.vue'
import PhaseTag from '../components/PhaseTag.vue'

const { t } = useI18n()
const data = ref<SummaryData>()
const invalidPolicies = ref<PolicyListItem[]>([])

const degradedGroups = computed(() => (data.value?.groups ?? []).filter((g) => g.enabled && g.phase === 'Degraded'))

const webhookPillClass = computed(() => {
  if (!data.value) return 'is-info'
  const wh = data.value.webhook
  if (wh.configured && wh.caBundleSet) return 'is-success'
  if (wh.configured) return 'is-warning'
  return 'is-danger'
})

const webhookText = computed(() => {
  if (!data.value) return ''
  const wh = data.value.webhook
  if (wh.configured && wh.caBundleSet) return t('overview.webhookOK', { n: wh.ruleCount })
  if (wh.configured) return t('overview.webhookNoCA')
  return t('overview.webhookMissing')
})

onMounted(async () => {
  try {
    data.value = await getSummary()
    invalidPolicies.value = await listPolicies({ phase: 'Invalid' })
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
})
</script>

<style scoped>
.stat-sub {
  font-family: var(--font-mono);
  font-size: 11px;
  color: var(--steel-400);
}
.stat-denom {
  font-size: 18px;
  font-weight: 500;
  color: var(--steel-400);
}
.group-row,
.attention-row,
.change-row {
  display: flex;
  align-items: center;
  gap: 10px;
  margin: 0 0 10px;
}
.group-row:last-child,
.attention-row:last-child,
.change-row:last-child {
  margin-bottom: 0;
}
.group-name {
  min-width: 110px;
  font-weight: 600;
}
.group-meta {
  font-size: 11.5px;
  color: var(--steel-300);
  margin-left: auto;
}
.change-time {
  margin-left: auto;
  font-size: 11.5px;
  color: var(--steel-400);
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
