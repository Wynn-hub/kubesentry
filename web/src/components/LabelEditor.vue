<template>
  <div style="width: 100%">
    <div v-for="(row, i) in rows" :key="i" style="display: flex; gap: 8px; margin-bottom: 8px">
      <el-input v-model="row.key" placeholder="key" @change="emitValue" />
      <el-input v-model="row.value" placeholder="value" @change="emitValue" />
      <el-button type="danger" plain @click="rows.splice(i, 1); emitValue()">-</el-button>
    </div>
    <el-button plain @click="rows.push({ key: '', value: '' })">+</el-button>
  </div>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'

const props = defineProps<{ modelValue?: Record<string, string> | null }>()
const emit = defineEmits<{ 'update:modelValue': [Record<string, string>] }>()

const rows = ref<{ key: string; value: string }[]>([])

watch(
  () => props.modelValue,
  (v) => {
    rows.value = Object.entries(v ?? {}).map(([key, value]) => ({ key, value }))
  },
  { immediate: true },
)

function emitValue() {
  const out: Record<string, string> = {}
  for (const r of rows.value) if (r.key.trim() !== '') out[r.key.trim()] = r.value
  emit('update:modelValue', out)
}
</script>
