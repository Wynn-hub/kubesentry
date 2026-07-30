<template>
  <el-drawer :model-value="modelValue" :title="$t('observability.settings')" size="420px" @update:model-value="$emit('update:modelValue', $event)">
    <el-skeleton :loading="loading" animated>
      <el-form v-if="settings" label-position="top">
        <el-form-item :label="$t('observability.retentionDays')">
          <el-input-number v-model="settings.retentionDays" :min="1" :max="365" />
        </el-form-item>
        <p class="hint">{{ $t('observability.retentionHint') }}</p>
        <el-button type="primary" :loading="saving" @click="save">{{ $t('common.save') }}</el-button>
      </el-form>
    </el-skeleton>
  </el-drawer>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { useI18n } from 'vue-i18n'
import { getObservationSettings, updateObservationSettings, type ObservationSettings } from '../api/observability'

const props = defineProps<{ modelValue: boolean }>()
const emit = defineEmits<{ 'update:modelValue': [value: boolean] }>()
const { t } = useI18n()
const settings = ref<ObservationSettings>()
const loading = ref(false)
const saving = ref(false)

watch(() => props.modelValue, async open => {
  if (!open) return
  loading.value = true
  try { settings.value = await getObservationSettings() }
  catch (error) { ElMessage.error((error as Error).message); emit('update:modelValue', false) }
  finally { loading.value = false }
})

async function save() {
  if (!settings.value) return
  saving.value = true
  try {
    settings.value = await updateObservationSettings(settings.value)
    ElMessage.success(t('common.saved'))
    emit('update:modelValue', false)
  } catch (error) { ElMessage.error((error as Error).message) }
  finally { saving.value = false }
}
</script>

<style scoped>
.hint{color:var(--steel-300);font-size:12px;line-height:1.7;margin:-4px 0 20px}
</style>
