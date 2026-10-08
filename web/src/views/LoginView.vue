<template>
  <AuthLayout>
      <h1 class="auth-heading">{{ $t('auth.loginTitle') }}</h1><p class="auth-hint">{{ $t('auth.loginHint') }}</p>
      <el-form label-position="top" @submit.prevent="submit">
        <el-form-item :label="$t('auth.username')"><el-input v-model="username" autocomplete="username" autofocus /></el-form-item>
        <el-form-item :label="$t('auth.password')"><el-input v-model="password" type="password" autocomplete="current-password" show-password /></el-form-item>
        <el-alert v-if="error" :title="error" type="error" show-icon :closable="false" />
        <el-button native-type="submit" type="primary" :loading="loading" class="submit">{{ $t('auth.login') }}</el-button>
      </el-form>
  </AuthLayout>
</template>
<script setup lang="ts">
import { ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { login } from '../api/auth'
import { refreshAuth } from '../auth/session'
import AuthLayout from '../layouts/AuthLayout.vue'
const route = useRoute(), router = useRouter()
const username = ref(''), password = ref(''), error = ref(''), loading = ref(false)
async function submit() {
  loading.value = true; error.value = ''
  try { await login(username.value, password.value); await refreshAuth(); await router.replace(typeof route.query.redirect === 'string' ? route.query.redirect : '/') }
  catch (e) { error.value = (e as Error).message }
  finally { loading.value = false }
}
</script>
