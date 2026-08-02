<template>
  <section>
    <PageHeader :title="$t('admissionEvent.title')" :subtitle="$t('admissionEvent.subtitle')">
      <el-button @click="openSettings">
        <el-icon style="margin-right:4px"><Setting /></el-icon>{{ $t('admissionEvent.storageSettings') }}
      </el-button>
    </PageHeader>
    <el-tabs v-model="activeTab" class="event-tabs">
      <el-tab-pane :label="$t('admissionEvent.eventListTab')" name="events">
        <AdmissionEventListView v-if="activeTab === 'events'" ref="list" embedded />
      </el-tab-pane>
      <!-- The tab is always present. Hiding it while the switch is off left no
           trace that the capability exists, and the switch itself lives on this
           page — so an operator had nowhere to discover either one. -->
      <el-tab-pane :label="$t('admissionEvent.linkHealthTab')" name="health">
        <template v-if="activeTab === 'health'">
          <AdmissionObservabilityView v-if="observationEnabled" embedded />
          <el-empty v-else class="disabled-state" :description="$t('admissionEvent.observationDisabledHint')">
            <el-button type="primary" @click="openSettings">
              {{ $t('admissionEvent.enableObservation') }}
            </el-button>
          </el-empty>
        </template>
      </el-tab-pane>
    </el-tabs>
  </section>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { Setting } from '@element-plus/icons-vue'
import PageHeader from '../components/PageHeader.vue'
import AdmissionEventListView from './AdmissionEventListView.vue'
import AdmissionObservabilityView from './AdmissionObservabilityView.vue'
import { getAdmissionSettings, type AdmissionSettings } from '../api/admissionEvent'

const route = useRoute()
const router = useRouter()
const settings = ref<AdmissionSettings>()
const activeTab = ref(route.query.tab === 'health' ? 'health' : 'events')
const list = ref<{ openSettings: () => void }>()
const observationEnabled = computed(() => settings.value?.observationEnabled ?? false)

watch(activeTab, tab => {
  void router.replace({ query: { ...route.query, tab: tab === 'events' ? undefined : tab } })
  // Reload the flag on every tab switch: the switch is edited in the event-list
  // drawer, so returning to the health tab is exactly when it may have changed.
  void loadSettings()
})

// The settings drawer lives in the event list (it edits storage rules), so the
// header action switches to that tab first and then opens it there. The list is
// v-if'd on the active tab, so one nextTick is not always enough for the ref to
// be populated — poll briefly rather than silently doing nothing.
async function openSettings() {
  if (activeTab.value !== 'events') activeTab.value = 'events'
  for (let attempt = 0; attempt < 10 && !list.value; attempt++) await nextTick()
  list.value?.openSettings()
}

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
.disabled-state{padding:48px 0}
</style>
