<template>
  <PageHeader show-back :title="$t(`comingSoon.${feature}.name`)" :subtitle="$t('comingSoon.hint')" />
  <div class="soon-card">
    <svg aria-hidden="true" width="40" height="40" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" class="soon-icon">
      <circle cx="12" cy="12" r="9" />
      <path d="M12 7v5l3 3" stroke-linecap="round" />
    </svg>
    <span class="status-pill is-warning">{{ $t(`comingSoon.${feature}.phase`) }}</span>
    <p class="soon-desc">{{ $t(`comingSoon.${feature}.desc`) }}</p>
    <p class="soon-doc mono-num">{{ $t('comingSoon.docLink') }}</p>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import PageHeader from '../components/PageHeader.vue'

const route = useRoute()
const feature = computed(() => String(route.meta.feature ?? 'clusters'))
</script>

<style scoped>
.soon-card {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 14px;
  padding: 72px 24px;
  background: var(--ink-800);
  border: 1px dashed var(--ink-600);
  border-radius: 12px;
  text-align: center;
}
.soon-icon {
  color: var(--steel-400);
}
.soon-desc {
  margin: 0;
  max-width: 560px;
  color: var(--steel-300);
  font-size: 13.5px;
  line-height: 1.7;
}
.soon-doc {
  margin: 0;
  font-size: 11px;
  color: var(--steel-400);
}
</style>
