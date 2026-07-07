<template>
  <el-tag :type="type" size="small">{{ phase || '-' }}</el-tag>
</template>

<script setup lang="ts">
import { computed } from 'vue'

const props = defineProps<{ phase?: string }>()
const type = computed(() => {
  if (props.phase === 'Ready' || props.phase === 'Active') return 'success'
  if (props.phase === 'Invalid' || props.phase === 'Degraded') return 'danger'
  return 'info'
})
</script>
