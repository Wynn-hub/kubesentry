<!-- web/src/components/TagInput.vue -->
<template>
  <div class="tag-input-box" @click="inputRef?.focus()">
    <el-tag v-for="v in modelValue" :key="v" closable size="small" @close="remove(v)">{{ v }}</el-tag>
    <input
      ref="inputRef"
      v-model="draft"
      class="tag-input-native"
      :placeholder="modelValue.length ? '' : placeholder"
      @keyup.enter="add"
    />
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'

const props = defineProps<{ modelValue: string[]; placeholder?: string }>()
const emit = defineEmits<{ 'update:modelValue': [string[]] }>()

const draft = ref('')
const inputRef = ref<HTMLInputElement>()

// 回车时如果值已经存在，直接忽略（既不新增也不删除已有标签）——用
// el-select 的 allow-create 做这个交互时，重复输入同一个值会被内部当成
// "再次选中一个已选项"从而把它整个 toggle 掉，体验上等于"删掉了输入
// 框"。改成自己控制增删就没有这个问题。
function add() {
  const v = draft.value.trim()
  draft.value = ''
  if (v === '' || props.modelValue.includes(v)) return
  emit('update:modelValue', [...props.modelValue, v])
}

function remove(v: string) {
  emit('update:modelValue', props.modelValue.filter((x) => x !== v))
}
</script>

<style scoped>
.tag-input-box {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px;
  width: 100%;
  min-height: 32px;
  box-sizing: border-box;
  padding: 3px 8px;
  border: 1px solid var(--el-border-color);
  border-radius: var(--el-border-radius-base);
  background: var(--el-fill-color-blank);
  cursor: text;
}
.tag-input-box:focus-within {
  border-color: var(--el-color-primary);
}
.tag-input-native {
  flex: 1;
  min-width: 80px;
  height: 22px;
  border: none;
  outline: none;
  background: transparent;
  font-size: 13px;
  color: var(--el-text-color-regular);
}
</style>
