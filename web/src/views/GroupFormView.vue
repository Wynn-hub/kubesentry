<template>
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

    <el-form-item :label="$t('group.members')">
      <el-transfer
        v-model="selectedNames"
        :data="allPolicies"
        :titles="[$t('policy.title'), $t('group.members')]"
        filterable
      />
    </el-form-item>
    <el-form-item v-if="selectedNames.length" label=" ">
      <el-table :data="selectedNames" size="small" style="width: 480px">
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
    </el-form-item>

    <el-form-item :label="$t('group.bySelector')">
      <LabelEditor v-model="bySelectorLabels" />
    </el-form-item>
    <el-form-item :label="$t('group.selectorMode')">
      <el-select v-model="form.selectorEnforcementMode" clearable :placeholder="$t('group.followPolicy')" style="width: 200px">
        <el-option :label="$t('policy.enforce')" value="enforce" />
        <el-option :label="$t('policy.audit')" value="audit" />
      </el-select>
    </el-form-item>
    <el-form-item :label="$t('group.nsSelector')">
      <LabelEditor v-model="nsSelectorLabels" />
    </el-form-item>

    <el-form-item>
      <el-button type="primary" :loading="saving" @click="onSave">{{ $t('common.save') }}</el-button>
      <el-button @click="$router.back()">{{ $t('common.cancel') }}</el-button>
    </el-form-item>
  </el-form>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useI18n } from 'vue-i18n'
import { ApiError } from '../api/http'
import { createGroup, getGroup, updateGroup, type GroupRequest } from '../api/group'
import { listPolicies } from '../api/policy'
import LabelEditor from '../components/LabelEditor.vue'
import { computeMembership } from '../components/groupMembership'

const route = useRoute()
const router = useRouter()
const { t } = useI18n()

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
  selectorEnforcementMode: '',
})
const allPolicies = ref<{ key: string; label: string }[]>([])
const selectedNames = ref<string[]>([])
const modeOverrides = reactive<Record<string, string>>({})
const bySelectorLabels = ref<Record<string, string>>({})
const nsSelectorLabels = ref<Record<string, string>>({})

function toRequest(): GroupRequest {
  const hasSelector = Object.keys(bySelectorLabels.value).length > 0
  const hasNs = Object.keys(nsSelectorLabels.value).length > 0
  const { byName, exclude } = computeMembership({
    initialResolvedNames,
    initialByNameNames,
    initialExclude,
    finalSelectedNames: selectedNames.value,
    modeOverrides,
  })
  return {
    name: isEdit ? undefined : form.name,
    displayName: form.displayName,
    description: form.description,
    enabled: form.enabled,
    namespaceSelector: hasNs ? { matchLabels: nsSelectorLabels.value } : null,
    policies: {
      byName,
      bySelector: hasSelector ? { matchLabels: bySelectorLabels.value } : undefined,
      exclude: exclude.length ? exclude : undefined,
    },
    selectorEnforcementMode: form.selectorEnforcementMode || undefined,
    resourceVersion: isEdit ? resourceVersion : undefined,
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
    allPolicies.value = (await listPolicies()).map((p) => ({ key: p.name, label: p.name }))
    if (!isEdit) return
    const d = await getGroup(editName)
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
    bySelectorLabels.value = d.spec.policies?.bySelector?.matchLabels ?? {}
    nsSelectorLabels.value = d.spec.namespaceSelector?.matchLabels ?? {}
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
})
</script>
