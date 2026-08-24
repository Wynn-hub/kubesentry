<template>
  <section v-loading="loading">
    <PageHeader :title="$t('platformObservability.title')" :subtitle="$t('platformObservability.subtitle')">
      <el-button @click="openSettings"><el-icon><Setting /></el-icon>{{ $t('platformObservability.settings') }}</el-button>
      <el-button type="primary" plain @click="refresh"><el-icon><Refresh /></el-icon>{{ $t('common.refresh') }}</el-button>
    </PageHeader>

    <el-empty
      v-if="overview && !overview.enabled"
      class="disabled-state"
      :description="$t('platformObservability.disabledDescription')"
    >
      <el-button type="primary" @click="openSettings">{{ $t('platformObservability.enableNow') }}</el-button>
    </el-empty>

    <template v-else-if="overview">
      <!-- Closable: an operator who has read the banner and decided the cause is
           acceptable should not have to look at it for the rest of the session.
           It returns whenever the health level actually changes. -->
      <el-alert
        v-if="bannerVisible"
        :type="overview.platformHealth === 'unavailable' ? 'error' : 'warning'"
        show-icon
        class="platform-alert"
        :title="$t(`platformObservability.banner_${overview.platformHealth}`)"
        :description="bannerDescription"
        @close="dismissBanner"
      />

      <div class="section-heading">
        <div><h2>{{ $t('platformObservability.gatewayPlane') }}</h2><p>{{ $t('platformObservability.gatewayPlaneHint') }}</p></div>
        <span class="live"><span />{{ $t('platformObservability.liveRefresh') }}</span>
      </div>
      <div class="component-grid">
        <PlatformComponentCard
          v-for="component in overview.gatewayPlane"
          :key="component.component"
          :component="component"
          @select="instance => selectInstance(component.component, undefined, instance)"
          @history="openHistory(component.component)"
        />
      </div>

      <div class="section-heading">
        <div><h2>{{ $t('platformObservability.memberPlane') }}</h2><p>{{ $t('platformObservability.memberPlaneHint') }}</p></div>
        <el-select v-model="healthFilter" clearable :placeholder="$t('platformObservability.allHealth')" style="width:180px">
          <el-option value="ok" :label="$t('platformObservability.health_ok')" />
          <el-option value="degraded" :label="$t('platformObservability.health_degraded')" />
          <el-option value="unavailable" :label="$t('platformObservability.health_unavailable')" />
        </el-select>
      </div>

      <!-- Same card component as the gateway plane: member components are the
           same kind of thing and were previously rendered as table pills, which
           made the two planes look unrelated. -->
      <article v-for="cluster in overview.memberPlane.clusters" :key="cluster.name" class="cluster-block">
        <header class="cluster-heading">
          <div>
            <router-link :to="{ name: 'cluster-detail', params: { name: cluster.name } }">{{ cluster.name }}</router-link>
            <span class="phase">{{ cluster.phase }}</span>
          </div>
          <el-tag :type="healthTag(cluster.health)" size="small" effect="dark">
            {{ $t(`platformObservability.health_${cluster.health}`) }}
          </el-tag>
        </header>
        <div class="component-grid member">
          <PlatformComponentCard
            v-for="component in cluster.components"
            :key="component.component"
            :component="component"
            @select="instance => selectInstance(component.component, cluster.name, instance)"
            @history="openHistory(component.component, cluster.name)"
          />
        </div>
      </article>
      <el-empty
        v-if="!overview.memberPlane.clusters.length"
        :description="$t('platformObservability.noMemberData')"
        :image-size="60"
      />
      <PaginationBar
        :total="overview.memberPlane.total"
        :page="page"
        :page-size="pageSize"
        @update:page="value => { page = value; load() }"
        @update:page-size="value => { pageSize = value; page = 1; load() }"
      />
    </template>

    <PlatformInstanceDrawer v-model="instanceOpen" :target="selectedInstance" />

    <el-dialog v-model="historyOpen" :title="$t('platformObservability.leadershipHistory')" width="min(860px, 94vw)">
      <el-table :data="leadership?.items ?? []" v-loading="historyLoading">
        <el-table-column prop="changedAt" :label="$t('platformObservability.changed')" width="180">
          <template #default="{ row }">{{ new Date(row.changedAt).toLocaleString() }}</template>
        </el-table-column>
        <el-table-column prop="instance" :label="$t('platformObservability.instance')" min-width="180" />
        <el-table-column :label="$t('platformObservability.roleChange')" min-width="150">
          <template #default="{ row }">{{ role(row.roleFrom) }} → {{ role(row.roleTo) }}</template>
        </el-table-column>
        <el-table-column :label="$t('platformObservability.detectedBy')" min-width="150">
          <template #default="{ row }">{{ $t(`platformObservability.detected_${row.detectedBy}`) }}</template>
        </el-table-column>
      </el-table>
      <PaginationBar
        :total="leadership?.total ?? 0"
        :page="historyPage"
        :page-size="10"
        @update:page="value => { historyPage = value; loadHistory() }"
        @update:page-size="() => undefined"
      />
    </el-dialog>

    <el-drawer v-model="settingsOpen" :title="$t('platformObservability.settings')" size="480px">
      <el-form label-position="top" v-loading="settingsLoading">
        <el-form-item :label="$t('platformObservability.enabledLabel')">
          <el-switch
            v-model="form.enabled"
            :active-text="$t('common.enabled')"
            :inactive-text="$t('common.disabled')"
            :before-change="confirmEnable"
          />
        </el-form-item>
        <el-form-item :label="$t('platformObservability.retentionLabel')">
          <el-input-number v-model="form.retentionDays" :min="1" :max="settings?.maxRetentionDays ?? 7" />
        </el-form-item>
        <p class="hint">{{ $t('platformObservability.retentionHint', { max: settings?.maxRetentionDays ?? 7 }) }}</p>
        <el-button type="primary" class="save" :loading="settingsSaving" @click="saveSettings">
          {{ $t('common.save') }}
        </el-button>
      </el-form>
    </el-drawer>
  </section>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Refresh, Setting } from '@element-plus/icons-vue'
import { useI18n } from 'vue-i18n'
import PageHeader from '../components/PageHeader.vue'
import PaginationBar from '../components/PaginationBar.vue'
import PlatformComponentCard from '../components/PlatformComponentCard.vue'
import PlatformInstanceDrawer, { type PlatformInstanceTarget } from '../components/PlatformInstanceDrawer.vue'
import {
  getPlatformOverview,
  getPlatformSettings,
  listPlatformLeadershipEvents,
  updatePlatformSettings,
  type PlatformInstance,
  type PlatformComponent,
  type PlatformLeadershipPage,
  type PlatformOverview,
  type PlatformSettings,
} from '../api/platform'
import { createObservationPoller } from './observationPolling'

const { t } = useI18n()
const overview = ref<PlatformOverview>()
const loading = ref(true)
const page = ref(1)
const pageSize = ref(20)
const healthFilter = ref<'' | Extract<PlatformOverview['platformHealth'], 'ok' | 'degraded' | 'unavailable'>>('')
const instanceOpen = ref(false)
const selectedInstance = ref<PlatformInstanceTarget>()
const historyOpen = ref(false)
const historyLoading = ref(false)
const historyPage = ref(1)
const historyComponent = ref('')
const historyCluster = ref<string>()
const leadership = ref<PlatformLeadershipPage>()
const settingsOpen = ref(false)
const settingsLoading = ref(false)
const settingsSaving = ref(false)
const settings = ref<PlatformSettings>()
const form = ref({ enabled: false, retentionDays: 1 })
const dismissedFor = ref('')

const bannerVisible = computed(() =>
  !!overview.value?.enabled &&
  overview.value.platformHealth !== 'ok' &&
  dismissedFor.value !== overview.value.platformHealth)
const bannerDescription = computed(() => {
  // Only genuinely faulted components belong here. A pending component has not
  // reported its first sample yet, which is what every component looks like for
  // the first minute after the switch is turned on.
  const affected = (overview.value?.gatewayPlane ?? [])
    .filter(component => component.health === 'degraded' || component.health === 'unavailable')
    .map(component => t(`platformObservability.component_${component.component.replace('-', '_')}`))
  return affected.length
    ? t('platformObservability.bannerAdvice', { components: affected.join('、') })
    : t('platformObservability.bannerAdviceGeneric')
})
function dismissBanner() {
  dismissedFor.value = overview.value?.platformHealth ?? ''
}

async function load() {
  try {
    overview.value = await getPlatformOverview({
      page: page.value,
      pageSize: pageSize.value as 10 | 20 | 50 | 100,
      ...(healthFilter.value ? { health: healthFilter.value } : {}),
    })
  } catch (error) {
    ElMessage.error((error as Error).message)
  } finally {
    loading.value = false
  }
}
const refresh = () => load()
const poller = createObservationPoller([{ interval: 10_000, run: () => { void load() } }])
function visibility() {
  if (document.hidden) poller.stop()
  else {
    void load()
    poller.start(false)
  }
}
watch(healthFilter, () => {
  page.value = 1
  void load()
})

async function openSettings() {
  settingsOpen.value = true
  settingsLoading.value = true
  try {
    settings.value = await getPlatformSettings()
    form.value = { enabled: settings.value.enabled, retentionDays: settings.value.retentionDays }
  } catch (error) {
    ElMessage.error((error as Error).message)
  } finally {
    settingsLoading.value = false
  }
}
// Turning this on means every component writes a sample every minute and the
// console aggregates them on each poll. Make that explicit before accepting it.
async function confirmEnable() {
  if (form.value.enabled) return true
  try {
    await ElMessageBox.confirm(
      t('platformObservability.enableWarning'),
      t('platformObservability.enableWarningTitle'),
      { type: 'warning', confirmButtonText: t('common.confirm'), cancelButtonText: t('common.cancel') },
    )
    return true
  } catch {
    return false
  }
}
async function saveSettings() {
  settingsSaving.value = true
  try {
    settings.value = await updatePlatformSettings(form.value)
    form.value = { enabled: settings.value.enabled, retentionDays: settings.value.retentionDays }
    ElMessage.success(t('common.saved'))
    settingsOpen.value = false
    await load()
  } catch (error) {
    ElMessage.error((error as Error).message)
  } finally {
    settingsSaving.value = false
  }
}

function selectInstance(component: PlatformComponent['component'], cluster: string | undefined, instance: PlatformInstance) {
  selectedInstance.value = { component, cluster, instance }
  instanceOpen.value = true
}
function openHistory(component: PlatformComponent['component'], cluster?: string) {
  historyComponent.value = component
  historyCluster.value = cluster
  historyPage.value = 1
  historyOpen.value = true
  void loadHistory()
}
async function loadHistory() {
  historyLoading.value = true
  try {
    const to = new Date()
    leadership.value = await listPlatformLeadershipEvents({
      page: historyPage.value,
      pageSize: 10,
      component: historyComponent.value as PlatformComponent['component'],
      cluster: historyCluster.value,
      from: new Date(to.getTime() - 30 * 864e5).toISOString(),
      to: to.toISOString(),
    })
  } catch (error) {
    ElMessage.error((error as Error).message)
  } finally {
    historyLoading.value = false
  }
}
const healthTag = (health: string) => health === 'ok' ? 'success' : health === 'degraded' ? 'warning' : 'info'
const role = (value: string) => value === 'leader'
  ? t('platformObservability.leader')
  : value === 'standby' ? t('platformObservability.standby') : '—'

onMounted(async () => {
  await load()
  document.addEventListener('visibilitychange', visibility)
  poller.start(document.hidden)
})
onBeforeUnmount(() => {
  poller.stop()
  document.removeEventListener('visibilitychange', visibility)
})
</script>

<style scoped>
.platform-alert{margin-bottom:16px}
.disabled-state{padding:60px 0}
.section-heading{display:flex;align-items:flex-end;justify-content:space-between;gap:16px;margin:18px 0 12px}
.section-heading h2{margin:0 0 4px;color:var(--paper-050);font-size:16px}.section-heading p{margin:0;color:var(--steel-400);font-size:11px}
.live{display:flex;align-items:center;gap:7px;color:var(--steel-400);font:10px var(--font-mono)}
.live span{width:7px;height:7px;border-radius:50%;background:var(--accent-scan);box-shadow:0 0 0 4px rgba(51,214,192,.09)}
.component-grid{display:grid;grid-template-columns:repeat(4,minmax(0,1fr));gap:12px}
.component-grid.member{grid-template-columns:repeat(2,minmax(0,1fr))}
.cluster-block{margin-bottom:14px;padding:14px;border:1px solid var(--ink-600);border-radius:10px;background:rgba(18,27,35,.94)}
.cluster-heading{display:flex;align-items:center;justify-content:space-between;gap:12px;margin-bottom:12px}
.cluster-heading>div{display:flex;align-items:center;gap:10px;min-width:0}
.cluster-heading a{color:var(--paper-050);font:13px var(--font-mono);text-decoration:none}
.cluster-heading a:hover{color:var(--accent-scan)}
.phase{color:var(--steel-400);font:10px var(--font-mono)}
.save{margin-top:18px;width:100%}.hint{margin:-6px 0 0;color:var(--steel-300);font-size:12px;line-height:1.7}
@media(max-width:1200px){.component-grid{grid-template-columns:repeat(2,minmax(0,1fr))}}
@media(max-width:700px){.component-grid,.component-grid.member{grid-template-columns:1fr}.section-heading{align-items:flex-start;flex-direction:column}}
</style>
