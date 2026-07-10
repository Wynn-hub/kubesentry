<!-- web/src/components/ConditionEditor.vue -->
<template>
  <div style="display: flex; gap: 8px; align-items: center">
    <FieldPathCascader
      :group="group" :version="version" :resource="resource" :path="modelValue.path"
      @update:field="onField"
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

// onField 把字段路径和叶子类型的更新合并成对 modelValue 的一次读-改-写。
// path 和 leafType 本该在同一次选择里一起生效——分两次独立的 modelValue.value
// 赋值会互相踩踏：defineModel 的 prop 要等父组件重渲染才反映上一次写入，第二
// 次赋值前读到的还是旧值，等于把第一次的更新覆盖掉。
function onField({ path, leafType: newType }: { path: PathSegment[]; leafType: FieldLeafType }) {
  // 编辑已保存策略时，级联选择器加载完 schema 会带着 annotation 恢复出的同一
  // 条 path 重放一次本事件（此时本地 leafType 还是初始的 'string'）——路径没
  // 变就不能按"类型切换"清值，否则恢复出来的 value 会被误删。
  const samePath = JSON.stringify(path) === JSON.stringify(modelValue.value.path)
  const typeChanged = newType !== leafType.value
  leafType.value = newType
  const validOperators = OPERATORS_BY_TYPE[newType]
  const operator = validOperators.some((op) => op.value === modelValue.value.operator)
    ? modelValue.value.operator
    : validOperators[0].value
  modelValue.value = {
    ...modelValue.value,
    path,
    operator,
    // 字段路径切到了不同类型的叶子（比如从 map 的 string 值切到一个 integer
    // 字段），旧值的类型已经对不上了，留着会悄悄拼进生成的 Rego 里造出一个
    // 永远不匹配（或永远匹配）的条件——切换类型时把值一并清空。
    value: typeChanged && !samePath ? undefined : modelValue.value.value,
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
