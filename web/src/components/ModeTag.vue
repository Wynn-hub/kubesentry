<template>
  <span class="status-pill" :class="mode === 'enforce' ? 'is-danger' : 'is-scan'">
    <span class="status-dot" />
    {{ mode === 'enforce' ? $t('policy.enforce') : $t('policy.audit') }}
  </span>
</template>

<script setup lang="ts">
defineProps<{ mode: string }>()
</script>
