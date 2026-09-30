<template>
  <main class="auth-page">
    <el-card class="auth-card">
      <h1>{{ $t('auth.loginTitle') }}</h1>
      <el-form label-position="top" @submit.prevent="submit">
        <el-form-item :label="$t('auth.username')"><el-input v-model="username" autocomplete="username" autofocus /></el-form-item>
        <el-form-item :label="$t('auth.password')"><el-input v-model="password" type="password" autocomplete="current-password" show-password /></el-form-item>
        <el-alert v-if="error" :title="error" type="error" show-icon :closable="false" />
        <el-button native-type="submit" type="primary" :loading="loading" class="submit">{{ $t('auth.login') }}</el-button>
      </el-form>
    </el-card>
  </main>
</template>
<script setup lang="ts">
import { ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { login } from '../api/auth'
import { refreshAuth } from '../auth/session'
const route = useRoute(), router = useRouter()
const username = ref(''), password = ref(''), error = ref(''), loading = ref(false)
async function submit() {
  loading.value = true; error.value = ''
  try { await login(username.value, password.value); await refreshAuth(); await router.replace(typeof route.query.redirect === 'string' ? route.query.redirect : '/') }
  catch (e) { error.value = (e as Error).message }
  finally { loading.value = false }
}
</script>
<style scoped>
.auth-page{min-height:100vh;display:grid;place-items:center;background:var(--ink-950)}.auth-card{width:min(400px,calc(100vw - 32px))}.submit{width:100%;margin-top:18px}h1{margin:0 0 24px}
</style>
