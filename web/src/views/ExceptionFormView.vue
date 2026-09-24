<template>
  <PageHeader
    show-back
    :title="$t(isEdit ? 'common.editObject' : 'common.createObject', { name: $t('exception.title') })"
  />
  <el-form label-width="180px" style="max-width: 900px">
    <el-form-item :label="$t('common.name')" required>
      <el-input v-model="form.name" :disabled="isEdit" />
    </el-form-item>
    <el-form-item :label="$t('exception.targetClusters')" required>
      <el-switch v-model="allClusters" :disabled="isDefault" :active-text="$t('exception.allClusters')" :inactive-text="$t('exception.selectedClusters')" />
      <span v-if="isDefault" style="margin-left: 12px; color: var(--el-text-color-secondary)">{{ $t('exception.defaultProtectedHint') }}</span>
      <el-select v-if="!allClusters && !isDefault" v-model="clusterNames" multiple filterable style="width: 100%; margin-top: 8px" :placeholder="$t('exception.selectClusters')">
        <el-option v-for="name in availableClusters" :key="name" :label="name" :value="name" />
      </el-select>
    </el-form-item>

    <el-form-item :label="$t('exception.target')" required>
      <el-radio-group v-model="targetKind" :disabled="isDefault">
        <el-radio value="policies">{{ $t('exception.targetPolicies') }}</el-radio>
        <el-radio value="groups">{{ $t('exception.targetGroups') }}</el-radio>
        <el-radio value="all">{{ $t('exception.targetAll') }}</el-radio>
      </el-radio-group>
    </el-form-item>
    <el-form-item v-if="targetKind === 'policies' && !isDefault" label=" ">
      <el-select v-model="form.policyRefs" multiple filterable style="width: 100%">
        <el-option v-for="p in policyNames" :key="p" :label="p" :value="p" />
      </el-select>
    </el-form-item>
    <el-form-item v-if="targetKind === 'groups' && !isDefault" label=" ">
      <el-select v-model="form.policyGroupRefs" multiple filterable style="width: 100%">
        <el-option v-for="g in groupNames" :key="g" :label="g" :value="g" />
      </el-select>
    </el-form-item>

    <el-form-item :label="$t('exception.nsTarget')" required>
      <div style="width: 100%">
        <el-tabs v-model="nsMode">
          <el-tab-pane :label="$t('group.nsModeName')" name="name">
            <TagInput v-model="namespaceNames" :placeholder="$t('group.nsNamePlaceholder')" />
            <HintMark :content="$t('common.multiValueOrHint')" />
          </el-tab-pane>
          <el-tab-pane :label="$t('group.nsModeLabel')" name="label">
            <LabelEditor v-model="nsSelector" />
          </el-tab-pane>
        </el-tabs>
        <p style="color: var(--el-text-color-secondary); margin: 4px 0 0; font-size: 13px">
          {{ $t('exception.nsAllHint') }}
        </p>
      </div>
    </el-form-item>
    <el-form-item v-if="!isDefault" :label="$t('exception.resSelector')">
      <div style="width: 100%">
        <LabelEditor v-model="resourceSelector" />
        <p style="color: var(--el-text-color-secondary); margin: 4px 0 0; font-size: 13px">
          {{ $t('exception.resSelectorHint') }}
        </p>
      </div>
    </el-form-item>

    <el-form-item :label="$t('exception.duration')" required>
      <span v-if="isDefault">{{ $t('exception.permanent') }}</span>
      <el-input v-else v-model="form.duration" placeholder="24h / 30m / 168h" style="width: 200px" />
    </el-form-item>
    <el-form-item v-if="!isDefault" :label="$t('exception.retain')">
      <el-input v-model="form.retainAfterExpiry" placeholder="0s" style="width: 200px" />
    </el-form-item>
    <el-form-item :label="$t('exception.reason')" required>
      <el-input v-model="form.reason" type="textarea" :rows="2" />
    </el-form-item>

    <el-form-item>
      <el-button type="primary" :loading="saving" @click="onSave">{{ $t('common.save') }}</el-button>
      <el-button @click="goBack">{{ $t('common.cancel') }}</el-button>
    </el-form-item>
  </el-form>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { useI18n } from 'vue-i18n'
import { ApiError } from '../api/http'
import { collectAllPages } from '../api/pagination'
import {
  createException, getException, updateException, type ExceptionRequest,
} from '../api/exception'
import { listPolicies } from '../api/policy'
import { listGroups, type LabelSelector } from '../api/group'
import { listClusters } from '../api/cluster'
import LabelEditor from '../components/LabelEditor.vue'
import TagInput from '../components/TagInput.vue'
import HintMark from '../components/HintMark.vue'
import PageHeader from '../components/PageHeader.vue'
import { useBackNavigation } from '../composables/useBackNavigation'

const route = useRoute()
const router = useRouter()
const { t } = useI18n()
const { goBack } = useBackNavigation()

const isEdit = route.name === 'exception-edit'
const editName = String(route.params.name ?? '')
const isDefault = isEdit && editName === 'default-system-namespaces'
const saving = ref(false)
let resourceVersion = ''

const targetKind = ref<'policies' | 'groups' | 'all'>('policies')
const policyNames = ref<string[]>([])
const groupNames = ref<string[]>([])
// 命名空间目标：按名称或按标签二选一，两者是同一次匹配的两种录入方式，
// 不像策略成员的 byName/bySelector 那样可以并存——命名空间名称列表和
// namespaceSelector 是后端两个独立字段，若同时下发，语义是"名字在列表里
// 且标签也匹配"的交集，不是"任选一种途径"的并集，所以只提交当前 tab。
const nsMode = ref<'name' | 'label'>('name')
const namespaceNames = ref<string[]>([])
const nsSelector = ref<LabelSelector | null>(null)
const resourceSelector = ref<LabelSelector | null>(null)
const allClusters = ref(true)
const clusterNames = ref<string[]>([])
const availableClusters = ref<string[]>([])

const form = reactive({
  name: '',
  policyRefs: [] as string[],
  policyGroupRefs: [] as string[],
  duration: '24h',
  retainAfterExpiry: '',
  reason: '',
})

function toRequest(): ExceptionRequest {
  return {
    name: isEdit ? undefined : form.name,
    policyRefs: !isDefault && targetKind.value === 'policies' ? form.policyRefs : undefined,
    policyGroupRefs: !isDefault && targetKind.value === 'groups' ? form.policyGroupRefs : undefined,
    allPolicies: isDefault || targetKind.value === 'all' ? true : undefined,
    match: {
      namespaces: nsMode.value === 'name' && namespaceNames.value.length ? namespaceNames.value : undefined,
      namespaceSelector: nsMode.value === 'label' ? (nsSelector.value ?? undefined) : undefined,
      resourceSelector: isDefault ? undefined : (resourceSelector.value ?? undefined),
    },
    duration: form.duration,
    retainAfterExpiry: form.retainAfterExpiry || undefined,
    reason: form.reason,
    resourceVersion: isEdit ? resourceVersion : undefined,
    allClusters: isDefault ? true : allClusters.value,
    clusterNames: isDefault || allClusters.value ? undefined : clusterNames.value,
  }
}

async function onSave() {
  if (!isDefault && !allClusters.value && clusterNames.value.length === 0) { ElMessage.error(t('exception.selectClusters')); return }
  saving.value = true
  try {
    if (isEdit) {
      await updateException(editName, toRequest())
    } else {
      await createException(toRequest())
    }
    ElMessage.success(t('common.saved'))
    router.push('/exceptions')
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
    policyNames.value = (await collectAllPages(listPolicies)).map((p) => p.name)
    groupNames.value = (await collectAllPages(listGroups)).map((g) => g.name)
    availableClusters.value = (await collectAllPages(listClusters)).filter((c) => c.phase !== 'Uninstalled').map((c) => c.name)
    if (!isEdit) return
    const d = await getException(editName)

    allClusters.value = d.scope.allClusters
    clusterNames.value = d.scope.clusterNames ?? []
    resourceVersion = d.resourceVersion
    form.name = d.name
    form.duration = d.spec.duration
    form.retainAfterExpiry = d.spec.retainAfterExpiry ?? ''
    form.reason = d.spec.reason
    form.policyRefs = d.spec.policyRefs ?? []
    form.policyGroupRefs = d.spec.policyGroupRefs ?? []
    targetKind.value = d.spec.allPolicies ? 'all' : d.spec.policyGroupRefs?.length ? 'groups' : 'policies'
    namespaceNames.value = d.spec.match.namespaces ?? []
    nsSelector.value = d.spec.match.namespaceSelector ?? null
    nsMode.value = namespaceNames.value.length ? 'name' : 'label'
    resourceSelector.value = d.spec.match.resourceSelector ?? null
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
})
</script>
