<template>
  <el-container class="shell">
    <el-aside width="212px" class="shell-aside">
      <div class="brand">
        <Logo :size="24" />
        <span>KubeSentry</span>
      </div>
      <el-menu :default-active="$route.path" router class="nav">
        <el-menu-item index="/">
          <svg class="nav-icon" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <rect x="3" y="3" width="8" height="8" rx="1.5" /><rect x="13" y="3" width="8" height="8" rx="1.5" />
            <rect x="3" y="13" width="8" height="8" rx="1.5" /><rect x="13" y="13" width="8" height="8" rx="1.5" />
          </svg>
          {{ $t('nav.overview') }}
        </el-menu-item>
        <el-menu-item index="/policies">
          <svg class="nav-icon" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M6 3h9l4 4v14a1 1 0 0 1-1 1H6a1 1 0 0 1-1-1V4a1 1 0 0 1 1-1Z" stroke-linejoin="round" />
            <path d="M8.5 13.5l2.2 2.2L16 11" stroke-linecap="round" stroke-linejoin="round" />
          </svg>
          {{ $t('nav.policies') }}
        </el-menu-item>
        <el-menu-item index="/policygroups">
          <svg class="nav-icon" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M12 3 3 8l9 5 9-5-9-5Z" stroke-linejoin="round" />
            <path d="M3 13l9 5 9-5" stroke-linecap="round" stroke-linejoin="round" />
          </svg>
          {{ $t('nav.groups') }}
        </el-menu-item>
        <el-menu-item index="/exceptions">
          <svg class="nav-icon" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M12 3 4 6.5V12c0 5 3.4 8.3 8 9 4.6-.7 8-4 8-9V6.5L12 3Z" stroke-linejoin="round" />
            <path d="M8.5 8.5l7 7" stroke-linecap="round" />
          </svg>
          {{ $t('nav.exceptions') }}
        </el-menu-item>
      </el-menu>
    </el-aside>
    <el-container class="shell-main">
      <el-header class="shell-header">
        <el-select :model-value="locale" style="width: 110px" @change="onLocale">
          <el-option label="中文" value="zh-CN" />
          <el-option label="English" value="en-US" />
        </el-select>
      </el-header>
      <el-main class="shell-content"><router-view /></el-main>
    </el-container>
  </el-container>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { setLocale } from '../locales'
import Logo from '../components/Logo.vue'

const { locale } = useI18n()
const onLocale = (l: 'zh-CN' | 'en-US') => setLocale(l)
</script>

<style scoped>
.shell {
  height: 100vh;
}

.shell-aside {
  background: var(--ink-900);
  border-right: 1px solid var(--ink-700);
  display: flex;
  flex-direction: column;
}

.brand {
  display: flex;
  align-items: center;
  gap: 9px;
  padding: 18px 18px 16px;
  font-weight: 700;
  font-size: 15px;
  letter-spacing: -0.01em;
  color: var(--paper-050);
  border-bottom: 1px solid var(--ink-700);
}

.nav {
  border-right: none;
  background: transparent;
  padding: 12px 8px;
}

.nav :deep(.el-menu-item) {
  height: 38px;
  line-height: 38px;
  border-radius: 8px;
  margin-bottom: 2px;
  gap: 10px;
  font-size: 13px;
}

.nav-icon {
  flex-shrink: 0;
}

.nav :deep(.el-menu-item.is-active) {
  background: rgba(51, 214, 192, 0.1);
  box-shadow: inset 2px 0 0 var(--accent-scan);
}

.shell-header {
  display: flex;
  justify-content: flex-end;
  align-items: center;
  border-bottom: 1px solid var(--ink-700);
  background: var(--ink-900);
}

.shell-content {
  background:
    repeating-linear-gradient(135deg, rgba(255, 255, 255, 0.012) 0 1px, transparent 1px 14px),
    var(--ink-950);
}
</style>
