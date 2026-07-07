<template>
  <div v-if="d">
    <div style="display: flex; align-items: center; gap: 12px; margin-bottom: 12px">
      <h2 style="margin: 0">{{ d.spec.displayName || d.name }}</h2>
      <el-tag size="small" :type="d.source === 'builtin' ? 'info' : 'primary'">{{ d.source }}</el-tag>
      <PhaseTag :phase="d.status.phase" />
      <div style="flex: 1" />
      <el-button @click="$router.push(`/policygroups/${d.name}/edit`)">{{ $t('common.edit') }}</el-button>
      <el-button @click="$router.back()">{{ $t('common.back') }}</el-button>
    </div>

    <el-descriptions :column="2" border style="margin-bottom: 16px">
      <el-descriptions-item :label="$t('common.name')">{{ d.name }}</el-descriptions-item>
      <el-descriptions-item :label="$t('group.enabled')">{{ d.spec.enabled }}</el-descriptions-item>
      <el-descriptions-item :label="$t('common.description')" :span="2">{{ d.spec.description }}</el-descriptions-item>
      <el-descriptions-item :label="$t('group.nsSelector')" :span="2">
        <code>{{ JSON.stringify(d.spec.namespaceSelector ?? {}) }}</code>
      </el-descriptions-item>
    </el-descriptions>

    <h3>{{ $t('group.resolved') }} ({{ d.status.resolvedCount ?? 0 }})</h3>
    <el-table :data="d.status.resolvedPolicies ?? []">
      <el-table-column :label="$t('common.name')">
        <template #default="{ row }">
          <router-link :to="`/policies/${row.name}`">{{ row.name }}</router-link>
        </template>
      </el-table-column>
      <el-table-column :label="$t('group.memberSource')" prop="source" width="120" />
      <el-table-column :label="$t('group.effectiveMode')" width="120">
        <template #default="{ row }"><ModeTag :mode="row.enforcementMode" /></template>
      </el-table-column>
    </el-table>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { ElMessage } from 'element-plus'
import { getGroup, type GroupDetail } from '../api/group'
import ModeTag from '../components/ModeTag.vue'
import PhaseTag from '../components/PhaseTag.vue'

const route = useRoute()
const d = ref<GroupDetail>()

onMounted(async () => {
  try {
    d.value = await getGroup(String(route.params.name))
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
})
</script>
