<template>
  <span class="status-pill" :class="statusClass">
    <span class="status-dot" />
    {{ label }}
  </span>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'

const props = defineProps<{ phase?: string, kind?: 'policy' | 'group' }>()
const { t, te } = useI18n()
const label = computed(() => {
  const contextualKey = props.kind && props.phase ? `phase.${props.kind}.${props.phase}` : ''
  if (contextualKey && te(contextualKey)) return t(contextualKey)
  return props.phase && te(`phase.${props.phase}`) ? t(`phase.${props.phase}`) : (props.phase || '-')
})
const statusClass = computed(() => {
  if (props.phase === 'Ready' || props.phase === 'Active') return 'is-success'
  if (props.phase === 'Invalid' || props.phase === 'Degraded') return 'is-danger'
  if (props.phase === 'Syncing') return 'is-warning'
  return 'is-info'
})
</script>
