<!-- web/src/components/ConditionEditor.vue -->
<template>
  <div style="display: flex; gap: 8px; align-items: center">
    <FieldPathCascader
      :group="group" :version="version" :resource="resource"
      @update:path="onPath" @update:leaf-type="onLeafType"
    />
    <el-select v-model="modelValue.operator" style="width: 180px" :placeholder="$t('ruleBuilder.operator')">
      <el-option v-for="op in operatorOptions" :key="op.value" :value="op.value" :label="$t(op.labelKey)" />
    </el-select>
    <ValueEditorInline v-model="modelValue.value" :operator="modelValue.operator" :leaf-type="leafType" />
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import FieldPathCascader from './FieldPathCascader.vue'
import ValueEditorInline from './ValueEditorInline.vue'
import type { Condition, FieldLeafType, Operator, PathSegment } from './ruleGroupCodegen'

defineProps<{ group: string; version: string; resource: string }>()
const modelValue = defineModel<Condition>({ required: true })

function onPath(path: PathSegment[]) {
  modelValue.value = { ...modelValue.value, path }
}
function onLeafType(t: FieldLeafType) {
  // 字段路径切到了不同类型的叶子（比如从 map 的 string 值切到一个 integer
  // 字段），旧值的类型已经对不上了，留着会悄悄拼进生成的 Rego 里造出一个
  // 永远不匹配（或永远匹配）的条件——切换类型时把值一并清空。
  if (t !== leafType.value) {
    modelValue.value = { ...modelValue.value, value: undefined }
  }
  leafType.value = t
  const validOperators = OPERATORS_BY_TYPE[t]
  if (!validOperators.some((op) => op.value === modelValue.value.operator)) {
    modelValue.value = { ...modelValue.value, operator: validOperators[0].value }
  }
}

const leafType = defineModel<FieldLeafType>('leafType', { default: 'string' })

const OPERATORS_BY_TYPE: Record<FieldLeafType, { value: Operator; labelKey: string }[]> = {
  string: [
    { value: 'eq', labelKey: 'ruleBuilder.opEq' },
    { value: 'neq', labelKey: 'ruleBuilder.opNeq' },
    { value: 'exists', labelKey: 'ruleBuilder.opExists' },
    { value: 'notExists', labelKey: 'ruleBuilder.opNotExists' },
    { value: 'in', labelKey: 'ruleBuilder.opIn' },
    { value: 'notIn', labelKey: 'ruleBuilder.opNotIn' },
    { value: 'regex', labelKey: 'ruleBuilder.opRegex' },
  ],
  integer: [
    { value: 'eq', labelKey: 'ruleBuilder.opEq' },
    { value: 'neq', labelKey: 'ruleBuilder.opNeq' },
    { value: 'exists', labelKey: 'ruleBuilder.opExists' },
    { value: 'notExists', labelKey: 'ruleBuilder.opNotExists' },
    { value: 'gt', labelKey: 'ruleBuilder.opGt' },
    { value: 'lt', labelKey: 'ruleBuilder.opLt' },
    { value: 'gte', labelKey: 'ruleBuilder.opGte' },
    { value: 'lte', labelKey: 'ruleBuilder.opLte' },
    { value: 'between', labelKey: 'ruleBuilder.opBetween' },
  ],
  number: [],
  boolean: [
    { value: 'isTrue', labelKey: 'ruleBuilder.opIsTrue' },
    { value: 'isFalse', labelKey: 'ruleBuilder.opIsFalse' },
  ],
  array: [
    { value: 'exists', labelKey: 'ruleBuilder.opExists' },
    { value: 'notExists', labelKey: 'ruleBuilder.opNotExists' },
    { value: 'countEq', labelKey: 'ruleBuilder.opCountEq' },
    { value: 'countGt', labelKey: 'ruleBuilder.opCountGt' },
    { value: 'countLt', labelKey: 'ruleBuilder.opCountLt' },
  ],
  object: [
    { value: 'exists', labelKey: 'ruleBuilder.opExists' },
    { value: 'notExists', labelKey: 'ruleBuilder.opNotExists' },
    { value: 'countEq', labelKey: 'ruleBuilder.opCountEq' },
    { value: 'countGt', labelKey: 'ruleBuilder.opCountGt' },
    { value: 'countLt', labelKey: 'ruleBuilder.opCountLt' },
  ],
}
OPERATORS_BY_TYPE.number = OPERATORS_BY_TYPE.integer

const operatorOptions = computed(() => OPERATORS_BY_TYPE[leafType.value])
</script>
