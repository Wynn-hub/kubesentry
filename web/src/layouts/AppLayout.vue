<template>
  <el-container class="shell">
    <el-aside width="212px" class="shell-aside">
      <router-link to="/" class="brand" :aria-label="$t('nav.home')" @keydown.space.prevent="$router.push('/')">
        <Logo :size="24" />
        <span>KubeSentry</span>
      </router-link>
      <el-tooltip :content="$t('nav.clusterTip')" placement="right">
        <div class="cluster-switcher">
          <svg aria-hidden="true" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <circle cx="12" cy="12" r="3" />
            <path d="M12 2v3M12 19v3M2 12h3M19 12h3M4.9 4.9l2.2 2.2M16.9 16.9l2.2 2.2M19.1 4.9l-2.2 2.2M7.1 16.9l-2.2 2.2" stroke-linecap="round" />
          </svg>
          <span class="cluster-name">local</span>
          <span class="cluster-badge">{{ $t('nav.soon') }}</span>
        </div>
      </el-tooltip>
      <el-menu :default-active="$route.path" router class="nav">
        <div class="nav-group-title">{{ $t('nav.groupPlatform') }}</div>
        <el-menu-item index="/">
          <svg aria-hidden="true" class="nav-icon" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <rect x="3" y="3" width="8" height="8" rx="1.5" /><rect x="13" y="3" width="8" height="8" rx="1.5" />
            <rect x="3" y="13" width="8" height="8" rx="1.5" /><rect x="13" y="13" width="8" height="8" rx="1.5" />
          </svg>
          {{ $t('nav.overview') }}
        </el-menu-item>
        <el-menu-item index="/clusters">
          <svg aria-hidden="true" class="nav-icon" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <circle cx="6" cy="6" r="3" /><circle cx="18" cy="6" r="3" /><circle cx="12" cy="18" r="3" />
            <path d="M8 8l3 7M16 8l-3 7" stroke-linecap="round" />
          </svg>
          {{ $t('nav.clusters') }}
        </el-menu-item>
        <el-menu-item v-if="can('platform.manage')" index="/tenants">
          <svg aria-hidden="true" class="nav-icon" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <circle cx="9" cy="8" r="3.5" /><path d="M3 20c0-3.3 2.7-6 6-6s6 2.7 6 6" stroke-linecap="round" />
            <path d="M16 4.5a3.5 3.5 0 0 1 0 7M21 20c0-2.8-1.9-5.1-4.5-5.8" stroke-linecap="round" />
          </svg>
          {{ $t('nav.tenants') }}
        </el-menu-item>
        <el-menu-item v-if="can('tenant.manage')" index="/users">{{ $t('nav.users') }}</el-menu-item>

        <div class="nav-group-title">{{ $t('nav.groupProtection') }}</div>
        <el-menu-item index="/policies">
          <svg aria-hidden="true" class="nav-icon" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M6 3h9l4 4v14a1 1 0 0 1-1 1H6a1 1 0 0 1-1-1V4a1 1 0 0 1 1-1Z" stroke-linejoin="round" />
            <path d="M8.5 13.5l2.2 2.2L16 11" stroke-linecap="round" stroke-linejoin="round" />
          </svg>
          {{ $t('nav.policies') }}
        </el-menu-item>
        <el-menu-item index="/policygroups">
          <svg aria-hidden="true" class="nav-icon" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M12 3 3 8l9 5 9-5-9-5Z" stroke-linejoin="round" />
            <path d="M3 13l9 5 9-5" stroke-linecap="round" stroke-linejoin="round" />
          </svg>
          {{ $t('nav.groups') }}
        </el-menu-item>
        <el-menu-item index="/exceptions">
          <svg aria-hidden="true" class="nav-icon" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M12 3 4 6.5V12c0 5 3.4 8.3 8 9 4.6-.7 8-4 8-9V6.5L12 3Z" stroke-linejoin="round" />
            <path d="M8.5 8.5l7 7" stroke-linecap="round" />
          </svg>
          {{ $t('nav.exceptions') }}
        </el-menu-item>
        <div class="nav-group-title">{{ $t('nav.groupObservability') }}</div>
        <el-menu-item index="/observability">
          <svg aria-hidden="true" class="nav-icon" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M4 19V9M10 19V5M16 19v-7M22 19V3" stroke-linecap="round" />
            <path d="M2 19h20" stroke-linecap="round" />
          </svg>
          {{ $t('nav.observability') }}
        </el-menu-item>
        <el-menu-item index="/events">
          <svg aria-hidden="true" class="nav-icon" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M4 17l4-8 4 5 3-3 5 6" stroke-linecap="round" stroke-linejoin="round" />
            <path d="M3 21h18" stroke-linecap="round" />
          </svg>
          {{ $t('nav.events') }}
        </el-menu-item>
        <el-menu-item index="/audit" class="nav-soon">
          <svg aria-hidden="true" class="nav-icon" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <circle cx="11" cy="11" r="7" /><path d="M21 21l-4.5-4.5" stroke-linecap="round" />
          </svg>
          {{ $t('nav.audit') }}<span class="soon-badge">{{ $t('nav.soon') }}</span>
        </el-menu-item>
        <el-menu-item index="/alerts" class="nav-soon">
          <svg aria-hidden="true" class="nav-icon" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M18 9a6 6 0 1 0-12 0c0 6-2 7-2 7h16s-2-1-2-7Z" stroke-linejoin="round" />
            <path d="M10 20a2 2 0 0 0 4 0" stroke-linecap="round" />
          </svg>
          {{ $t('nav.alerts') }}<span class="soon-badge">{{ $t('nav.soon') }}</span>
        </el-menu-item>
        <div class="nav-group-title">{{ $t('nav.groupSystem') }}</div>
        <el-menu-item index="/system/api-docs">
          <svg aria-hidden="true" class="nav-icon" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M4 5.5A2.5 2.5 0 0 1 6.5 3H20v16.5A1.5 1.5 0 0 0 18.5 18H6.5A2.5 2.5 0 0 0 4 20.5v-15Z" stroke-linejoin="round" />
            <path d="M8 7h8M8 11h8M8 15h4" stroke-linecap="round" />
          </svg>
          {{ $t('nav.apiDocs') }}
        </el-menu-item>
      </el-menu>
    </el-aside>
    <el-container class="shell-main">
      <el-header class="shell-header">
        <el-select :model-value="locale" style="width: 110px" @change="onLocale">
          <el-option label="中文" value="zh-CN" />
          <el-option label="English" value="en-US" />
        </el-select>
        <span class="current-user">{{ authState.me?.username }}</span>
        <el-button text @click="onLogout">{{ $t('auth.logout') }}</el-button>
      </el-header>
      <el-main class="shell-content"><router-view /></el-main>
    </el-container>
  </el-container>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { setLocale } from '../locales'
import Logo from '../components/Logo.vue'
import { useRouter } from 'vue-router'
import { authState, can, signOut } from '../auth/session'

const { locale } = useI18n()
const router = useRouter()
const onLocale = (l: 'zh-CN' | 'en-US') => setLocale(l)
const onLogout = async () => { await signOut(); await router.push({ name: 'login' }) }
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
  overflow-y: auto;
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
  text-decoration: none;
  transition: background-color 0.15s ease, color 0.15s ease;
}
.brand:hover {
  background: var(--ink-800);
  color: var(--accent-scan);
}
.brand:focus-visible {
  outline: 2px solid var(--accent-scan);
  outline-offset: -3px;
}

.cluster-switcher {
  display: flex;
  align-items: center;
  gap: 8px;
  margin: 12px 12px 0;
  padding: 8px 12px;
  border: 1px solid var(--ink-600);
  border-radius: 8px;
  background: var(--ink-800);
  color: var(--steel-300);
  font-size: 12.5px;
  cursor: default;
}
.cluster-name {
  flex: 1;
  font-family: var(--font-mono);
  color: var(--paper-050);
}
.cluster-badge,
.soon-badge {
  font-size: 10px;
  font-family: var(--font-mono);
  letter-spacing: 0.04em;
  color: var(--steel-400);
  border: 1px solid var(--ink-600);
  border-radius: 999px;
  padding: 1px 7px;
  line-height: 1.6;
  flex-shrink: 0;
}
.soon-badge {
  margin-left: 8px;
}

.nav {
  border-right: none;
  background: transparent;
  padding: 8px 8px 16px;
}

.nav-group-title {
  font-family: var(--font-mono);
  font-size: 10px;
  letter-spacing: 0.08em;
  text-transform: uppercase;
  color: var(--steel-400);
  padding: 14px 12px 6px;
}

.nav :deep(.el-menu-item) {
  height: 38px;
  line-height: 38px;
  border-radius: 8px;
  margin-bottom: 2px;
  gap: 10px;
  font-size: 13px;
}

.nav :deep(.el-menu-item.nav-soon) {
  color: var(--steel-400);
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
.current-user { margin-left: 12px; color: var(--steel-300); font-size: 13px; }

.shell-content {
  background:
    repeating-linear-gradient(135deg, rgba(255, 255, 255, 0.012) 0 1px, transparent 1px 14px),
    var(--ink-950);
}
</style>
