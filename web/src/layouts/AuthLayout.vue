<template>
  <main class="auth-shell">
    <section class="brand-panel">
      <div class="brand-lockup"><Logo :size="36" /><span>KubeSentry</span></div>
      <div class="brand-copy">
        <p class="eyebrow">KUBERNETES ADMISSION CONTROL</p>
        <h1>{{ $t('auth.brandTitle') }}</h1>
        <p>{{ $t('auth.brandDescription') }}</p>
      </div>
      <span class="version">KubeSentry Console</span>
    </section>
    <section class="form-panel">
      <header><div class="mobile-brand"><Logo :size="28" /><span>KubeSentry</span></div><el-select :model-value="locale" class="locale" @change="onLocale"><el-option label="中文" value="zh-CN" /><el-option label="English" value="en-US" /></el-select></header>
      <div class="form-wrap"><slot /></div>
      <footer>© 2026 KubeSentry</footer>
    </section>
  </main>
</template>
<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import Logo from '../components/Logo.vue'
import { setLocale } from '../locales'
const { locale } = useI18n()
const onLocale = (value: 'zh-CN' | 'en-US') => setLocale(value)
</script>
<style scoped>
.auth-shell{min-height:100vh;display:grid;grid-template-columns:minmax(340px,44%) 1fr;background:var(--ink-950);color:var(--paper-050)}
.brand-panel{position:relative;display:flex;flex-direction:column;padding:48px 56px;background:radial-gradient(circle at 25% 30%,rgba(51,214,192,.16),transparent 42%),linear-gradient(145deg,var(--ink-800),var(--ink-950));border-right:1px solid var(--ink-700);overflow:hidden}
.brand-panel:after{content:"";position:absolute;inset:0;background:repeating-linear-gradient(135deg,rgba(255,255,255,.025) 0 1px,transparent 1px 18px);pointer-events:none}.brand-lockup,.mobile-brand{display:flex;align-items:center;gap:12px;font-size:20px;font-weight:750;position:relative;z-index:1}.brand-copy{margin:auto 0;max-width:520px;position:relative;z-index:1}.eyebrow{font:600 11px/1.4 var(--font-mono);letter-spacing:.16em;color:var(--accent-scan)}.brand-copy h1{font-size:clamp(34px,4vw,58px);line-height:1.08;margin:18px 0}.brand-copy>p:last-child{max-width:470px;color:var(--steel-300);font-size:16px;line-height:1.8}.version{position:relative;z-index:1;color:var(--steel-400);font:12px var(--font-mono)}
.form-panel{display:grid;grid-template-rows:auto 1fr auto;min-width:0;padding:28px 42px}.form-panel header{display:flex;justify-content:flex-end}.locale{width:116px}.mobile-brand{display:none}.form-wrap{width:min(440px,100%);margin:auto}.form-wrap :deep(.auth-heading){font-size:30px;margin:0 0 10px}.form-wrap :deep(.auth-hint){color:var(--steel-300);line-height:1.65;margin:0 0 28px}.form-wrap :deep(.submit){width:100%;margin-top:18px}.form-panel footer{text-align:center;color:var(--steel-500);font:11px var(--font-mono)}
@media(max-width:800px){.auth-shell{display:block}.brand-panel{display:none}.form-panel{min-height:100vh;padding:22px 20px}.form-panel header{justify-content:space-between;align-items:center}.mobile-brand{display:flex;font-size:16px}.form-wrap{padding:36px 0}}
</style>
