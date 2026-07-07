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
          <el-input v-model="res.apiGroups" :placeholder="$t('policy.apiGroups') + ' (a,b)'" />
          <el-input v-model="res.apiVersions" :placeholder="$t('policy.apiVersions') + ' (v1)'" />
          <el-input v-model="res.resources" :placeholder="$t('policy.resourceTypes') + ' (pods)'" />
          <el-button type="danger" plain @click="form.resources.splice(i, 1)">-</el-button>
        </div>
        <el-button plain @click="form.resources.push({ apiGroups: '', apiVersions: '', resources: '' })">+</el-button>
      </div>
    </el-form-item>

    <el-form-item :label="$t('policy.rego')" required>
      <el-input v-model="form.rego" type="textarea" :rows="16" style="font-family: monospace" />
    </el-form-item>

    <el-form-item>
      <el-button @click="onValidate">{{ $t('policy.validate') }}</el-button>
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
import {
  createPolicy, getPolicy, updatePolicy, validateRego,
  type MatchResource, type PolicyRequest,
} from '../api/policy'

const route = useRoute()
const router = useRouter()
const { t } = useI18n()

const isEdit = route.name === 'policy-edit'
const editName = String(route.params.name ?? '')
const saving = ref(false)
let resourceVersion = ''
let source = 'custom'

interface ResourceRow {
  apiGroups: string
  apiVersions: string
  resources: string
}

const form = reactive({
  name: '',
  description: '',
  enforcementMode: 'audit',
  operations: ['CREATE'] as string[],
  resources: [{ apiGroups: '', apiVersions: 'v1', resources: 'pods' }] as ResourceRow[],
  rego: 'package kubesentry\n\ndeny[msg] {\n\t# condition\n\tmsg := "reason"\n}\n',
})

const splitCSV = (s: string) => s.split(',').map((x) => x.trim()).filter((x) => x !== '')
// "" 段表示 core API 组。空串输入 → ['']；非空输入按逗号切分并保留空段
// （如 ",apps" → ['', 'apps']），使 core+具名组混合值可无损往返。
const splitGroups = (s: string) => (s.trim() === '' ? [''] : s.split(',').map((x) => x.trim()))

function toRequest(): PolicyRequest {
  const match = {
    operations: form.operations,
    resources: form.resources.map(
      (r): MatchResource => ({
        apiGroups: splitGroups(r.apiGroups),
        apiVersions: splitCSV(r.apiVersions),
        resources: splitCSV(r.resources),
      }),
    ),
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
  saving.value = true
  try {
    if (isEdit) {
      await updatePolicy(editName, toRequest())
    } else {
      await createPolicy(toRequest())
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
      apiGroups: r.apiGroups.join(','),
      apiVersions: r.apiVersions.join(','),
      resources: r.resources.join(','),
    }))
    form.rego = d.spec.rego
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
})
</script>
