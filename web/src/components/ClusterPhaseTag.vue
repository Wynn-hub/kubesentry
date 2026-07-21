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
  if (props.phase === 'Online') return 'is-success'
  if (props.phase === 'Offline') return 'is-danger'
  if (props.phase === 'Registering') return 'is-warning'
  return 'is-info'
})
</script>
