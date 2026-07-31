<template>
  <section>
    <PageHeader :title="$t('admissionEvent.title')" :subtitle="$t('admissionEvent.subtitle')" />
    <el-tabs v-model="activeTab" class="event-tabs">
      <el-tab-pane :label="$t('admissionEvent.eventListTab')" name="events">
        <AdmissionEventListView v-if="activeTab === 'events'" embedded />
      </el-tab-pane>
      <el-tab-pane v-if="settings.admissionPipelineEnabled" :label="$t('admissionEvent.linkHealthTab')" name="health">
        <AdmissionObservabilityView v-if="activeTab === 'health'" embedded />
      </el-tab-pane>
    </el-tabs>
  </section>
</template>

<script setup lang="ts">
import { onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import PageHeader from '../components/PageHeader.vue'
import AdmissionEventListView from './AdmissionEventListView.vue'
import AdmissionObservabilityView from './AdmissionObservabilityView.vue'
import { getPlatformSettings, type PlatformSettings } from '../api/platform'

const route = useRoute()
const router = useRouter()
const settings = ref<PlatformSettings>({ admissionPipelineEnabled: false })
const activeTab = ref(route.query.tab === 'health' ? 'health' : 'events')

watch(activeTab, tab => {
  void router.replace({ query: { ...route.query, tab: tab === 'events' ? undefined : tab } })
})

onMounted(async () => {
  try {
    settings.value = await getPlatformSettings()
    if (!settings.value.admissionPipelineEnabled && activeTab.value === 'health') activeTab.value = 'events'
  } catch (error) {
    ElMessage.error((error as Error).message)
    activeTab.value = 'events'
  }
})
</script>

<style scoped>
.event-tabs{margin-top:8px}
</style>
