<template>
  <el-form label-width="140px" style="max-width: 800px">
    <el-form-item :label="$t('common.name')" required>
      <el-input v-model="form.name" :disabled="isEdit" />
    </el-form-item>

    <el-form-item :label="$t('distribution.policyGroups')" required>
      <el-select v-model="selectedGroups" multiple filterable style="width: 100%">
        <el-option v-for="g in allGroups" :key="g" :label="g" :value="g" />
      </el-select>
    </el-form-item>

    <el-form-item :label="$t('distribution.byName')">
      <el-select v-model="selectedClusterNames" multiple filterable style="width: 100%">
        <el-option v-for="c in allClusters" :key="c" :label="c" :value="c" />
      </el-select>
    </el-form-item>

    <el-form-item :label="$t('distribution.clusterSelector')">
      <LabelEditor v-model="clusterSelector" />
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
  createDistribution, getDistribution, updateDistribution, type DistributionRequest,
} from '../api/distribution'
import { listGroups } from '../api/group'
import { listClusters } from '../api/cluster'
import { collectAllPages } from '../api/pagination'
import type { LabelSelector } from '../api/group'
import LabelEditor from '../components/LabelEditor.vue'

const route = useRoute()
const router = useRouter()
const { t } = useI18n()

const isEdit = route.name === 'distribution-edit'
const editName = String(route.params.name ?? '')
const saving = ref(false)
let resourceVersion = ''

const form = reactive({ name: '' })
const allGroups = ref<string[]>([])
const allClusters = ref<string[]>([])
const selectedGroups = ref<string[]>([])
const selectedClusterNames = ref<string[]>([])
const clusterSelector = ref<LabelSelector | null>(null)

function toRequest(): DistributionRequest {
  return {
    name: isEdit ? undefined : form.name,
    spec: {
      policyGroups: selectedGroups.value,
      clusters: {
        byName: selectedClusterNames.value.length ? selectedClusterNames.value : undefined,
        clusterSelector: clusterSelector.value ?? undefined,
      },
    },
    resourceVersion: isEdit ? resourceVersion : undefined,
  }
}

async function onSave() {
  saving.value = true
  try {
    if (isEdit) {
      await updateDistribution(editName, toRequest())
    } else {
      await createDistribution(toRequest())
    }
    ElMessage.success(t('common.saved'))
    router.push('/distributions')
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
    allGroups.value = (await collectAllPages(listGroups)).map((g) => g.name)
    allClusters.value = (await collectAllPages(listClusters)).map((c) => c.name)
    if (!isEdit) return
    const d = await getDistribution(editName)
    resourceVersion = d.resourceVersion
    form.name = d.name
    selectedGroups.value = d.spec.policyGroups ?? []
    selectedClusterNames.value = d.spec.clusters?.byName ?? []
    clusterSelector.value = d.spec.clusters?.clusterSelector ?? null
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
})
</script>
