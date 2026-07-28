<template>
  <el-select
    :model-value="modelValue"
    multiple
    collapse-tags
    filterable
    clearable
    allow-create
    default-first-option
    :placeholder="placeholder"
    @update:model-value="emit('update:modelValue', $event)"
  >
    <el-option v-for="gvr in options" :key="gvr" :label="gvr" :value="gvr" />
  </el-select>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive } from 'vue'
import { getResourceSuggestions } from '../api/policy'

defineProps<{ modelValue: string[]; placeholder?: string }>()
const emit = defineEmits<{ 'update:modelValue': [string[]] }>()

const suggestions = reactive({ resourcesByGroup: {} as Record<string, string[]> })
const options = computed(() => {
  const values = new Set<string>()
  for (const [group, resources] of Object.entries(suggestions.resourcesByGroup)) {
    for (const resource of resources) values.add(group === '' ? resource : `${group}/${resource}`)
  }
  return [...values].sort()
})

onMounted(async () => {
  try {
    const result = await getResourceSuggestions()
    suggestions.resourcesByGroup = result.resourcesByGroup
  } catch {
    // allow-create keeps the control usable when discovery is temporarily unavailable.
  }
})
</script>
