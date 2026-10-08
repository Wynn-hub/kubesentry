<template>
  <el-dialog v-model="visible" :title="$t('cluster.assignTenant', { name: cluster?.name })" width="480px">
    <el-select v-model="tenantId" clearable style="width:100%" :placeholder="$t('cluster.unassigned')">
      <el-option v-for="tenant in tenants" :key="tenant.id" :value="tenant.id!" :label="tenant.name" />
    </el-select>
    <template #footer><el-button @click="visible=false">{{ $t('common.cancel') }}</el-button><el-button type="primary" :loading="saving" @click="save">{{ $t('common.save') }}</el-button></template>
  </el-dialog>
</template>
<script setup lang="ts">
import { ref } from 'vue'
import { ElMessage } from 'element-plus'
import { useI18n } from 'vue-i18n'
import { assignClusterTenant, type ClusterListItem } from '../api/cluster'
import { listTenants, type Tenant } from '../api/identity'
const emit = defineEmits<{ saved: [] }>(); const { t } = useI18n()
const visible=ref(false), saving=ref(false), cluster=ref<ClusterListItem>(), tenants=ref<Tenant[]>([]), tenantId=ref<string>()
async function open(value: ClusterListItem) { cluster.value=value; tenantId.value=value.tenantId; try { tenants.value=(await listTenants()).filter(x=>x.status==='active'); visible.value=true } catch(e){ ElMessage.error((e as Error).message) } }
async function save(){ if(!cluster.value)return; saving.value=true; try{await assignClusterTenant(cluster.value.name,tenantId.value);visible.value=false;emit('saved');ElMessage.success(t('common.saved'))}catch(e){ElMessage.error((e as Error).message)}finally{saving.value=false} }
defineExpose({open})
</script>
