<!-- web/src/views/PolicyFormView.vue -->
<template>
  <el-form :model="form" label-width="140px" style="max-width: 900px">
    <el-form-item :label="$t('common.name')" required>
      <el-input v-model="form.name" :disabled="isEdit" />
    </el-form-item>
    <el-form-item :label="$t('common.description')">
      <el-input v-model="form.description" type="textarea" :rows="2" />
    </el-form-item>
    <el-form-item :label="$t('policy.mode')" required>
      <el-radio-group v-model="form.enforcementMode">
        <el-radio value="enforce">{{ $t('policy.enforce') }}</el-radio>
        <el-radio value="audit">{{ $t('policy.audit') }}</el-radio>
      </el-radio-group>
    </el-form-item>
    <el-form-item :label="$t('policy.operations')" required>
      <el-checkbox-group v-model="form.operations">
        <el-checkbox v-for="op in ['CREATE', 'UPDATE', 'DELETE']" :key="op" :value="op">{{ op }}</el-checkbox>
      </el-checkbox-group>
    </el-form-item>

    <el-form-item :label="$t('policy.resources')" required>
      <div style="width: 100%">
        <div v-for="(res, i) in form.resources" :key="i" style="display: flex; gap: 8px; margin-bottom: 8px">
          <el-select
            v-model="res.apiGroup" filterable clearable allow-create default-first-option
            :placeholder="$t('policy.apiGroups')" style="flex: 1"
          >
            <el-option :value="CORE_GROUP" :label="$t('policy.coreGroup')" />
            <el-option v-for="g in suggestions.apiGroups.filter((x) => x !== '')" :key="g" :label="g" :value="g" />
          </el-select>
          <el-select
            v-model="res.apiVersion" filterable clearable allow-create default-first-option
            :placeholder="$t('policy.apiVersions')" style="flex: 1"
          >
            <el-option v-for="v in suggestions.apiVersions" :key="v" :label="v" :value="v" />
          </el-select>
          <el-select
            v-model="res.resource" filterable clearable allow-create default-first-option
            :placeholder="$t('policy.resourceTypes')" style="flex: 1"
          >
            <el-option v-for="rsc in resourceOptionsFor(res.apiGroup)" :key="rsc" :label="rsc" :value="rsc" />
          </el-select>
          <el-button type="danger" plain @click="form.resources.splice(i, 1)">-</el-button>
        </div>
        <el-button plain @click="form.resources.push({ apiGroup: null, apiVersion: null, resource: null })">+</el-button>
      </div>
    </el-form-item>

    <el-form-item :label="$t('ruleBuilder.buildMode')">
      <el-radio-group v-model="builderMode" @change="onBuilderModeChange">
        <el-radio value="manual">{{ $t('ruleBuilder.modeManual') }}</el-radio>
        <el-radio value="visual">{{ $t('ruleBuilder.modeVisual') }}</el-radio>
      </el-radio-group>
    </el-form-item>

    <el-form-item v-if="builderMode === 'manual'" :label="$t('policy.rego')" required>
      <el-input v-model="form.rego" type="textarea" :rows="16" style="font-family: monospace" />
    </el-form-item>

    <el-form-item v-else :label="$t('ruleBuilder.group')" required>
      <div style="width: 100%">
        <RuleGroupEditor
          v-for="(g, i) in visualGroups" :key="i"
          v-model="visualGroups[i]" :index="i"
          :resources="matchResources"
          @remove="visualGroups.splice(i, 1)"
        />
        <el-button plain @click="visualGroups.push({ conditions: [], message: '' })">
          {{ $t('ruleBuilder.addGroup') }}
        </el-button>
      </div>
    </el-form-item>

    <el-form-item>
      <el-button @click="onValidate">{{ $t('policy.validate') }}</el-button>
      <el-button type="primary" :loading="saving" @click="onSave">{{ $t('common.save') }}</el-button>
      <el-button @click="$router.back()">{{ $t('common.cancel') }}</el-button>
    </el-form-item>
  </el-form>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useI18n } from 'vue-i18n'
import { ApiError } from '../api/http'
import {
  createPolicy, getPolicy, getResourceSuggestions, updatePolicy, validateRego,
  type MatchResource, type PolicyRequest,
} from '../api/policy'
import RuleGroupEditor from '../components/RuleGroupEditor.vue'
import {
  deserializeRuleGroups, ruleGroupsToRego, serializeRuleGroups, VISUAL_BUILDER_ANNOTATION,
  type RuleGroup,
} from '../components/ruleGroupCodegen'

const route = useRoute()
const router = useRouter()
const { t } = useI18n()

const isEdit = route.name === 'policy-edit'
const editName = String(route.params.name ?? '')
const saving = ref(false)
let resourceVersion = ''
let source = 'custom'

interface ResourceRow {
  apiGroup: string | null
  apiVersion: string | null
  resource: string | null
}

const suggestions = reactive({
  apiGroups: [] as string[],
  apiVersions: [] as string[],
  resources: [] as string[],
  resourcesByGroup: {} as Record<string, string[]>,
})

// el-select 的单选模式把 modelValue === '' 当成"未选择"处理，即使选项存在也
// 只显示 placeholder（core 组的原始 apiGroup 值恰好就是空字符串）——用一个
// 哨兵值区分"选中了 core 组"和"什么都没选"，只在读写后端数据的边界上转换回
// 真正的空字符串。
const CORE_GROUP = '__core__'

// resourceOptionsFor 把资源类型下拉的候选值限制在已选资源组范围内，避免出现
// "选了 apps 组却能选到只属于 core 组的资源" 这种组合会在保存时报错的情况；
// 未选资源组时退回展示全部候选值。
function resourceOptionsFor(apiGroup: string | null): string[] {
  if (apiGroup === null) return suggestions.resources
  const raw = apiGroup === CORE_GROUP ? '' : apiGroup
  return suggestions.resourcesByGroup[raw] ?? []
}

const form = reactive({
  name: '',
  description: '',
  enforcementMode: 'audit',
  operations: ['CREATE'] as string[],
  resources: [{ apiGroup: null, apiVersion: null, resource: null }] as ResourceRow[],
  rego: 'package kubesentry\n\ndeny[msg] {\n\t# condition\n\tmsg := "reason"\n}\n',
})

// matchResources 把单选的 ResourceRow 包装成后端 MatchResource 需要的数组形态
// （webhook 规则允许一条 match 里填多个 group/version/resource），也是
// RuleGroupEditor 解析可视化条件时用来查 GVK 的数据源。
const matchResources = computed<MatchResource[]>(() =>
  form.resources.map((r) => ({
    apiGroups: r.apiGroup !== null ? [r.apiGroup === CORE_GROUP ? '' : r.apiGroup] : [],
    apiVersions: r.apiVersion !== null ? [r.apiVersion] : [],
    resources: r.resource !== null ? [r.resource] : [],
  })),
)

const builderMode = ref<'manual' | 'visual'>('visual')
const visualGroups = ref<RuleGroup[]>([{ conditions: [], message: '' }])

async function onBuilderModeChange(newMode: string | number | boolean) {
  const mode = newMode as 'manual' | 'visual'
  const hasManualContent = form.rego.trim().length > 0 &&
    form.rego !== 'package kubesentry\n\ndeny[msg] {\n\t# condition\n\tmsg := "reason"\n}\n'
  const hasVisualContent = visualGroups.value.some((g) => g.conditions.length > 0 || g.message !== '')
  const needsConfirm = (mode === 'visual' && hasManualContent) || (mode === 'manual' && hasVisualContent)

  if (needsConfirm) {
    try {
      await ElMessageBox.confirm(t('ruleBuilder.switchConfirm'))
    } catch {
      builderMode.value = mode === 'visual' ? 'manual' : 'visual' // 用户取消，改回原模式
      return
    }
  }
  if (mode === 'visual') form.rego = ''
  else visualGroups.value = [{ conditions: [], message: '' }]
}

function toRequest(): PolicyRequest {
  const match = {
    operations: form.operations,
    resources: matchResources.value,
  }
  return {
    name: isEdit ? undefined : form.name,
    description: form.description,
    enforcementMode: form.enforcementMode,
    match,
    rego: form.rego,
    resourceVersion: isEdit ? resourceVersion : undefined,
  }
}

async function onValidate() {
  try {
    await validateRego(form.rego)
    ElMessage.success(t('policy.validateOK'))
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}

async function onSave() {
  if (source === 'builtin') {
    try {
      await ElMessageBox.confirm(t('common.builtinWarning'))
    } catch {
      return
    }
  }

  if (builderMode.value === 'visual') {
    const emptyGroupIndex = visualGroups.value.findIndex((g) => g.conditions.length === 0)
    if (emptyGroupIndex !== -1) {
      ElMessage.error(t('ruleBuilder.groupEmpty', { n: emptyGroupIndex + 1 }))
      return
    }
  }

  let rego: string
  try {
    rego = builderMode.value === 'visual' ? ruleGroupsToRego(visualGroups.value) : form.rego
  } catch (e) {
    ElMessage.error((e as Error).message)
    return
  }

  try {
    await validateRego(rego)
  } catch (e) {
    ElMessage.error(t('ruleBuilder.compileError', { msg: (e as Error).message }))
    return
  }
  form.rego = rego

  saving.value = true
  try {
    const req = toRequest()
    req.annotations = {
      [VISUAL_BUILDER_ANNOTATION]: builderMode.value === 'visual' ? serializeRuleGroups(visualGroups.value) : '',
    }
    if (isEdit) {
      await updatePolicy(editName, req)
    } else {
      await createPolicy(req)
    }
    ElMessage.success(t('common.saved'))
    router.push('/policies')
  } catch (e) {
    if (e instanceof ApiError && e.status === 409) {
      ElMessage.error(t('common.conflictRefresh'))
    } else {
      ElMessage.error((e as Error).message)
    }
  } finally {
    saving.value = false
  }
}

onMounted(async () => {
  try {
    const s = await getResourceSuggestions()
    suggestions.apiGroups = s.apiGroups
    suggestions.apiVersions = s.apiVersions
    suggestions.resources = s.resources
    suggestions.resourcesByGroup = s.resourcesByGroup
  } catch {
    // 建议列表拉取失败不阻塞表单，用户仍可用 allow-create 手动输入
  }
  if (!isEdit) return
  try {
    const d = await getPolicy(editName)
    resourceVersion = d.resourceVersion
    source = d.source
    form.name = d.name
    form.description = d.spec.description ?? ''
    form.enforcementMode = d.spec.enforcementMode
    form.operations = d.spec.match.operations
    form.resources = d.spec.match.resources.map((r) => ({
      apiGroup: r.apiGroups[0] === undefined ? null : r.apiGroups[0] === '' ? CORE_GROUP : r.apiGroups[0],
      apiVersion: r.apiVersions[0] ?? null,
      resource: r.resources[0] ?? null,
    }))
    form.rego = d.spec.rego

    const savedGroups = d.annotations?.[VISUAL_BUILDER_ANNOTATION]
      ? deserializeRuleGroups(d.annotations[VISUAL_BUILDER_ANNOTATION])
      : null
    if (savedGroups) {
      visualGroups.value = savedGroups
      builderMode.value = 'visual'
    } else {
      builderMode.value = 'manual'
    }
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
})
</script>
