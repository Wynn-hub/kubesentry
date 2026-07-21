<template>
  <div style="width: 100%">
    <div v-for="(row, i) in rows" :key="i" style="display: flex; gap: 8px; margin-bottom: 8px; align-items: center">
      <el-input v-model="row.key" placeholder="key" style="width: 160px; flex-shrink: 0" @change="emitValue" />
      <el-input v-model="row.value" placeholder="value" style="flex: 1" @change="emitValue" />
      <el-button type="danger" plain @click="rows.splice(i, 1); emitValue()">-</el-button>
    </div>
    <el-button plain @click="rows.push({ key: '', value: '' })">+</el-button>
  </div>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'

const props = defineProps<{ modelValue: Record<string, string> }>()
const emit = defineEmits<{ 'update:modelValue': [Record<string, string>] }>()

const rows = ref<{ key: string; value: string }[]>([])

// Same echo-suppression technique as LabelEditor.vue: emitValue() drops
// half-filled rows (empty key) from the emitted map, but that filtered
// result flows back in through props.modelValue via v-model — without
// this guard, the watch below would rebuild `rows` from the filtered
// value and wipe out whatever half-filled row the user was still typing.
let lastEmitted: string | null = null

watch(
  () => props.modelValue,
  (v) => {
    const serialized = JSON.stringify(v ?? {})
    if (serialized === lastEmitted) return
    rows.value = Object.entries(v ?? {}).map(([key, value]) => ({ key, value }))
  },
  { immediate: true },
)

function emitValue() {
  const out: Record<string, string> = {}
  for (const r of rows.value) {
    if (r.key.trim() !== '') out[r.key.trim()] = r.value
  }
  lastEmitted = JSON.stringify(out)
  emit('update:modelValue', out)
}
</script>
