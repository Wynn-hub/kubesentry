<template>
  <AuthLayout>
      <h1 class="auth-heading">{{ $t('auth.setupTitle') }}</h1><p class="auth-hint">{{ $t('auth.setupHint') }}</p>
      <el-form label-position="top" @submit.prevent="submit">
        <el-form-item :label="$t('auth.password')"><el-input v-model="password" type="password" autocomplete="new-password" show-password autofocus /></el-form-item>
        <el-form-item :label="$t('auth.confirmPassword')"><el-input v-model="confirmation" type="password" autocomplete="new-password" show-password /></el-form-item>
        <el-alert v-if="error" :title="error" type="error" show-icon :closable="false" />
        <el-button native-type="submit" type="primary" :loading="loading" class="submit">{{ $t('auth.initialize') }}</el-button>
      </el-form>
  </AuthLayout>
</template>
<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { setupAdmin } from '../api/auth'
import { refreshAuth } from '../auth/session'
import AuthLayout from '../layouts/AuthLayout.vue'
const router = useRouter(), password = ref(''), confirmation = ref(''), error = ref(''), loading = ref(false)
const { t } = useI18n()
async function submit() {
  if (password.value !== confirmation.value) { error.value = t('auth.passwordMismatch'); return }
  loading.value = true; error.value = ''
  try { await setupAdmin(password.value); await refreshAuth(); await router.replace({ name: 'login' }) }
  catch (e) { error.value = (e as Error).message }
  finally { loading.value = false }
}
</script>
