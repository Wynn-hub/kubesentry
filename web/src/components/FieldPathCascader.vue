<!-- web/src/components/FieldPathCascader.vue -->
<template>
  <div style="display: flex; gap: 8px; align-items: center; flex: 1">
    <el-cascader
      v-model="selectedPath"
      :options="options"
      :props="{ expandTrigger: 'hover', checkStrictly: true }"
      style="flex: 1"
      @change="onChange"
    />
    <el-input
      v-if="lastNodeIsMap"
      v-model="mapKey"
      :placeholder="$t('ruleBuilder.mapKeyPlaceholder')"
      style="width: 160px"
      @input="onMapKeyInput"
    />
    <el-button :icon="Refresh" circle @click="load(true)" />
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { Refresh } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import { getFieldSchema } from '../api/schema'
import {
  cascaderPathToSegments, fieldTreeToCascaderOptions, resolveLeafType,
  type CascaderFieldOption,
} from './fieldTree'
import type { FieldLeafType, PathSegment } from './ruleGroupCodegen'

const props = defineProps<{ group: string; version: string; resource: string }>()
const emit = defineEmits<{
  'update:path': [PathSegment[]]
  'update:leafType': [FieldLeafType]
}>()

const options = ref<CascaderFieldOption[]>([])
const selectedPath = ref<string[]>([])
const mapKey = ref('')

const lastNodeIsMap = computed(() => {
  if (selectedPath.value.length === 0) return false
  try {
    return cascaderPathToSegments(selectedPath.value, options.value).at(-1)?.isMap === true
  } catch {
    return false
  }
})

async function load(refresh = false) {
  if (!props.version || !props.resource) return
  try {
    options.value = fieldTreeToCascaderOptions(await getFieldSchema(props.group, props.version, props.resource, refresh))
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}

function emitPath() {
  if (selectedPath.value.length === 0) return
  const segments = cascaderPathToSegments(selectedPath.value, options.value)
  if (segments.at(-1)?.isMap) {
    segments[segments.length - 1] = { ...segments.at(-1)!, mapKey: mapKey.value }
  }
  emit('update:path', segments)
  emit('update:leafType', resolveLeafType(selectedPath.value, options.value))
}

function onChange() {
  mapKey.value = ''
  emitPath()
}
function onMapKeyInput() {
  emitPath()
}

watch(() => [props.group, props.version, props.resource], () => load(), { immediate: true })
</script>
