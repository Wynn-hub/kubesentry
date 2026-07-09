<!-- web/src/components/RuleGroupEditor.vue -->
<template>
  <el-card style="margin-bottom: 12px">
    <template #header>
      <div style="display: flex; gap: 8px; align-items: center">
        <span>{{ $t('ruleBuilder.group') }}</span>
        <el-select
          v-if="resourceOptions.length > 1"
          v-model="modelValue.targetResource" style="width: 160px"
          :placeholder="$t('ruleBuilder.targetResource')"
        >
          <el-option v-for="r in resourceOptions" :key="r" :value="r" :label="r" />
        </el-select>
        <el-button type="danger" plain style="margin-left: auto" @click="$emit('remove')">
          {{ $t('ruleBuilder.removeGroup') }}
        </el-button>
      </div>
    </template>

    <div v-for="(cond, i) in modelValue.conditions" :key="i" style="display: flex; gap: 8px; margin-bottom: 8px">
      <ConditionEditor
        v-model="modelValue.conditions[i]" v-model:leaf-type="leafTypes[i]"
        :group="targetGroupVersion?.group ?? ''" :version="targetGroupVersion?.version ?? 'v1'"
        :resource="effectiveResource"
      />
      <el-button type="danger" plain @click="modelValue.conditions.splice(i, 1); leafTypes.splice(i, 1)">-</el-button>
    </div>
    <el-button plain @click="addCondition">{{ $t('ruleBuilder.addCondition') }}</el-button>

    <el-form-item :label="$t('ruleBuilder.message')" style="margin-top: 12px">
      <el-input v-model="modelValue.message" />
    </el-form-item>

    <pre style="background: var(--el-fill-color-light); padding: 8px; overflow-x: auto">{{ preview }}</pre>
  </el-card>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import ConditionEditor from './ConditionEditor.vue'
import { ruleGroupToRego, type FieldLeafType, type RuleGroup } from './ruleGroupCodegen'
import { findGroupVersionForResource } from './fieldTree'
import type { MatchResource } from '../api/policy'

const props = defineProps<{ resources: MatchResource[]; index: number }>()
defineEmits<{ remove: [] }>()
const modelValue = defineModel<RuleGroup>({ required: true })

const leafTypes = ref<FieldLeafType[]>(modelValue.value.conditions.map(() => 'string'))

const resourceOptions = computed(() => Array.from(new Set(props.resources.flatMap((r) => r.resources))))
const effectiveResource = computed(() => modelValue.value.targetResource ?? resourceOptions.value[0] ?? '')
const targetGroupVersion = computed(() => findGroupVersionForResource(props.resources, effectiveResource.value))

function addCondition() {
  modelValue.value.conditions.push({ path: [], operator: 'exists' })
  leafTypes.value.push('string')
}

const preview = computed(() => {
  try {
    return ruleGroupToRego(modelValue.value, props.index)
  } catch (e) {
    return `(${(e as Error).message})`
  }
})
</script>
