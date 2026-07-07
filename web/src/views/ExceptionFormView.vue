<template>
  <el-form label-width="180px" style="max-width: 900px">
    <el-form-item :label="$t('common.name')" required>
      <el-input v-model="form.name" :disabled="isEdit" />
    </el-form-item>

    <el-form-item :label="$t('exception.target')" required>
      <el-radio-group v-model="targetKind">
        <el-radio value="policies">{{ $t('exception.targetPolicies') }}</el-radio>
        <el-radio value="groups">{{ $t('exception.targetGroups') }}</el-radio>
        <el-radio value="all">{{ $t('exception.targetAll') }}</el-radio>
      </el-radio-group>
    </el-form-item>
    <el-form-item v-if="targetKind === 'policies'" label=" ">
      <el-select v-model="form.policyRefs" multiple filterable style="width: 100%">
        <el-option v-for="p in policyNames" :key="p" :label="p" :value="p" />
      </el-select>
    </el-form-item>
    <el-form-item v-if="targetKind === 'groups'" label=" ">
      <el-select v-model="form.policyGroupRefs" multiple filterable style="width: 100%">
        <el-option v-for="g in groupNames" :key="g" :label="g" :value="g" />
      </el-select>
    </el-form-item>

    <el-form-item :label="$t('exception.namespaces')">
      <el-input v-model="namespacesCSV" placeholder="ns-a, ns-b" />
    </el-form-item>
    <el-form-item :label="$t('exception.nsSelector')">
      <LabelEditor v-model="nsLabels" />
    </el-form-item>
    <el-form-item :label="$t('exception.resSelector')">
      <LabelEditor v-model="resLabels" />
    </el-form-item>

    <el-form-item :label="$t('exception.duration')" required>
      <el-input v-model="form.duration" placeholder="24h / 30m / 168h" style="width: 200px" />
    </el-form-item>
    <el-form-item :label="$t('exception.retain')">
      <el-input v-model="form.retainAfterExpiry" placeholder="0s" style="width: 200px" />
    </el-form-item>
    <el-form-item :label="$t('exception.reason')" required>
      <el-input v-model="form.reason" type="textarea" :rows="2" />
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
import { ElMessage } from 'element-plus'
import { useI18n } from 'vue-i18n'
import { ApiError } from '../api/http'
import {
  createException, getException, updateException, type ExceptionRequest,
} from '../api/exception'
import { listPolicies } from '../api/policy'
import { listGroups } from '../api/group'
import LabelEditor from '../components/LabelEditor.vue'

const route = useRoute()
const router = useRouter()
const { t } = useI18n()

const isEdit = route.name === 'exception-edit'
const editName = String(route.params.name ?? '')
const saving = ref(false)
let resourceVersion = ''

const targetKind = ref<'policies' | 'groups' | 'all'>('policies')
const policyNames = ref<string[]>([])
const groupNames = ref<string[]>([])
const namespacesCSV = ref('')
const nsLabels = ref<Record<string, string>>({})
const resLabels = ref<Record<string, string>>({})

const form = reactive({
  name: '',
  policyRefs: [] as string[],
  policyGroupRefs: [] as string[],
  duration: '24h',
  retainAfterExpiry: '',
  reason: '',
})

function toRequest(): ExceptionRequest {
  const namespaces = namespacesCSV.value.split(',').map((s) => s.trim()).filter((s) => s !== '')
  return {
    name: isEdit ? undefined : form.name,
    policyRefs: targetKind.value === 'policies' ? form.policyRefs : undefined,
    policyGroupRefs: targetKind.value === 'groups' ? form.policyGroupRefs : undefined,
    allPolicies: targetKind.value === 'all' ? true : undefined,
    match: {
      namespaces: namespaces.length ? namespaces : undefined,
      namespaceSelector: Object.keys(nsLabels.value).length ? { matchLabels: nsLabels.value } : undefined,
      resourceSelector: Object.keys(resLabels.value).length ? { matchLabels: resLabels.value } : undefined,
    },
    duration: form.duration,
    retainAfterExpiry: form.retainAfterExpiry || undefined,
    reason: form.reason,
    resourceVersion: isEdit ? resourceVersion : undefined,
  }
}

async function onSave() {
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
    policyNames.value = (await listPolicies()).map((p) => p.name)
    groupNames.value = (await listGroups()).map((g) => g.name)
    if (!isEdit) return
    const d = await getException(editName)
    resourceVersion = d.resourceVersion
    form.name = d.name
    form.duration = d.spec.duration
    form.retainAfterExpiry = d.spec.retainAfterExpiry ?? ''
    form.reason = d.spec.reason
    form.policyRefs = d.spec.policyRefs ?? []
    form.policyGroupRefs = d.spec.policyGroupRefs ?? []
    targetKind.value = d.spec.allPolicies ? 'all' : d.spec.policyGroupRefs?.length ? 'groups' : 'policies'
    namespacesCSV.value = (d.spec.match.namespaces ?? []).join(', ')
    nsLabels.value = d.spec.match.namespaceSelector?.matchLabels ?? {}
    resLabels.value = d.spec.match.resourceSelector?.matchLabels ?? {}
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
})
</script>
