<template>
  <el-drawer v-model="visible" :title="$t('user.securitySettings')" size="min(640px, 92vw)">
    <el-form v-if="form" label-position="top">
      <div class="grid">
        <el-form-item :label="$t('user.passwordLifetimeDays')"><el-input-number v-model="form.passwordLifetimeDays" :min="0" /></el-form-item>
        <el-form-item :label="$t('user.passwordMinLength')"><el-input-number v-model="form.passwordMinLength" :min="8" /></el-form-item>
        <el-form-item :label="$t('user.passwordHistoryCount')"><el-input-number v-model="form.passwordHistoryCount" :min="0" /></el-form-item>
        <el-form-item :label="$t('user.activationLifetimeHours')"><el-input-number v-model="form.activationLifetimeHours" :min="1" /></el-form-item>
        <el-form-item :label="$t('user.maxFailedLoginAttempts')"><el-input-number v-model="form.maxFailedLoginAttempts" :min="1" /></el-form-item>
        <el-form-item :label="$t('user.lockoutDurationMinutes')"><el-input-number v-model="form.lockoutDurationMinutes" :min="1" /></el-form-item>
        <el-form-item :label="$t('user.sessionIdleMinutes')"><el-input-number v-model="form.sessionIdleMinutes" :min="1" /></el-form-item>
        <el-form-item :label="$t('user.sessionAbsoluteMinutes')"><el-input-number v-model="form.sessionAbsoluteMinutes" :min="1" /></el-form-item>
      </div>
      <el-form-item :label="$t('user.allowConcurrentSessions')">
        <el-switch v-model="form.allowConcurrentSessions" :active-text="$t('common.enabled')" :inactive-text="$t('common.disabled')" />
      </el-form-item>
      <el-alert :title="$t('user.passwordLifetimeHint')" type="info" :closable="false" />
      <el-button class="save" type="primary" :loading="saving" @click="save">{{ $t('common.save') }}</el-button>
    </el-form>
  </el-drawer>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { ElMessage } from 'element-plus'
import { useI18n } from 'vue-i18n'
import { getSecuritySettings, updateSecuritySettings, type SecuritySettingsInput } from '../api/identity'

const { t } = useI18n()
const visible = ref(false)
const saving = ref(false)
const form = ref<SecuritySettingsInput>()

async function open() {
  try {
    form.value = { ...await getSecuritySettings() }
    visible.value = true
  } catch (error) { ElMessage.error((error as Error).message) }
}

async function save() {
  if (!form.value) return
  saving.value = true
  try {
    form.value = { ...await updateSecuritySettings(form.value) }
    ElMessage.success(t('common.saved'))
  } catch (error) { ElMessage.error((error as Error).message) }
  finally { saving.value = false }
}

defineExpose({ open })
</script>

<style scoped>.grid{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:0 16px}.save{margin-top:20px}@media(max-width:640px){.grid{grid-template-columns:1fr}}</style>
