<template>
  <PageHeader :title="content.title" :subtitle="content.subtitle" />
  <div class="help-layout">
    <nav class="help-nav" :aria-label="content.title"><a v-for="s in content.sections" :key="s.id" :href="`#${s.id}`">{{ s.title }}</a></nav>
    <main class="help-content">
      <section v-for="s in content.sections" :id="s.id" :key="s.id">
        <h2>{{ s.title }}</h2><p v-for="p in s.paragraphs" :key="p">{{ p }}</p>
        <ul><li v-for="item in s.items" :key="item">{{ item }}</li></ul>
        <a v-if="s.link" :href="s.link.href" target="_blank" rel="noreferrer">{{ s.link.label }}</a>
      </section>
    </main>
  </div>
</template>
<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import PageHeader from '../components/PageHeader.vue'
import { helpContent } from './helpContent'

const { locale } = useI18n()
const content = computed(() => helpContent[locale.value === 'en-US' ? 'en-US' : 'zh-CN'])
</script>
<style scoped>
.help-layout{display:grid;grid-template-columns:220px minmax(0,820px);gap:24px;align-items:start}.help-nav{position:sticky;top:16px;display:flex;flex-direction:column;padding:10px;border:1px solid var(--ink-700);border-radius:10px;background:var(--ink-900)}.help-nav a{padding:8px 10px;border-radius:6px;color:var(--steel-300);font-size:13px;text-decoration:none}.help-nav a:hover,.help-nav a:focus-visible{background:var(--ink-800);color:var(--accent-scan)}.help-content{display:flex;flex-direction:column;gap:14px}.help-content section{scroll-margin-top:16px;padding:22px 24px;border:1px solid var(--ink-700);border-radius:10px;background:var(--ink-900);color:var(--steel-300)}h2{margin:0 0 12px;color:var(--paper-050);font-size:18px}p,li{line-height:1.7}li{margin:6px 0}.help-content a{color:var(--accent-scan)}@media(max-width:800px){.help-layout{grid-template-columns:1fr}.help-nav{position:static}}
</style>
