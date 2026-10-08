<template>
  <el-dialog v-model="visible" :title="$t('tenant.assignClusters', { name: tenant?.name })" width="600px">
    <el-alert v-if="transfers.length" type="warning" :closable="false" :title="$t('tenant.transferWarning', { names: transfers.join(', ') })" />
    <el-checkbox-group v-model="selected" class="cluster-list">
      <el-checkbox v-for="cluster in clusters" :key="cluster.name" :value="cluster.name">
        {{ cluster.name }} <span class="muted">{{ ownerName(cluster.tenantId) }}</span>
      </el-checkbox>
    </el-checkbox-group>
    <template #footer><el-button @click="visible=false">{{ $t('common.cancel') }}</el-button><el-button type="primary" :loading="saving" @click="save">{{ $t('common.save') }}</el-button></template>
  </el-dialog>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { useI18n } from 'vue-i18n'
import { listClusters, type ClusterListItem } from '../api/cluster'
import { listTenants, syncTenantClusters, type Tenant } from '../api/identity'
import { collectAllPages } from '../api/pagination'

const emit = defineEmits<{ saved: [] }>()
const { t } = useI18n()
const visible = ref(false), saving = ref(false), tenant = ref<Tenant>(), clusters = ref<ClusterListItem[]>([]), tenants = ref<Tenant[]>([]), selected = ref<string[]>([])
const transfers = computed(() => clusters.value.filter(c => selected.value.includes(c.name) && c.tenantId && c.tenantId !== tenant.value?.id).map(c => c.name))
const ownerName = (id?: string) => id ? `· ${tenants.value.find(t => t.id === id)?.name ?? id}` : `· ${t('cluster.unassigned')}`

async function open(value: Tenant) {
  tenant.value = value
  try {
    ;[clusters.value, tenants.value] = await Promise.all([collectAllPages(listClusters), listTenants()])
    selected.value = clusters.value.filter(c => c.tenantId === value.id).map(c => c.name)
    visible.value = true
  } catch (error) { ElMessage.error((error as Error).message) }
}
async function save() {
  if (!tenant.value?.id) return
  saving.value = true
  try {
    await syncTenantClusters(tenant.value.id, selected.value, transfers.value.length > 0)
    visible.value = false; emit('saved'); ElMessage.success(t('common.saved'))
  } catch (error) { ElMessage.error((error as Error).message) }
  finally { saving.value = false }
}
defineExpose({ open })
</script>
<style scoped>.cluster-list{display:grid;gap:10px;margin-top:16px}.muted{color:var(--el-text-color-secondary);font-size:12px}</style>
