<template>
  <main class="auth-page">
    <el-card class="auth-card">
      <h1>{{ $t('auth.setupTitle') }}</h1><p>{{ $t('auth.setupHint') }}</p>
      <el-form label-position="top" @submit.prevent="submit">
        <el-form-item :label="$t('auth.password')"><el-input v-model="password" type="password" autocomplete="new-password" show-password autofocus /></el-form-item>
        <el-form-item :label="$t('auth.confirmPassword')"><el-input v-model="confirmation" type="password" autocomplete="new-password" show-password /></el-form-item>
        <el-alert v-if="error" :title="error" type="error" show-icon :closable="false" />
        <el-button native-type="submit" type="primary" :loading="loading" class="submit">{{ $t('auth.initialize') }}</el-button>
      </el-form>
    </el-card>
  </main>
</template>
<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { setupAdmin } from '../api/auth'
import { refreshAuth } from '../auth/session'
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
<style scoped>
.auth-page{min-height:100vh;display:grid;place-items:center;background:var(--ink-950)}.auth-card{width:min(400px,calc(100vw - 32px))}.submit{width:100%;margin-top:18px}h1{margin:0 0 8px}p{color:var(--steel-300);margin:0 0 24px}
</style>
