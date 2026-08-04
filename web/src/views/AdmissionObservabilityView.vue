<template>
  <section>
    <PageHeader v-if="!embedded" :title="$t('linkIncident.title')" :subtitle="$t('linkIncident.subtitle')" />

    <div class="toolbar">
      <el-select v-model="clusters" multiple collapse-tags clearable filterable :placeholder="$t('linkIncident.cluster')" style="width:200px">
        <el-option v-for="name in clusterOptions" :key="name" :label="name" :value="name" />
      </el-select>
      <el-select v-model="components" multiple collapse-tags clearable :placeholder="$t('linkIncident.component')" style="width:180px">
        <el-option v-for="value in componentOptions" :key="value" :label="$t(`linkIncident.component_${value}`)" :value="value" />
      </el-select>
      <el-select v-model="reasons" multiple collapse-tags clearable :placeholder="$t('linkIncident.reason')" style="width:210px">
        <el-option v-for="value in reasonOptions" :key="value" :label="$t(`linkIncident.reason_${value}`)" :value="value" />
      </el-select>
      <el-select v-model="statuses" multiple collapse-tags clearable :placeholder="$t('linkIncident.status')" style="width:190px">
        <el-option v-for="value in statusOptions" :key="value" :label="$t(`linkIncident.status_${value}`)" :value="value" />
      </el-select>
      <el-select v-model="rangeKey" style="width:160px">
        <el-option :label="$t('admissionEvent.last1h')" value="1h" />
        <el-option :label="$t('admissionEvent.last6h')" value="6h" />
        <el-option :label="$t('admissionEvent.last24h')" value="24h" />
        <el-option :label="$t('admissionEvent.last7d')" value="7d" />
        <el-option :label="$t('admissionEvent.customRange')" value="custom" />
      </el-select>
      <el-date-picker v-if="rangeKey === 'custom'" v-model="customRange" type="datetimerange" :teleported="false" style="width:360px" />
    </div>

    <section v-if="activeIncidents.length" class="active-section">
      <div class="section-heading">
        <div><h2>{{ $t('linkIncident.activeTitle') }}</h2><p>{{ $t('linkIncident.activeHint') }}</p></div>
        <span class="live"><span class="live-dot" />{{ $t('linkIncident.liveRefresh') }}</span>
      </div>
      <div class="active-grid">
        <el-card v-for="incident in activeIncidents" :key="incident.id" shadow="never" class="active-card" @click="openDetail(incident)">
          <div class="active-card-head">
            <el-tag type="danger" effect="dark">{{ $t(`linkIncident.reason_${incident.reason}`) }}</el-tag>
            <strong>{{ duration(incident.startedAt, now) }}</strong>
          </div>
          <div class="active-meta">{{ incident.clusterName }} · {{ $t(`linkIncident.component_${incident.component}`) }}</div>
          <div class="active-count">{{ $t('linkIncident.affected', { count: incident.affectedCount }) }}</div>
        </el-card>
      </div>
    </section>

    <div class="section-heading history-heading">
      <div><h2>{{ $t('linkIncident.historyTitle') }}</h2><p>{{ $t('linkIncident.historyHint') }}</p></div>
    </div>
    <el-table :data="incidents" v-loading="loading" row-key="id">
      <el-table-column :label="$t('linkIncident.timeWindow')" min-width="220">
        <template #default="{ row }">{{ formatTime(row.startedAt) }}<br><span class="muted">→ {{ formatTime(row.lastSeenAt) }}</span></template>
      </el-table-column>
      <el-table-column prop="clusterName" :label="$t('linkIncident.cluster')" min-width="120" />
      <el-table-column :label="$t('linkIncident.component')" width="110"><template #default="{ row }">{{ $t(`linkIncident.component_${row.component}`) }}</template></el-table-column>
      <el-table-column :label="$t('linkIncident.reason')" min-width="170"><template #default="{ row }">{{ $t(`linkIncident.reason_${row.reason}`) }}</template></el-table-column>
      <el-table-column prop="affectedCount" :label="$t('linkIncident.affectedCount')" width="110" />
      <el-table-column :label="$t('linkIncident.status')" width="120">
        <template #default="{ row }"><el-tag :type="statusTag(row.status)">{{ $t(`linkIncident.status_${row.status}`) }}</el-tag></template>
      </el-table-column>
      <el-table-column :label="$t('common.actions')" width="90" fixed="right"><template #default="{ row }"><el-button link type="primary" @click="openDetail(row)">{{ $t('common.view') }}</el-button></template></el-table-column>
    </el-table>
    <el-empty v-if="!loading && incidents.length === 0" :description="$t('linkIncident.empty')" />
    <PaginationBar :total="total" :page="page" :page-size="pageSize" @update:page="p => { page=p; loadHistory() }" @update:page-size="s => { pageSize=s; page=1; loadHistory() }" />

    <el-drawer v-model="detailOpen" :title="$t('linkIncident.detailTitle')" size="620px" destroy-on-close>
      <el-skeleton :loading="detailLoading" animated>
        <template #default>
          <template v-if="detail">
            <el-descriptions :column="2" border>
              <el-descriptions-item :label="$t('linkIncident.cluster')">{{ detail.clusterName }}</el-descriptions-item>
              <el-descriptions-item :label="$t('linkIncident.component')">{{ $t(`linkIncident.component_${detail.component}`) }}</el-descriptions-item>
              <el-descriptions-item :label="$t('linkIncident.reason')">{{ $t(`linkIncident.reason_${detail.reason}`) }}</el-descriptions-item>
              <el-descriptions-item :label="$t('linkIncident.status')"><el-tag :type="statusTag(detail.status)">{{ $t(`linkIncident.status_${detail.status}`) }}</el-tag></el-descriptions-item>
              <el-descriptions-item :label="$t('linkIncident.duration')">{{ duration(detail.startedAt, detail.lastSeenAt) }}</el-descriptions-item>
              <el-descriptions-item :label="$t('linkIncident.affectedCount')">{{ detail.affectedCount }}</el-descriptions-item>
              <el-descriptions-item :label="$t('linkIncident.instance')" :span="2">{{ detail.reporterInstanceId }}</el-descriptions-item>
            </el-descriptions>
            <h3>{{ $t('linkIncident.reasonDescription') }}</h3><p>{{ $t(`linkIncident.description_${detail.reason}`) }}</p>
            <h3>{{ $t('linkIncident.advice') }}</h3><el-alert type="warning" :closable="false" :title="$t(`linkIncident.advice_${detail.reason}`)" />
            <h3>{{ $t('linkIncident.samples') }}</h3>
            <div v-if="detail.samples.length" class="samples">
              <el-button v-for="sample in detail.samples" :key="sample.eventId" link :disabled="sample.expired" @click="openEvent(sample.eventDatabaseId)">
                {{ sample.eventId }} <span v-if="sample.expired">({{ $t('linkIncident.expired') }})</span>
              </el-button>
            </div>
			<span v-else class="muted">{{ $t('admissionEvent.none') }}</span>
            <h3>{{ $t('linkIncident.lastError') }}</h3><pre class="detail-error">{{ detail.detail || '—' }}</pre>
			<el-button v-if="detail.status === 'interrupted'" type="primary" plain @click="router.push('/observability')">{{ $t('linkIncident.gotoPlatform') }}</el-button>
          </template>
        </template>
      </el-skeleton>
    </el-drawer>
  </section>
</template>

<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { useRouter } from 'vue-router'
import PageHeader from '../components/PageHeader.vue'
import PaginationBar from '../components/PaginationBar.vue'
import { getLinkIncident, listLinkIncidents, type LinkIncident, type LinkIncidentComponent, type LinkIncidentDetail, type LinkIncidentReason, type LinkIncidentStatus } from '../api/observability'
import { listClusters } from '../api/cluster'
import { collectAllPages } from '../api/pagination'
import { eventDetailTarget } from './admissionEventsTabs'

defineProps<{ embedded?: boolean }>()
const router = useRouter()
const componentOptions: LinkIncidentComponent[] = ['webhook','operator','gateway']
const reasonOptions: LinkIncidentReason[] = ['queue_full','retry_exhausted','ack_timeout','auth_failure','batch_failure','write_failed','report_dropped']
const statusOptions: LinkIncidentStatus[] = ['active','interrupted','closed']
const clusters=ref<string[]>([]), components=ref<LinkIncidentComponent[]>([]), reasons=ref<LinkIncidentReason[]>([]), statuses=ref<LinkIncidentStatus[]>([])
const clusterOptions=ref<string[]>([]), rangeKey=ref('1h'), customRange=ref<[Date,Date]|null>(null)
const activeIncidents=ref<LinkIncident[]>([]), incidents=ref<LinkIncident[]>([]), total=ref(0), page=ref(1), pageSize=ref(20), loading=ref(false)
const detailOpen=ref(false), detailLoading=ref(false), detail=ref<LinkIncidentDetail>(), now=ref(new Date().toISOString())
let activeTimer:number|undefined, historyTimer:number|undefined, clockTimer:number|undefined

function range(): Record<string,string>{
  if(rangeKey.value==='custom') return customRange.value?{from:customRange.value[0].toISOString(),to:customRange.value[1].toISOString()}:{}
  const spans:Record<string,number>={'1h':36e5,'6h':216e5,'24h':864e5,'7d':6048e5}; const end=Date.now()
  return {from:new Date(end-(spans[rangeKey.value]??36e5)).toISOString(),to:new Date(end).toISOString()}
}
function filters(): Record<string,string>{ return {
  ...(clusters.value.length?{cluster:clusters.value.join(',')}:{}), ...(components.value.length?{component:components.value.join(',')}:{}),
  ...(reasons.value.length?{reason:reasons.value.join(',')}:{}),
} }
async function loadActive(){
  if(document.hidden)return
  try{ const result=await listLinkIncidents({...filters(),status:'active',page:'1',pageSize:'100'}); activeIncidents.value=result.items }
  catch(error){ console.warn('load active link incidents',error) }
}
async function loadHistory(){
  if(document.hidden)return
  loading.value=true
  try{
    const result=await listLinkIncidents({...filters(),...(statuses.value.length?{status:statuses.value.join(',')}:{}),...range(),page:String(page.value),pageSize:String(pageSize.value)})
    incidents.value=result.items;total.value=result.total
  }catch(error){ElMessage.error((error as Error).message)}finally{loading.value=false}
}
function reload(){page.value=1;void loadActive();void loadHistory()}
watch([clusters,components,reasons,statuses,rangeKey,customRange],reload,{deep:true})
async function openDetail(row:LinkIncident){detailOpen.value=true;detailLoading.value=true;detail.value=undefined;try{detail.value=await getLinkIncident(row.id)}catch(error){ElMessage.error((error as Error).message)}finally{detailLoading.value=false}}
function openEvent(id?:string){if(id)void router.push(eventDetailTarget(id))}
function statusTag(status:LinkIncidentStatus){return status==='active'?'danger':status==='interrupted'?'warning':'success'}
function formatTime(value:string){return new Date(value).toLocaleString()}
function duration(start:string,end:string){const seconds=Math.max(0,Math.floor((new Date(end).getTime()-new Date(start).getTime())/1000));if(seconds<60)return `${seconds}s`;if(seconds<3600)return `${Math.floor(seconds/60)}m`;if(seconds<86400)return `${Math.floor(seconds/3600)}h ${Math.floor(seconds%3600/60)}m`;return `${Math.floor(seconds/86400)}d ${Math.floor(seconds%86400/3600)}h`}
function onVisibility(){if(!document.hidden)reload()}
onMounted(async()=>{try{clusterOptions.value=Array.from(new Set(['platform',...(await collectAllPages(listClusters)).map(value=>value.name)]))}catch{clusterOptions.value=['platform']}reload();activeTimer=window.setInterval(loadActive,10000);historyTimer=window.setInterval(loadHistory,60000);clockTimer=window.setInterval(()=>{now.value=new Date().toISOString()},1000);document.addEventListener('visibilitychange',onVisibility)})
onBeforeUnmount(()=>{if(activeTimer)clearInterval(activeTimer);if(historyTimer)clearInterval(historyTimer);if(clockTimer)clearInterval(clockTimer);document.removeEventListener('visibilitychange',onVisibility)})
</script>

<style scoped>
.toolbar{display:flex;flex-wrap:wrap;gap:8px;margin-bottom:18px}.section-heading{display:flex;align-items:center;justify-content:space-between}.section-heading h2{margin:0}.section-heading p{margin:4px 0 12px;color:var(--el-text-color-secondary);font-size:13px}.live{font-size:12px;color:var(--el-text-color-secondary)}.live-dot{display:inline-block;width:7px;height:7px;margin-right:6px;border-radius:50%;background:var(--el-color-success)}.active-section{margin-bottom:24px}.active-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(240px,1fr));gap:12px}.active-card{cursor:pointer;border-color:var(--el-color-danger-light-5)}.active-card-head{display:flex;align-items:center;justify-content:space-between}.active-meta,.active-count{margin-top:12px}.active-meta,.muted{color:var(--el-text-color-secondary);font-size:12px}.active-count{font-weight:600}.history-heading{margin-top:4px}.samples{display:flex;align-items:flex-start;flex-direction:column}.detail-error{padding:12px;white-space:pre-wrap;word-break:break-word;background:var(--el-fill-color-light);border-radius:6px}h3{margin:20px 0 8px}@media(max-width:760px){.toolbar>*{width:100%!important}}
</style>
