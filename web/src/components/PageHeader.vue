<template>
  <div class="page-header">
    <div class="page-header-leading">
      <el-button v-if="showBack" class="page-back" text @click="goBack">
        <svg aria-hidden="true" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <path d="m15 18-6-6 6-6" stroke-linecap="round" stroke-linejoin="round" />
        </svg>
        {{ $t('common.back') }}
      </el-button>
      <div class="page-header-text">
        <h1 class="page-title">{{ title }}</h1>
        <p v-if="subtitle" class="page-subtitle">{{ subtitle }}</p>
      </div>
    </div>
    <div class="page-header-actions"><slot /></div>
  </div>
</template>

<script setup lang="ts">
import { useBackNavigation } from '../composables/useBackNavigation'

defineProps<{ title: string; subtitle?: string; showBack?: boolean }>()
const { goBack } = useBackNavigation()
</script>

<style scoped>
.page-header-leading {
  display: flex;
  align-items: center;
  gap: 12px;
  min-width: 0;
}
.page-back {
  flex-shrink: 0;
  padding-inline: 6px;
}
</style>
