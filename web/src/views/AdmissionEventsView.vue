<template>
  <section>
    <PageHeader :title="$t('admissionEvent.title')" :subtitle="$t('admissionEvent.subtitle')">
      <el-button @click="openSettings">
        <el-icon style="margin-right:4px"><Setting /></el-icon>{{ $t('admissionEvent.storageSettings') }}
      </el-button>
    </PageHeader>
    <el-tabs v-model="activeTab" class="event-tabs">
      <el-tab-pane :label="$t('admissionEvent.eventListTab')" name="events">
        <AdmissionEventListView embedded />
      </el-tab-pane>
      <!-- The tab is always present. Hiding it while the switch is off left no
           trace that the capability exists, and the switch itself lives on this
           page — so an operator had nowhere to discover either one. -->
      <el-tab-pane :label="$t('admissionEvent.linkHealthTab')" name="health">
        <template v-if="activeTab === 'health'">
          <el-alert v-if="!observationEnabled" type="info" :closable="false" :title="$t('admissionEvent.observationDisabledHint')" class="disabled-alert">
            <el-button link type="primary" @click="openSettings">{{ $t('admissionEvent.enableObservation') }}</el-button>
          </el-alert>
          <AdmissionObservabilityView embedded />
        </template>
      </el-tab-pane>
    </el-tabs>
    <AdmissionEventSettingsDrawer ref="settingsDrawer" @saved="settings = $event" />
  </section>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { Setting } from '@element-plus/icons-vue'
import PageHeader from '../components/PageHeader.vue'
import AdmissionEventSettingsDrawer from '../components/AdmissionEventSettingsDrawer.vue'
import AdmissionEventListView from './AdmissionEventListView.vue'
import AdmissionObservabilityView from './AdmissionObservabilityView.vue'
import { getAdmissionSettings, type AdmissionSettings } from '../api/admissionEvent'
import { resolveEventsTab, type AdmissionEventsTab } from './admissionEventsTabs'

const route = useRoute()
const router = useRouter()
const settings = ref<AdmissionSettings>()
const activeTab = ref<AdmissionEventsTab>(resolveEventsTab(route.query.tab))
const settingsDrawer = ref<{ open: () => void }>()
const observationEnabled = computed(() => settings.value?.observationEnabled ?? false)

// The incident drawer links to a sample event, which pushes ?tab=events on this
// same route. Without this the tab ref keeps its setup-time value and the event
// drawer opens on top of the health tab instead of beside its list.
watch(() => route.query.tab, tab => { activeTab.value = resolveEventsTab(tab) })

watch(activeTab, tab => {
  void router.replace({ query: { ...route.query, tab: tab === 'events' ? undefined : tab } })
  // Reload the flag on every tab switch: the switch is edited in the event-list
  // drawer, so returning to the health tab is exactly when it may have changed.
  void loadSettings()
})

function openSettings() { settingsDrawer.value?.open() }

async function loadSettings() {
  try {
    settings.value = await getAdmissionSettings()
  } catch (error) {
    ElMessage.error((error as Error).message)
  }
}

onMounted(loadSettings)
</script>

<style scoped>
.event-tabs{margin-top:8px}
.disabled-alert{margin-bottom:16px}
</style>
