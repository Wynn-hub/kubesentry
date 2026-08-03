<template>
  <section>
    <PageHeader v-if="!embedded" :title="$t('admissionEvent.title')" :subtitle="$t('admissionEvent.subtitle')" />

    <div class="list-toolbar">
      <el-select v-model="verdicts" multiple collapse-tags clearable :placeholder="$t('admissionEvent.verdict')" style="width:170px">
        <el-option :label="$t('admissionEvent.audit')" value="audit" />
        <el-option :label="$t('admissionEvent.deny')" value="deny" />
      </el-select>
      <TagInput v-model="clusters" :placeholder="$t('admissionEvent.clusterPlaceholder')" style="width:200px" />
      <TagInput v-model="namespaces" :placeholder="$t('admissionEvent.namespacePlaceholder')" style="width:200px" />
      <ResourceGVRSelect v-model="resources" :placeholder="$t('admissionEvent.resource')" style="width:240px" />
      <el-select v-model="rangeKey" style="width:170px">
        <el-option :label="$t('admissionEvent.last15m')" value="15m" />
        <el-option :label="$t('admissionEvent.last1h')" value="1h" />
        <el-option :label="$t('admissionEvent.last6h')" value="6h" />
        <el-option :label="$t('admissionEvent.last24h')" value="24h" />
        <el-option :label="$t('admissionEvent.last7d')" value="7d" />
        <el-option :label="$t('admissionEvent.last30d')" value="30d" />
        <el-option :label="$t('admissionEvent.customRange')" value="custom" />
      </el-select>
      <el-date-picker v-if="rangeKey === 'custom'" v-model="customRange" type="datetimerange" :teleported="false" style="width:360px" />
    </div>

    <el-table :data="events" v-loading="loading" row-key="id">
      <el-table-column prop="occurredAt" :label="$t('admissionEvent.time')" min-width="170"><template #default="{row}">{{ new Date(row.occurredAt).toLocaleString() }}</template></el-table-column>
      <el-table-column prop="clusterName" :label="$t('admissionEvent.cluster')" min-width="110"/>
      <el-table-column prop="verdict" :label="$t('admissionEvent.verdict')" width="90"><template #default="{row}"><el-tag :type="verdictTagType(row.verdict)" effect="dark">{{ row.verdict }}</el-tag></template></el-table-column>
      <el-table-column :label="$t('admissionEvent.resource')" min-width="150"><template #default="{row}">{{ resourceType(row) }}</template></el-table-column>
      <el-table-column prop="namespace" :label="$t('admissionEvent.namespace')" min-width="120"><template #default="{row}">{{ row.namespace || '—' }}</template></el-table-column>
      <el-table-column prop="username" :label="$t('admissionEvent.operator')" min-width="120"/>
      <el-table-column :label="$t('admissionEvent.policies')" min-width="200">
        <template #default="{row}">
          <div v-if="row.policyNames?.length" class="policy-tags">
            <el-tag v-for="policy in row.policyNames.slice(0, POLICY_PREVIEW)" :key="policy" size="small" class="policy-link" @click="goPolicy(policy)">{{ policy }}</el-tag>
            <el-popover v-if="row.policyNames.length > POLICY_PREVIEW" placement="top" trigger="hover" :width="260">
              <template #reference>
                <el-tag size="small" type="info" class="policy-more">+{{ row.policyNames.length - POLICY_PREVIEW }}</el-tag>
              </template>
              <div class="policy-pop-title">{{ $t('admissionEvent.morePolicies', { count: row.policyNames.length }) }}</div>
              <div class="policy-pop-list">
                <el-tag v-for="policy in row.policyNames" :key="policy" size="small" class="policy-link" @click="goPolicy(policy)">{{ policy }}</el-tag>
              </div>
            </el-popover>
          </div>
          <span v-else class="muted">{{ $t('admissionEvent.unidentified') }}</span>
        </template>
      </el-table-column>
      <el-table-column :label="$t('admissionEvent.detail')" width="90" fixed="right"><template #default="{row}"><el-button link type="primary" @click="openDetail(row)">{{ $t('common.view') }}</el-button></template></el-table-column>
    </el-table>
    <PaginationBar :total="total" :page="page" :page-size="pageSize" @update:page="p => { page=p; load() }" @update:page-size="s => { pageSize=s; page=1; load() }" />

    <el-drawer v-model="detailOpen" :title="$t('admissionEvent.detailTitle')" size="680px" destroy-on-close @close="closeDetail">
      <el-skeleton :loading="detailLoading" animated>
        <template #default>
          <template v-if="detail">
            <el-descriptions :column="2" border>
              <el-descriptions-item :label="$t('admissionEvent.time')">{{ new Date(detail.occurredAt).toLocaleString() }}</el-descriptions-item>
              <el-descriptions-item :label="$t('admissionEvent.verdict')"><el-tag :type="verdictTagType(detail.verdict)" effect="dark">{{ detail.verdict }}</el-tag></el-descriptions-item>
              <el-descriptions-item :label="$t('admissionEvent.cluster')">{{ detail.clusterName || '—' }}</el-descriptions-item>
              <el-descriptions-item :label="$t('admissionEvent.resource')">{{ resourceType(detail) }}</el-descriptions-item>
              <el-descriptions-item :label="$t('admissionEvent.namespace')">{{ detail.namespace || '—' }}</el-descriptions-item>
              <el-descriptions-item :label="$t('admissionEvent.resourceName')">{{ detail.resourceName || '—' }}</el-descriptions-item>
              <el-descriptions-item :label="$t('admissionEvent.operation')">{{ detail.operation }}</el-descriptions-item>
              <el-descriptions-item :label="$t('admissionEvent.operator')">{{ detail.username }}</el-descriptions-item>
              <el-descriptions-item :label="$t('admissionEvent.policies')" :span="2"><div v-if="detail.policyNames?.length" class="policy-tags"><el-tag v-for="policy in detail.policyNames" :key="policy" size="small" class="policy-link" @click="goPolicy(policy)">{{ policy }}</el-tag></div><span v-else class="muted">{{ $t('admissionEvent.unidentified') }}</span></el-descriptions-item>
            </el-descriptions>
            <h3>{{ $t('admissionEvent.violations') }}</h3>
            <div v-if="detail.violations.length" class="violations">
              <pre v-for="(violation, index) in detail.violations" :key="index" class="violation-view" :class="detail.verdict === 'deny' ? 'violation-deny' : 'violation-audit'">{{ violation }}</pre>
            </div>
            <span v-else class="muted">{{ $t('admissionEvent.none') }}</span>
            <h3>{{ $t('admissionEvent.specSnapshot') }}</h3>
            <pre class="json-view" v-html="highlightJSON(detail.specSnapshot)"></pre>
            <h3>{{ $t('admissionEvent.metadataSnapshot') }}</h3>
            <pre class="json-view" v-html="highlightJSON(detail.metadataSnapshot)"></pre>
          </template>
        </template>
      </el-skeleton>
    </el-drawer>
  </section>
</template>
<script setup lang="ts">
import { onMounted, ref, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import PageHeader from '../components/PageHeader.vue'
import PaginationBar from '../components/PaginationBar.vue'
import TagInput from '../components/TagInput.vue'
import ResourceGVRSelect from '../components/ResourceGVRSelect.vue'
import { getAdmissionEvent, listAdmissionEvents, type AdmissionEvent, type AdmissionEventDetail } from '../api/admissionEvent'

// How many policy tags to show inline in the list before collapsing the rest
// into a "+N" badge; the detail drawer always shows the full set.
const POLICY_PREVIEW = 2
defineProps<{ embedded?: boolean }>()
const { t } = useI18n()
const router = useRouter()
const route = useRoute()
const events=ref<AdmissionEvent[]>([]), total=ref(0), loading=ref(false), page=ref(1), pageSize=ref(20)
const verdicts=ref<string[]>([]), clusters=ref<string[]>([]), namespaces=ref<string[]>([])
const resources=ref<string[]>([])
const rangeKey=ref('24h'), customRange=ref<[Date,Date]|null>(null)
const detailOpen=ref(false), detailLoading=ref(false), detail=ref<AdmissionEventDetail>()

// Grafana-style range: relative shortcuts resolve to [from,to] at query time;
// "custom" uses the absolute datetimerange picker.
function resolveRange(): { from?: string; to?: string } {
  if (rangeKey.value === 'custom') {
    if (!customRange.value) return {}
    return { from: customRange.value[0].toISOString(), to: customRange.value[1].toISOString() }
  }
  const now = Date.now()
  const spans: Record<string, number> = { '15m': 9e5, '1h': 36e5, '6h': 216e5, '24h': 864e5, '7d': 6048e5, '30d': 2592e6 }
  const span = spans[rangeKey.value]
  if (!span) return {}
  return { from: new Date(now - span).toISOString(), to: new Date(now).toISOString() }
}

async function load(){
  loading.value=true
  try {
    const { from, to } = resolveRange()
    const result=await listAdmissionEvents({
      page:String(page.value), pageSize:String(pageSize.value), sortBy:'occurredAt', sortOrder:'desc',
      ...(verdicts.value.length?{verdict:verdicts.value.join(',')}:{}),
      ...(clusters.value.length?{cluster:clusters.value.join(',')}:{}),
      ...(namespaces.value.length?{namespace:namespaces.value.join(',')}:{}),
      ...(resources.value.length?{gvk:resources.value.join(',')}:{}),
      ...(from?{from}:{}), ...(to?{to}:{}),
    })
    events.value=result.items; total.value=result.total
  } finally { loading.value=false }
}

// All filters are discrete (multi-select or tag add/remove), so every change
// applies immediately without debouncing.
function reloadFromFirstPage(){ page.value=1; load() }
watch([verdicts, clusters, namespaces, resources, rangeKey, customRange], reloadFromFirstPage, { deep: true })

// Normalized GVR: core group renders as bare resource ("pods"), matching the filter/canonical format.
function resourceType(event: AdmissionEvent){ return event.apiGroup ? `${event.apiGroup}/${event.resource}` : event.resource }
// deny = blocked (danger red), audit = warned (warning amber).
function verdictTagType(verdict: string){ return verdict === 'deny' ? 'danger' : 'warning' }
// Policy names map 1:1 to the /policies/:name detail route. Confirm first so a
// stray click in the event list doesn't navigate away unexpectedly.
async function goPolicy(name: string){
  try { await ElMessageBox.confirm(t('admissionEvent.gotoPolicy', { name })) } catch { return }
  router.push(`/policies/${name}`)
}

// Lightweight JSON syntax highlighting: pretty-print, HTML-escape (the snapshot
// is untrusted user resource data rendered via v-html), then wrap tokens in
// coloured spans. Avoids pulling in a highlighter dependency.
function escapeHTML(s: string){ return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;') }
function highlightJSON(value: unknown){
  const json = escapeHTML(JSON.stringify(value ?? {}, null, 2))
  return json.replace(
    /("(\\u[a-zA-Z0-9]{4}|\\[^u]|[^\\"])*"(\s*:)?|\b(true|false|null)\b|-?\d+(?:\.\d+)?(?:[eE][+-]?\d+)?)/g,
    (match) => {
      let cls = 'jn' // number
      if (/^"/.test(match)) cls = /:$/.test(match) ? 'jk' : 'js' // key vs string
      else if (/true|false/.test(match)) cls = 'jb' // boolean
      else if (/null/.test(match)) cls = 'ju' // null
      return `<span class="${cls}">${match}</span>`
    },
  )
}
// The detail drawer is driven by the ?event=<id> query rather than local state,
// so it survives navigation: opening pushes a history entry, which means coming
// back from a policy detail page reopens the drawer instead of dropping the user
// on the bare list. Closing removes the param.
function openDetail(event: AdmissionEvent){ router.push({ query: { ...route.query, event: event.id } }) }
// Guarded so the drawer's own @close (which also fires when a route change
// closes it) cannot push a redundant history entry.
function closeDetail(){
  if (!route.query.event) return
  const query = { ...route.query }
  delete query.event
  router.push({ query })
}

async function syncDetailFromRoute(){
  const id = typeof route.query.event === 'string' ? route.query.event : ''
  if (!id) { detailOpen.value = false; detail.value = undefined; return }
  detailOpen.value = true
  detailLoading.value = true
  detail.value = undefined
  try { detail.value = await getAdmissionEvent(id) }
  catch (e) { ElMessage.error((e as Error).message); closeDetail() }
  finally { detailLoading.value = false }
}
watch(() => route.query.event, syncDetailFromRoute)

onMounted(async () => {
  await load()
  // Deep link / restored history entry may already carry ?event=<id>.
  await syncDetailFromRoute()
})
</script>
<style scoped>
.list-toolbar { display:flex; flex-wrap:wrap; align-items:center; gap:8px; margin-bottom:12px }
.list-toolbar > * { flex-shrink:0 }
.muted{color:var(--el-text-color-secondary);font-size:12px}
.policy-tags{display:flex;gap:4px;flex-wrap:wrap}
.policy-link{cursor:pointer;transition:opacity .15s}
.policy-link:hover{opacity:.75;text-decoration:underline}
.policy-more{cursor:default}
.policy-pop-title{font-size:12px;color:var(--el-text-color-secondary);margin-bottom:8px}
.policy-pop-list{display:flex;flex-wrap:wrap;gap:6px;max-height:220px;overflow:auto}
.json-view,.violation-view{margin:8px 0 20px;max-height:360px;overflow:auto;padding:12px;border-radius:6px;background:var(--el-fill-color-light);white-space:pre-wrap;word-break:break-word}
/* JSON syntax highlighting tokens (see highlightJSON). */
.json-view :deep(.jk){color:var(--el-color-primary)}
.json-view :deep(.js){color:var(--el-color-success)}
.json-view :deep(.jn){color:var(--el-color-warning)}
.json-view :deep(.jb){color:var(--el-color-danger)}
.json-view :deep(.ju){color:var(--el-text-color-secondary)}
/* Colour-code violations by verdict so deny (blocked) reads as more severe than audit (warned). */
.violation-view{border-left:4px solid var(--el-border-color)}
.violation-deny{border-left-color:var(--el-color-danger);background:var(--el-color-danger-light-9)}
.violation-audit{border-left-color:var(--el-color-warning);background:var(--el-color-warning-light-9)}
.violations .violation-view:last-child{margin-bottom:20px}
</style>
