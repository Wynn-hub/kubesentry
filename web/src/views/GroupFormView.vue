<template>
  <PageHeader
    show-back
    :title="$t(isEdit ? 'common.editObject' : 'common.createObject', { name: $t('group.title') })"
  />
  <el-form label-width="180px" style="max-width: 1000px">
    <el-form-item :label="$t('common.name')" required>
      <el-input v-model="form.name" :disabled="isEdit" />
    </el-form-item>
    <el-form-item :label="$t('group.displayName')">
      <el-input v-model="form.displayName" />
    </el-form-item>
    <el-form-item :label="$t('common.description')">
      <el-input v-model="form.description" type="textarea" :rows="2" />
    </el-form-item>
    <el-form-item :label="$t('group.enabled')">
      <el-switch v-model="form.enabled" />
    </el-form-item>
    <el-form-item :label="$t('group.targetClusters')" required>
      <el-switch v-model="allClusters" :active-text="$t('group.allClusters')" :inactive-text="$t('group.selectedClusters')" />
      <el-select v-if="!allClusters" v-model="clusterNames" multiple filterable style="width: 100%; margin-top: 8px" :placeholder="$t('group.selectClusters')">
        <el-option v-for="name in availableClusters" :key="name" :label="name" :value="name" />
      </el-select>
    </el-form-item>

    <el-form-item :label="$t('group.policyMembers')">
      <el-tabs v-model="policyTab" style="width: 100%">
        <el-tab-pane :label="$t('group.members')" name="byName">
          <el-transfer
            v-model="selectedNames"
            :data="allPolicies"
            :titles="[$t('policy.title'), $t('group.members')]"
            filterable
          />
          <el-table v-if="selectedNames.length" :data="selectedPage.items" size="small" style="width: 480px; margin-top: 12px">
            <el-table-column :label="$t('common.name')">
              <template #default="{ row }">{{ row }}</template>
            </el-table-column>
            <el-table-column :label="$t('group.effectiveMode')" width="200">
              <template #default="{ row }">
                <el-select v-model="modeOverrides[row]" clearable :placeholder="$t('group.followPolicy')">
                  <el-option :label="$t('policy.enforce')" value="enforce" />
                  <el-option :label="$t('policy.audit')" value="audit" />
                </el-select>
              </template>
            </el-table-column>
          </el-table>
          <PaginationBar
            v-if="selectedNames.length"
            :total="selectedPage.total"
            :page="selectedState.page"
            :page-size="selectedState.pageSize"
            @update:page="(page) => selectedRouteList.replace({ page })"
            @update:page-size="(pageSize) => selectedRouteList.replace({ pageSize })"
          />
        </el-tab-pane>
        <el-tab-pane :label="$t('group.bySelector')" name="bySelector">
          <LabelEditor v-model="bySelector" />
          <el-form-item :label="$t('group.selectorMode')" label-position="top" style="margin-top: 12px">
            <el-select v-model="form.selectorEnforcementMode" clearable :placeholder="$t('group.followPolicy')" style="width: 200px">
              <el-option :label="$t('policy.enforce')" value="enforce" />
              <el-option :label="$t('policy.audit')" value="audit" />
            </el-select>
          </el-form-item>
        </el-tab-pane>
      </el-tabs>
    </el-form-item>

    <el-form-item :label="$t('group.scope')">
      <el-radio-group v-model="nsScope">
        <el-radio value="all">{{ $t('group.scopeAll') }}</el-radio>
        <el-radio value="specific">{{ $t('group.scopeSpecific') }}</el-radio>
      </el-radio-group>
    </el-form-item>
    <el-form-item v-if="nsScope === 'specific'" :label="$t('group.nsSelector')">
      <div style="width: 100%">
        <el-radio-group v-model="nsMode" style="margin-bottom: 12px">
          <el-radio value="label">{{ $t('group.nsModeLabel') }}</el-radio>
          <el-radio value="name">{{ $t('group.nsModeName') }}</el-radio>
        </el-radio-group>
        <LabelEditor v-if="nsMode === 'label'" v-model="nsSelector" />
        <div v-else style="display: flex; align-items: center">
          <TagInput v-model="nsNameList" :placeholder="$t('group.nsNamePlaceholder')" />
          <HintMark :content="$t('common.multiValueOrHint')" />
        </div>
      </div>
    </el-form-item>

    <el-form-item>
      <el-button type="primary" :loading="saving" @click="onSave">{{ $t('common.save') }}</el-button>
      <el-button @click="goBack">{{ $t('common.cancel') }}</el-button>
    </el-form-item>
  </el-form>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useI18n } from 'vue-i18n'
import { ApiError } from '../api/http'
import { collectAllPages } from '../api/pagination'
import { createGroup, getGroup, updateGroup, type GroupRequest, type LabelSelector, type PolicyRef } from '../api/group'
import { listPolicies } from '../api/policy'
import { listClusters } from '../api/cluster'
import LabelEditor from '../components/LabelEditor.vue'
import HintMark from '../components/HintMark.vue'
import TagInput from '../components/TagInput.vue'
import PageHeader from '../components/PageHeader.vue'
import PaginationBar from '../components/PaginationBar.vue'
import { useBackNavigation } from '../composables/useBackNavigation'
import { useClientPagination } from '../composables/useClientPagination'
import { useRouteListState } from '../composables/useRouteListState'
import { computeMembership } from '../components/groupMembership'

const NS_NAME_KEY = 'kubernetes.io/metadata.name'

function hasSelectorContent(sel: LabelSelector | null | undefined): boolean {
  return !!(Object.keys(sel?.matchLabels ?? {}).length || sel?.matchExpressions?.length)
}

const route = useRoute()
const router = useRouter()
const { t } = useI18n()
const { goBack } = useBackNavigation()

const isEdit = route.name === 'group-edit'
const editName = String(route.params.name ?? '')
const saving = ref(false)
let resourceVersion = ''
let source = 'custom'
let initialResolvedNames: string[] = []
let initialByNameNames: string[] = []
let initialExclude: string[] = []

const form = reactive({
  name: '',
  displayName: '',
  description: '',
  enabled: true,
  selectorEnforcementMode: '' as '' | NonNullable<GroupRequest['selectorEnforcementMode']>,
})
const allPolicies = ref<{ key: string; label: string }[]>([])
const selectedNames = ref<string[]>([])
const selectedRouteList = useRouteListState({
  prefix: 'selected',
  defaultSortBy: 'name',
  defaultSortOrder: 'asc',
  allowedSort: ['name'],
})
const selectedState = selectedRouteList.state
const selectedPage = useClientPagination(
  selectedNames,
  selectedState,
  (a, b) => a.localeCompare(b),
)
watch(
  () => selectedNames.value.length,
  (total) => {
    const lastPage = Math.max(1, Math.ceil(total / selectedState.value.pageSize))
    if (selectedState.value.page > lastPage) void selectedRouteList.replace({ page: lastPage })
  },
)
const modeOverrides = reactive<Record<string, PolicyRef['enforcementMode'] | undefined>>({})
const bySelector = ref<LabelSelector | null>(null)
const policyTab = ref<'byName' | 'bySelector'>('byName')

// 命名空间作用范围：全部 / 指定；指定时再选"按标签"还是"按名称"，两者是
// 同一个 namespaceSelector 字段的两种录入方式，互斥（同时填会变成交集，
// 不是用户想要的"或"语义），所以只提交当前选中的那一种。
const nsScope = ref<'all' | 'specific'>('all')
const nsMode = ref<'label' | 'name'>('label')
const nsSelector = ref<LabelSelector | null>(null)
const nsNameList = ref<string[]>([])
const allClusters = ref(true)
const clusterNames = ref<string[]>([])
const availableClusters = ref<string[]>([])

function toRequest(): GroupRequest {
  const { byName, exclude } = computeMembership({
    initialResolvedNames,
    initialByNameNames,
    initialExclude,
    finalSelectedNames: selectedNames.value,
    modeOverrides,
  })

  let namespaceSelector: LabelSelector | null = null
  if (nsScope.value === 'specific') {
    if (nsMode.value === 'name' && nsNameList.value.length) {
      namespaceSelector = { matchExpressions: [{ key: NS_NAME_KEY, operator: 'In', values: nsNameList.value }] }
    } else if (nsMode.value === 'label') {
      namespaceSelector = nsSelector.value
    }
  }

  return {
    name: isEdit ? undefined : form.name,
    displayName: form.displayName,
    description: form.description,
    enabled: form.enabled,
    namespaceSelector: namespaceSelector ?? undefined,
    policies: {
      byName,
      bySelector: bySelector.value ?? undefined,
      exclude: exclude.length ? exclude : undefined,
    },
    selectorEnforcementMode: form.selectorEnforcementMode || undefined,
    resourceVersion: isEdit ? resourceVersion : undefined,
    allClusters: allClusters.value,
    clusterNames: allClusters.value ? undefined : clusterNames.value,
  }
}

async function onSave() {
  if (!allClusters.value && clusterNames.value.length === 0) { ElMessage.error(t('group.selectClusters')); return }
  if (source === 'builtin') {
    try {
      await ElMessageBox.confirm(t('common.builtinWarning'))
    } catch {
      return
    }
  }
  saving.value = true
  try {
    if (isEdit) {
      await updateGroup(editName, toRequest())
    } else {
      await createGroup(toRequest())
    }
    ElMessage.success(t('common.saved'))
    router.push('/policygroups')
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
    allPolicies.value = (await collectAllPages(listPolicies)).map((p) => ({ key: p.name, label: p.name }))
    availableClusters.value = (await collectAllPages(listClusters)).filter((c) => c.phase !== 'Uninstalled').map((c) => c.name)
    if (!isEdit) return
    const d = await getGroup(editName)

    allClusters.value = d.scope.allClusters
    clusterNames.value = d.scope.clusterNames ?? []
    resourceVersion = d.resourceVersion
    source = d.source
    form.name = d.name
    form.displayName = d.spec.displayName ?? ''
    form.description = d.spec.description ?? ''
    form.enabled = d.spec.enabled
    form.selectorEnforcementMode = d.spec.selectorEnforcementMode ?? ''
    selectedNames.value = (d.status.resolvedPolicies ?? []).map((m) => m.name)
    initialResolvedNames = selectedNames.value.slice()
    initialByNameNames = (d.spec.policies?.byName ?? []).map((r) => r.name)
    initialExclude = d.spec.policies?.exclude ?? []
    for (const r of d.spec.policies?.byName ?? []) {
      if (r.enforcementMode) modeOverrides[r.name] = r.enforcementMode
    }
    bySelector.value = d.spec.policies?.bySelector ?? null
    if (hasSelectorContent(bySelector.value)) policyTab.value = 'bySelector'

    const ns = d.spec.namespaceSelector
    const nameExpr = ns?.matchExpressions?.find((e) => e.key === NS_NAME_KEY && e.operator === 'In')
    if (nameExpr) {
      nsScope.value = 'specific'
      nsMode.value = 'name'
      nsNameList.value = nameExpr.values ?? []
    } else if (hasSelectorContent(ns)) {
      nsScope.value = 'specific'
      nsMode.value = 'label'
      nsSelector.value = ns ?? null
    }
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
})
</script>
