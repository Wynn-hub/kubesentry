<template>
  <span class="status-pill" :class="statusClass">
    <span class="status-dot" />
    {{ phase || '-' }}
  </span>
</template>

<script setup lang="ts">
import { computed } from 'vue'

const props = defineProps<{ phase?: string }>()
const statusClass = computed(() => {
  if (props.phase === 'Ready' || props.phase === 'Active') return 'is-success'
  if (props.phase === 'Invalid' || props.phase === 'Degraded') return 'is-danger'
  if (props.phase === 'Syncing') return 'is-warning'
  return 'is-info'
})
</script>
