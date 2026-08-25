<template>
  <PageHeader :title="$t('apiDocs.title')" :subtitle="$t('apiDocs.subtitle')" />

  <section class="contract-grid" :aria-label="$t('apiDocs.title')">
    <el-card>
      <h2>{{ $t('apiDocs.consoleTitle') }}</h2>
      <p>{{ $t('apiDocs.consoleDescription') }}</p>
      <p class="contract-auth">{{ $t('apiDocs.consoleAuth') }}</p>
      <a href="/api/docs/openapi.json" target="_blank" rel="noreferrer">/api/docs/openapi.json</a>
    </el-card>
    <el-card>
      <h2>{{ $t('apiDocs.gatewayTitle') }}</h2>
      <p>{{ $t('apiDocs.gatewayDescription') }}</p>
      <p class="contract-location">{{ $t('apiDocs.gatewayLocation') }}</p>
      <p class="contract-auth">{{ $t('apiDocs.gatewayAuth') }}</p>
    </el-card>
    <el-card>
      <h2>{{ $t('apiDocs.crdTitle') }}</h2>
      <p>{{ $t('apiDocs.crdDescription') }}</p>
      <p class="contract-location">{{ $t('apiDocs.crdLocation') }}</p>
      <p class="contract-auth">{{ $t('apiDocs.crdAuth') }}</p>
    </el-card>
  </section>

  <section class="reference-section" :aria-label="$t('apiDocs.scalarTitle')">
    <div class="reference-heading">
      <div>
        <h2>{{ $t('apiDocs.scalarTitle') }}</h2>
        <p>{{ $t('apiDocs.scalarHint') }}</p>
      </div>
    </div>
    <div ref="referenceRoot" class="scalar-reference" />
  </section>
</template>

<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { createApiReference } from '@scalar/api-reference'
import '@scalar/api-reference/style.css'
import PageHeader from '../components/PageHeader.vue'

const referenceRoot = ref<HTMLElement | null>(null)
let scalarReference: ReturnType<typeof createApiReference> | undefined

onMounted(() => {
  if (!referenceRoot.value) return
  scalarReference = createApiReference(referenceRoot.value, {
    url: '/api/docs/openapi.json',
    theme: 'purple',
    layout: 'modern',
  })
})

onBeforeUnmount(() => scalarReference?.destroy())
</script>

<style scoped>
.contract-grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 16px;
  margin-bottom: 24px;
}
.contract-grid h2,
.reference-heading h2 { margin: 0 0 8px; font-size: 16px; color: var(--paper-050); }
.contract-grid p,
.reference-heading p { margin: 0 0 10px; color: var(--steel-300); line-height: 1.6; font-size: 13px; }
.contract-location { color: var(--accent-scan) !important; font-family: var(--font-mono); font-size: 12px !important; }
.contract-auth { border-left: 2px solid var(--ink-600); padding-left: 8px; }
.contract-grid a { color: var(--accent-scan); font-family: var(--font-mono); font-size: 12px; }
.reference-section { border: 1px solid var(--ink-700); border-radius: 8px; overflow: hidden; background: var(--paper-050); }
.reference-heading { padding: 18px 20px; background: var(--ink-900); border-bottom: 1px solid var(--ink-700); }
.scalar-reference { min-height: 720px; color: #1f2937; }
@media (max-width: 1100px) { .contract-grid { grid-template-columns: 1fr; } }
</style>
