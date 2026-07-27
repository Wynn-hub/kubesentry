<template>
  <section>
    <PageHeader title="准入事件" subtitle="检索 audit 与 deny 准入判决" />
    <el-card shadow="never">
      <div class="toolbar"><el-select v-model="verdict" clearable placeholder="判决" style="width:130px"><el-option label="Audit" value="audit"/><el-option label="Deny" value="deny"/></el-select><el-input v-model="cluster" clearable placeholder="集群名称" style="width:180px"/><el-button type="primary" @click="load">查询</el-button><el-button @click="openSettings">存储策略</el-button></div>
      <el-table :data="events" v-loading="loading" row-key="id"><el-table-column prop="occurredAt" label="时间" min-width="180"><template #default="{row}">{{ new Date(row.occurredAt).toLocaleString() }}</template></el-table-column><el-table-column prop="clusterName" label="集群" min-width="130"/><el-table-column prop="verdict" label="判决" width="100"><template #default="{row}"><el-tag :type="row.verdict === 'deny' ? 'danger' : 'warning'">{{ row.verdict }}</el-tag></template></el-table-column><el-table-column label="资源" min-width="190"><template #default="{row}">{{ row.apiGroup || 'core' }}/{{ row.resource }} {{ row.namespace ? row.namespace + '/' : '' }}{{ row.resourceName }}</template></el-table-column><el-table-column prop="username" label="操作者" min-width="140"/><el-table-column label="策略" min-width="180"><template #default="{row}">{{ row.policyNames.join(', ') }}</template></el-table-column></el-table>
      <PaginationBar :total="total" :page="page" :page-size="pageSize" @update:page="p => { page=p; load() }" @update:page-size="s => { pageSize=s; page=1; load() }" />
    </el-card>
    <el-drawer v-model="settingsOpen" title="事件存储策略" size="520px"><el-form v-if="settings" label-position="top"><el-form-item label="默认保存天数"><el-input-number v-model="settings.defaultRetentionDays" :min="1" :max="365"/></el-form-item><el-alert type="info" :closable="false" title="规则按顺序匹配；当前页面可管理默认保留期，规则编辑将在后续迭代提供。"/><el-button type="primary" class="save" @click="saveSettings">保存</el-button></el-form></el-drawer>
  </section>
</template>
<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import PageHeader from '../components/PageHeader.vue'
import PaginationBar from '../components/PaginationBar.vue'
import { getAdmissionSettings, listAdmissionEvents, updateAdmissionSettings, type AdmissionEvent, type AdmissionSettings } from '../api/admissionEvent'
const events=ref<AdmissionEvent[]>([]), total=ref(0), loading=ref(false), page=ref(1), pageSize=ref(20), verdict=ref(''), cluster=ref(''), settingsOpen=ref(false), settings=ref<AdmissionSettings>()
async function load(){ loading.value=true; try { const result=await listAdmissionEvents({page:String(page.value),pageSize:String(pageSize.value),sortBy:'occurredAt',sortOrder:'desc',...(verdict.value?{verdict:verdict.value}:{}),...(cluster.value?{cluster:cluster.value}:{})}); events.value=result.items; total.value=result.total } finally { loading.value=false } }
async function openSettings(){ settings.value=await getAdmissionSettings(); settingsOpen.value=true }
async function saveSettings(){ if(!settings.value)return; settings.value=await updateAdmissionSettings(settings.value); ElMessage.success('已保存') }
onMounted(load)
</script>
<style scoped>.toolbar{display:flex;gap:10px;margin-bottom:16px}.save{margin-top:24px}</style>
