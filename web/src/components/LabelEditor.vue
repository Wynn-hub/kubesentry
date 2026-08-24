<template>
  <div style="width: 100%">
    <div v-for="(row, i) in rows" :key="i" style="display: flex; gap: 8px; margin-bottom: 8px; align-items: center">
      <el-input v-model="row.key" placeholder="key" style="width: 160px; flex-shrink: 0" @change="emitValue" />
      <TagInput v-model="row.values" :placeholder="$t('common.labelValuePlaceholder')" @update:model-value="emitValue" />
      <el-button type="danger" plain @click="rows.splice(i, 1); emitValue()">-</el-button>
    </div>
    <el-button plain @click="rows.push({ key: '', values: [] })">+</el-button>
    <HintMark :content="$t('common.labelEditorAndHint')" />
  </div>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import HintMark from './HintMark.vue'
import TagInput from './TagInput.vue'
import type { LabelSelector } from '../api/group'

const props = defineProps<{ modelValue?: LabelSelector | null }>()
const emit = defineEmits<{ 'update:modelValue': [LabelSelector | null] }>()

const rows = ref<{ key: string; values: string[] }[]>([])

// emitValue 只把"key 非空且至少有一个 value"的行写进 LabelSelector——半填
// 一半的行（比如刚输入了 key 还没加 value，或者把 value 都删光了）不应该
// 出现在下发的数据里。但这个过滤后的结果又会通过 v-model 传回 props，如果
// watch 不加区分地照单全收，就会把 rows 里那些"合法但暂时不完整"的行也
// 一起冲掉，用户刚打的 key 就凭空消失了。用 lastEmitted 记录"这次变化是不
// 是我自己刚发出去的回声"，是回声就跳过重建，rows 保持它本来更完整的状态。
let lastEmitted: string | null = null

watch(
  () => props.modelValue,
  (v) => {
    const serialized = JSON.stringify(v ?? null)
    if (serialized === lastEmitted) return
    const out: { key: string; values: string[] }[] = []
    for (const [key, value] of Object.entries(v?.matchLabels ?? {})) {
      out.push({ key, values: [value] })
    }
    // 只识别 In——Exists/NotIn/DoesNotExist 这类不是"同一个 key 多个可选值"
    // 的场景，这个编辑器不负责编辑它们，回填时按 ponytail 原则先跳过，等真
    // 的需要再加。
    for (const expr of v?.matchExpressions ?? []) {
      if (expr.operator === 'In') out.push({ key: expr.key, values: expr.values ?? [] })
    }
    rows.value = out
  },
  { immediate: true },
)

function emitValue() {
  const matchExpressions: NonNullable<LabelSelector['matchExpressions']> = rows.value
    .filter((r) => r.key.trim() !== '' && r.values.length > 0)
    .map((r) => ({ key: r.key.trim(), operator: 'In', values: r.values }))
  const out = matchExpressions.length ? { matchExpressions } : null
  lastEmitted = JSON.stringify(out)
  emit('update:modelValue', out)
}
</script>
