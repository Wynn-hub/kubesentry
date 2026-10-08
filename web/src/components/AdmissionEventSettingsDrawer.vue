<template>
  <el-drawer v-model="open" :title="$t('admissionEvent.settingsTitle')" size="min(920px, 92vw)">
    <el-form v-if="settings" label-position="top" :disabled="readonly">
      <el-alert v-if="readonly" type="info" :closable="false" :title="$t('admissionEvent.readonlyHint')" class="readonly-hint" />
      <h3 class="section-title">{{ $t('admissionEvent.deliverySettings') }}</h3>
      <div class="settings-grid">
        <el-form-item :label="$t('admissionEvent.retryEnabled')">
          <el-switch v-model="settings.retryEnabled" />
        </el-form-item>
        <el-form-item :label="$t('admissionEvent.maxRetryAttempts')">
          <el-input-number v-model="settings.maxRetryAttempts" :min="1" :max="100" :disabled="!settings.retryEnabled" />
        </el-form-item>
        <el-form-item :label="$t('admissionEvent.queueCapacity')">
          <el-input-number v-model="settings.queueCapacity" :min="1" :max="10000" />
        </el-form-item>
        <el-form-item :label="$t('admissionEvent.defaultRetentionDays')">
          <el-input-number v-model="settings.defaultRetentionDays" :min="1" :max="365" />
        </el-form-item>
        <el-form-item :label="$t('admissionEvent.incidentRetentionDays')">
          <el-input-number v-model="settings.incidentRetentionDays" :min="1" :max="365" />
        </el-form-item>
      </div>

      <h3 class="section-title">{{ $t('admissionEvent.observationSettings') }}</h3>
      <el-form-item :label="$t('admissionEvent.observationEnabled')">
        <el-switch
          v-model="settings.observationEnabled"
          :active-text="$t('common.enabled')"
          :inactive-text="$t('common.disabled')"
          :before-change="confirmObservation"
        />
      </el-form-item>
      <div class="muted observation-hint">{{ $t('admissionEvent.observationHint') }}</div>

      <div class="rules-heading">
        <div>
          <h3 class="section-title">{{ $t('admissionEvent.storageRules') }}</h3>
          <div class="muted">{{ $t('admissionEvent.ruleOrderHint') }}</div>
        </div>
        <el-button v-if="!readonly" type="primary" plain @click="addStorageRule">{{ $t('admissionEvent.addRule') }}</el-button>
      </div>

      <el-empty v-if="settings.storagePolicies.length === 0" :description="$t('admissionEvent.noRules')" :image-size="72" />
      <el-card v-for="(rule, index) in settings.storagePolicies" :key="index" class="storage-rule" shadow="never">
        <template #header>
          <div class="rule-header">
            <strong>{{ $t('admissionEvent.ruleNumber', { n: index + 1 }) }}</strong>
            <div class="rule-actions">
              <el-button link :disabled="index === 0" @click="moveStorageRule(index, -1)">↑</el-button>
              <el-button link :disabled="index === settings.storagePolicies.length - 1" @click="moveStorageRule(index, 1)">↓</el-button>
              <el-button link type="danger" @click="removeStorageRule(index)">{{ $t('common.delete') }}</el-button>
            </div>
          </div>
        </template>

        <el-form-item :label="$t('admissionEvent.matchMode')">
          <el-switch
            v-model="rule.matchMode"
            active-value="exclude"
            inactive-value="include"
            :active-text="$t('admissionEvent.exclude')"
            :inactive-text="$t('admissionEvent.include')"
          />
          <span class="mode-hint">{{ $t(rule.matchMode === 'exclude' ? 'admissionEvent.excludeHint' : 'admissionEvent.includeHint') }}</span>
        </el-form-item>

        <div class="rule-grid">
          <el-form-item :label="$t('admissionEvent.verdict')">
            <el-select v-model="rule.verdicts" multiple clearable :placeholder="$t('admissionEvent.allValues')">
              <el-option :label="$t('admissionEvent.audit')" value="audit" />
              <el-option :label="$t('admissionEvent.deny')" value="deny" />
            </el-select>
          </el-form-item>
          <el-form-item :label="$t('admissionEvent.cluster')">
            <el-select v-model="rule.clusters" multiple filterable clearable allow-create default-first-option :placeholder="$t('admissionEvent.allClusters')">
              <el-option v-for="name in clusterOptions" :key="name" :label="name" :value="name" />
            </el-select>
          </el-form-item>
          <el-form-item :label="$t('admissionEvent.namespace')">
            <TagInput v-model="rule.namespaces" :placeholder="$t('admissionEvent.allNamespaces')" />
          </el-form-item>
          <el-form-item :label="$t('admissionEvent.resource')">
            <ResourceGVRSelect v-model="rule.gvks" :placeholder="$t('admissionEvent.allResources')" />
          </el-form-item>
        </div>

        <el-alert
          v-if="rule.matchMode === 'exclude' && ruleIsEmpty(rule)"
          type="warning"
          :closable="false"
          :title="$t('admissionEvent.emptyExcludeWarning')"
          class="rule-warning"
        />
      </el-card>

      <el-alert type="info" :closable="false" :title="$t('admissionEvent.settingsHint')" />
      <el-button v-if="!readonly" type="primary" class="save" :loading="saving" @click="saveSettings">{{ $t('common.save') }}</el-button>
    </el-form>
  </el-drawer>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useI18n } from 'vue-i18n'
import { getAdmissionSettings, updateAdmissionSettings, type AdmissionSettings } from '../api/admissionEvent'
import { listClusters } from '../api/cluster'
import { collectAllPages } from '../api/pagination'
import ResourceGVRSelect from './ResourceGVRSelect.vue'
import TagInput from './TagInput.vue'

const emit = defineEmits<{ saved: [settings: AdmissionSettings] }>()
defineProps<{ readonly?: boolean }>()
const { t } = useI18n()
const open = ref(false)
const saving = ref(false)
const settings = ref<AdmissionSettings>()
const clusterOptions = ref<string[]>([])

async function show() {
  try {
    const [current, clusters] = await Promise.all([
      getAdmissionSettings(),
      collectAllPages(listClusters).catch(() => []),
    ])
    settings.value = current
    clusterOptions.value = clusters.filter(cluster => cluster.phase !== 'Uninstalled').map(cluster => cluster.name)
    open.value = true
  } catch (error) {
    ElMessage.error((error as Error).message)
  }
}

async function confirmObservation() {
  if (settings.value?.observationEnabled) return true
  try {
    await ElMessageBox.confirm(
      t('admissionEvent.observationWarning'),
      t('admissionEvent.observationWarningTitle'),
      { type: 'warning', confirmButtonText: t('common.confirm'), cancelButtonText: t('common.cancel') },
    )
    return true
  } catch {
    return false
  }
}

async function saveSettings() {
  if (!settings.value) return
  saving.value = true
  try {
    settings.value = await updateAdmissionSettings(settings.value)
    emit('saved', settings.value)
    ElMessage.success(t('common.saved'))
  } catch (error) {
    ElMessage.error((error as Error).message)
  } finally {
    saving.value = false
  }
}

function addStorageRule() {
  settings.value?.storagePolicies.push({ matchMode: 'include', verdicts: [], clusters: [], namespaces: [], gvks: [] })
}

function removeStorageRule(index: number) {
  settings.value?.storagePolicies.splice(index, 1)
}

function moveStorageRule(index: number, delta: number) {
  if (!settings.value) return
  const target = index + delta
  if (target < 0 || target >= settings.value.storagePolicies.length) return
  const [rule] = settings.value.storagePolicies.splice(index, 1)
  settings.value.storagePolicies.splice(target, 0, rule)
}

function ruleIsEmpty(rule: AdmissionSettings['storagePolicies'][number]) {
  return !rule.verdicts.length && !rule.clusters.length && !rule.namespaces.length && !rule.gvks.length
}

defineExpose({ open: show })
</script>

<style scoped>
.save{margin-top:24px}.readonly-hint{margin-bottom:16px}
.section-title{margin:0 0 12px}
.settings-grid,.rule-grid{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:0 16px}
.rules-heading{display:flex;align-items:flex-start;justify-content:space-between;margin:16px 0 12px}
.storage-rule{margin-bottom:12px}
.rule-header,.rule-actions{display:flex;align-items:center}
.rule-header{justify-content:space-between}
.rule-actions{gap:4px}
.mode-hint{margin-left:12px;color:var(--el-text-color-secondary);font-size:12px}
.rule-warning{margin-bottom:12px}
.muted{color:var(--el-text-color-secondary);font-size:12px}
@media (max-width: 760px){.settings-grid,.rule-grid{grid-template-columns:1fr}}
</style>
