<template>
  <section v-loading="loading">
    <PageHeader :title="$t('platformObservability.title')" :subtitle="$t('platformObservability.subtitle')">
      <el-button @click="settingsOpen = true"><el-icon><Setting /></el-icon>{{ $t('platformObservability.settings') }}</el-button>
      <el-button type="primary" plain @click="refresh"><el-icon><Refresh /></el-icon>{{ $t('common.refresh') }}</el-button>
    </PageHeader>

    <el-alert
      v-if="overview && overview.platformHealth !== 'ok'"
      :type="overview.platformHealth === 'unavailable' ? 'error' : 'warning'"
      :closable="false"
      show-icon
      class="platform-alert"
      :title="$t(`platformObservability.banner_${overview.platformHealth}`)"
    />

    <div class="section-heading">
      <div><h2>{{ $t('platformObservability.gatewayPlane') }}</h2><p>{{ $t('platformObservability.gatewayPlaneHint') }}</p></div>
      <span class="live"><span />{{ $t('platformObservability.liveRefresh') }}</span>
    </div>
    <div class="component-grid">
      <PlatformComponentCard
        v-for="component in overview?.gatewayPlane ?? []"
        :key="component.component"
        :component="component"
        @select="instance => selectInstance(component.component, undefined, instance)"
        @history="openHistory(component.component)"
      />
    </div>

    <div class="member-panel">
      <div class="section-heading">
        <div><h2>{{ $t('platformObservability.memberPlane') }}</h2><p>{{ $t('platformObservability.memberPlaneHint') }}</p></div>
        <el-select v-model="healthFilter" clearable :placeholder="$t('platformObservability.allHealth')" style="width:180px">
          <el-option value="ok" :label="$t('platformObservability.health_ok')" />
          <el-option value="degraded" :label="$t('platformObservability.health_degraded')" />
          <el-option value="unavailable" :label="$t('platformObservability.health_unavailable')" />
        </el-select>
      </div>
      <el-table :data="overview?.memberPlane.clusters ?? []" row-key="name">
        <el-table-column prop="name" :label="$t('cluster.name')" min-width="160">
          <template #default="{ row }">
            <router-link :to="{ name: 'cluster-detail', params: { name: row.name } }">{{ row.name }}</router-link>
          </template>
        </el-table-column>
        <el-table-column :label="$t('platformObservability.component_webhook')" min-width="230">
          <template #default="{ row }">
            <InstancePills
              :component="findComponent(row, 'webhook')"
              @select="instance => selectInstance('webhook', row.name, instance)"
            />
          </template>
        </el-table-column>
        <el-table-column :label="$t('platformObservability.component_operator')" min-width="260">
          <template #default="{ row }">
            <InstancePills
              :component="findComponent(row, 'operator')"
              @select="instance => selectInstance('operator', row.name, instance)"
              @history="openHistory('operator', row.name)"
            />
          </template>
        </el-table-column>
        <el-table-column :label="$t('platformObservability.health')" width="130">
          <template #default="{ row }">
            <el-tag :type="healthTag(row.health)" effect="dark">{{ $t(`platformObservability.health_${row.health}`) }}</el-tag>
          </template>
        </el-table-column>
      </el-table>
      <PaginationBar
        :total="overview?.memberPlane.total ?? 0"
        :page="page"
        :page-size="pageSize"
        @update:page="value => { page = value; load() }"
        @update:page-size="value => { pageSize = value; page = 1; load() }"
      />
    </div>

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
        <el-table-column prop="detectedBy" :label="$t('platformObservability.detectedBy')" min-width="130" />
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
        <el-form-item :label="$t('platformObservability.admissionPipeline')">
          <el-switch
            v-model="settings.admissionPipelineEnabled"
            :active-text="$t('common.enabled')"
            :inactive-text="$t('common.disabled')"
          />
        </el-form-item>
        <el-alert type="info" :closable="false" :title="$t('platformObservability.admissionPipelineHint')" />
        <el-button type="primary" class="save" :loading="settingsSaving" @click="saveSettings">{{ $t('common.save') }}</el-button>
      </el-form>
    </el-drawer>
  </section>
</template>

<script setup lang="ts">
import { defineComponent, h, onBeforeUnmount, onMounted, ref, watch, type PropType } from 'vue'
import { ElButton, ElMessage } from 'element-plus'
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
  type PlatformComponent,
  type PlatformInstance,
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
const healthFilter = ref('')
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
const settings = ref<PlatformSettings>({ admissionPipelineEnabled: false })

const InstancePills = defineComponent({
  props: { component: Object as PropType<PlatformComponent> },
  emits: ['select', 'history'],
  setup(props, { emit }) {
    return () => {
      if (!props.component?.instances.length) return h('span', { class: 'muted' }, '—')
      const pills = props.component.instances.map(instance => h(ElButton, {
        key: instance.instance,
        size: 'small',
        plain: true,
        type: instance.health === 'ok' ? 'success' : instance.health === 'degraded' ? 'warning' : 'info',
        onClick: () => emit('select', instance),
      }, () => `${instance.role === 'leader' ? t('platformObservability.leader') : instance.role === 'standby' ? t('platformObservability.standby') : ''} ${instance.instance}`.trim()))
      if (props.component.hasLeadership) {
        pills.push(h(ElButton, { key: 'history', link: true, onClick: () => emit('history') }, () => t('platformObservability.history')))
      }
      return h('div', { class: 'instance-pills' }, pills)
    }
  },
})

async function load() {
  try {
    overview.value = await getPlatformOverview({
      page: String(page.value),
      pageSize: String(pageSize.value),
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
watch(settingsOpen, async value => {
  if (!value) return
  settingsLoading.value = true
  try { settings.value = await getPlatformSettings() } catch (error) { ElMessage.error((error as Error).message) }
  finally { settingsLoading.value = false }
})
async function saveSettings() {
  settingsSaving.value = true
  try {
    settings.value = await updatePlatformSettings(settings.value)
    ElMessage.success(t('common.saved'))
  } catch (error) {
    ElMessage.error((error as Error).message)
  } finally {
    settingsSaving.value = false
  }
}
function selectInstance(component: string, cluster: string | undefined, instance: PlatformInstance) {
  selectedInstance.value = { component, cluster, instance }
  instanceOpen.value = true
}
function findComponent(row: { components: PlatformComponent[] }, component: string) {
  return row.components.find(item => item.component === component)
}
function openHistory(component: string, cluster?: string) {
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
      page: String(historyPage.value),
      pageSize: '10',
      component: historyComponent.value,
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
const role = (value: string) => value === 'leader' ? t('platformObservability.leader') : value === 'standby' ? t('platformObservability.standby') : '—'

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
.platform-alert{margin-bottom:16px}.section-heading{display:flex;align-items:flex-end;justify-content:space-between;gap:16px;margin:18px 0 12px}
.section-heading h2{margin:0 0 4px;color:var(--paper-050);font-size:16px}.section-heading p{margin:0;color:var(--steel-400);font-size:11px}
.live{display:flex;align-items:center;gap:7px;color:var(--steel-400);font:10px var(--font-mono)}.live span{width:7px;height:7px;border-radius:50%;background:var(--accent-scan);box-shadow:0 0 0 4px rgba(51,214,192,.09)}
.component-grid{display:grid;grid-template-columns:repeat(4,minmax(0,1fr));gap:12px}.member-panel{margin-top:22px;padding:18px;border:1px solid var(--ink-600);border-radius:10px;background:rgba(18,27,35,.94)}
.member-panel .section-heading{margin-top:0}.save{margin-top:18px;width:100%}
:deep(.instance-pills){display:flex;align-items:center;flex-wrap:wrap;gap:6px}
@media(max-width:1200px){.component-grid{grid-template-columns:repeat(2,minmax(0,1fr))}}
@media(max-width:700px){.component-grid{grid-template-columns:1fr}.section-heading{align-items:flex-start;flex-direction:column}}
</style>
