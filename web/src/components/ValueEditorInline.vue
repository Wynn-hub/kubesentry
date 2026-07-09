<!-- web/src/components/ValueEditorInline.vue -->
<template>
  <el-input
    v-if="operator === 'eq' || operator === 'neq' || operator === 'regex'"
    v-model="strValue" style="width: 220px" :placeholder="$t('ruleBuilder.value')"
  />
  <el-select
    v-else-if="operator === 'in' || operator === 'notIn'"
    v-model="listValue" multiple filterable allow-create default-first-option
    style="width: 220px" :placeholder="$t('ruleBuilder.valueList')"
  />
  <el-input-number v-else-if="operator === 'gt' || operator === 'lt' || operator === 'gte' || operator === 'lte' || operator === 'countEq' || operator === 'countGt' || operator === 'countLt'" v-model="numValue" style="width: 140px" />
  <div v-else-if="operator === 'between'" style="display: flex; gap: 4px; align-items: center">
    <el-input-number v-model="rangeLow" style="width: 100px" />
    <span>–</span>
    <el-input-number v-model="rangeHigh" style="width: 100px" />
  </div>
  <span v-else style="color: var(--el-text-color-secondary)">{{ $t('ruleBuilder.noValueNeeded') }}</span>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { Condition, Operator } from './ruleGroupCodegen'

defineProps<{ operator: Operator }>()
const modelValue = defineModel<Condition['value']>({ required: true })

const strValue = computed({
  get: () => (typeof modelValue.value === 'string' ? modelValue.value : ''),
  set: (v: string) => { modelValue.value = v },
})
const numValue = computed({
  get: () => (typeof modelValue.value === 'number' ? modelValue.value : 0),
  set: (v: number) => { modelValue.value = v },
})
const listValue = computed({
  get: () => (Array.isArray(modelValue.value) ? (modelValue.value as string[]) : []),
  set: (v: string[]) => { modelValue.value = v },
})
const rangeLow = computed({
  get: () => (Array.isArray(modelValue.value) ? (modelValue.value[0] as number) : 0),
  set: (v: number) => { modelValue.value = [v, rangeHigh.value] },
})
const rangeHigh = computed({
  get: () => (Array.isArray(modelValue.value) ? (modelValue.value[1] as number) : 0),
  set: (v: number) => { modelValue.value = [rangeLow.value, v] },
})
</script>
