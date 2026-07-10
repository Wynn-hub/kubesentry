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

const props = defineProps<{ group: string; version: string; resource: string; path?: PathSegment[] }>()
// path 和 leafType 通过同一次选择产生（leafType 由 path 推导而来），必须合并成
// 一个事件一起发出——分两次 emit 会让消费方对同一个 defineModel 值做两次连续
// 的读-改-写：defineModel 的 prop 要等父组件重新渲染才会反映第一次写入，第二
// 次的读取会读到写入前的旧值，把第一次的更新覆盖掉。
const emit = defineEmits<{
  'update:field': [{ path: PathSegment[]; leafType: FieldLeafType }]
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
    restoreFromProps()
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}

// restoreFromProps 把已保存条件里的字段路径回填进级联选择器。编辑一个用可视化
// 模式创建的策略时，annotation 恢复出来的 path 只存在于 condition 数据上，而
// 级联选择器的选中状态是本地 state——不回填的话编辑页会显示"请选择"，看起来
// 像字段丢了。只在 schema 加载完成且尚无选中值时执行，用户后续的选择不受影响。
function restoreFromProps() {
  if (selectedPath.value.length > 0 || !props.path || props.path.length === 0) return
  const values = props.path.map((s) => s.field)
  try {
    cascaderPathToSegments(values, options.value)
  } catch {
    return // 资源 schema 已变化，旧路径不存在了，保持未选中让用户重选
  }
  selectedPath.value = values
  mapKey.value = props.path.at(-1)?.mapKey ?? ''
  emitPath()
}

function emitPath() {
  if (selectedPath.value.length === 0) return
  const segments = cascaderPathToSegments(selectedPath.value, options.value)
  if (segments.at(-1)?.isMap) {
    segments[segments.length - 1] = { ...segments.at(-1)!, mapKey: mapKey.value }
  }
  emit('update:field', { path: segments, leafType: resolveLeafType(selectedPath.value, options.value) })
}

function onChange() {
  // 只有离开 map 字段时才清空已填的 key——如果新选中的仍是（同一个或另一个）
  // map 字段，保留用户已经打的 key 更符合直觉，之前误把它当成"每次 change
  // 都清空"，导致重新点选同一个字段时 key 会莫名消失。
  if (!lastNodeIsMap.value) mapKey.value = ''
  emitPath()
}
function onMapKeyInput() {
  emitPath()
}

watch(() => [props.group, props.version, props.resource], () => load(), { immediate: true })
</script>
