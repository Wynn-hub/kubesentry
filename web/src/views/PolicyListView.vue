<!-- web/src/views/PolicyListView.vue -->
<template>
  <div class="list-toolbar">
    <el-select
      v-model="filters.source"
      multiple
      collapse-tags
      clearable
      :placeholder="$t('policy.allSources')"
      style="width: 170px"
    >
      <el-option :label="$t('policy.builtin')" value="builtin" />
      <el-option :label="$t('policy.custom')" value="custom" />
    </el-select>
    <el-select
      v-model="filters.phase"
      multiple
      collapse-tags
      clearable
      :placeholder="$t('policy.allPhases')"
      style="width: 170px"
    >
      <el-option v-for="p in ['Ready', 'Invalid', 'Syncing']" :key="p" :label="p" :value="p" />
    </el-select>
    <el-select
      v-model="filters.mode"
      multiple
      collapse-tags
      clearable
      :placeholder="$t('policy.allModes')"
      style="width: 170px"
    >
      <el-option :label="$t('policy.enforce')" value="enforce" />
      <el-option :label="$t('policy.audit')" value="audit" />
    </el-select>
    <el-input v-model="filters.keyword" :placeholder="$t('common.search')" style="width: 200px" clearable />
    <el-button type="primary" class="toolbar-create" @click="$router.push('/policies/new')">
      {{ $t('common.create') }}
    </el-button>
  </div>

  <el-table :data="items" v-loading="loading">
    <el-table-column :label="$t('common.name')" prop="name" sortable min-width="180" show-overflow-tooltip>
      <template #default="{ row }">
        <router-link :to="`/policies/${row.name}`">{{ row.name }}</router-link>
      </template>
    </el-table-column>
    <el-table-column :label="$t('policy.source')" prop="source" sortable width="100">
      <template #default="{ row }">
        <el-tag size="small" :type="row.source === 'builtin' ? 'info' : 'primary'">{{ row.source }}</el-tag>
      </template>
    </el-table-column>
    <el-table-column :label="$t('policy.mode')" prop="enforcementMode" sortable width="115">
      <template #default="{ row }"><ModeTag :mode="row.enforcementMode" /></template>
    </el-table-column>
    <el-table-column :label="$t('policy.phase')" prop="phase" sortable width="100">
      <template #default="{ row }"><PhaseTag :phase="row.phase" /></template>
    </el-table-column>
    <el-table-column :label="$t('policy.currentVersion')" prop="currentVersion" sortable width="110">
      <template #default="{ row }">
        <el-button link type="primary" @click="$router.push(`/policies/${row.name}?tab=versions`)">
          v{{ row.currentVersion }}
        </el-button>
      </template>
    </el-table-column>
    <el-table-column :label="$t('policy.createdAt')" prop="createdAt" sortable width="155">
      <template #default="{ row }">
        {{ row.createdAt ? new Date(row.createdAt).toLocaleString() : '-' }}
      </template>
    </el-table-column>
    <el-table-column :label="$t('policy.updatedAt')" prop="updatedAt" sortable width="155">
      <template #default="{ row }">
        {{ row.updatedAt ? new Date(row.updatedAt).toLocaleString() : '-' }}
      </template>
    </el-table-column>
    <el-table-column :label="$t('policy.referencedBy')" min-width="125" show-overflow-tooltip>
      <template #default="{ row }">{{ (row.referencedBy ?? []).join(', ') }}</template>
    </el-table-column>
    <el-table-column :label="$t('common.actions')" width="130" fixed="right">
      <template #default="{ row }">
        <el-button link type="primary" @click="$router.push(`/policies/${row.name}/edit`)">
          {{ $t('common.edit') }}
        </el-button>
        <el-button link type="danger" @click="onDelete(row)">{{ $t('common.delete') }}</el-button>
      </template>
    </el-table-column>
  </el-table>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useI18n } from 'vue-i18n'
import { ApiError } from '../api/http'
import { deletePolicy, listPolicies, type PolicyListItem } from '../api/policy'
import ModeTag from '../components/ModeTag.vue'
import PhaseTag from '../components/PhaseTag.vue'

const KEYWORD_DEBOUNCE_MS = 300

const { t } = useI18n()
const items = ref<PolicyListItem[]>([])
const loading = ref(false)
const filters = reactive({
  source: [] as string[],
  phase: [] as string[],
  mode: [] as string[],
  keyword: '',
})

async function load() {
  loading.value = true
  try {
    const params: Record<string, string> = {}
    if (filters.source.length) params.source = filters.source.join(',')
    if (filters.phase.length) params.phase = filters.phase.join(',')
    if (filters.mode.length) params.mode = filters.mode.join(',')
    if (filters.keyword) params.keyword = filters.keyword
    items.value = await listPolicies(params)
  } catch (e) {
    ElMessage.error((e as Error).message)
  } finally {
    loading.value = false
  }
}

watch(() => [filters.source, filters.phase, filters.mode], load, { deep: true })

let keywordTimer: ReturnType<typeof setTimeout> | undefined
watch(
  () => filters.keyword,
  () => {
    clearTimeout(keywordTimer)
    keywordTimer = setTimeout(load, KEYWORD_DEBOUNCE_MS)
  },
)

async function onDelete(row: PolicyListItem) {
  try {
    await ElMessageBox.confirm(t('common.deleteConfirm', { name: row.name }))
    if (row.source === 'builtin') {
      await ElMessageBox.confirm(t('common.builtinWarning'))
    }
    await doDelete(row.name, false)
  } catch (e) {
    if (e === 'cancel') return
  }
}

async function doDelete(name: string, force: boolean) {
  try {
    await deletePolicy(name, force)
    ElMessage.success(t('common.deleted'))
    await load()
  } catch (e) {
    if (e instanceof ApiError && e.status === 409 && e.data) {
      const groups = ((e.data as { referencedBy?: string[] }).referencedBy ?? []).join(', ')
      try {
        await ElMessageBox.confirm(t('policy.referencedDeleteConfirm', { groups }))
        await doDelete(name, true)
      } catch {
        /* cancelled */
      }
      return
    }
    ElMessage.error((e as Error).message)
  }
}

onMounted(load)
</script>

<style scoped>
/* Wrap instead of squeezing the fixed-width filter controls on narrow screens. */
.list-toolbar {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
  margin-bottom: 12px;
}

.list-toolbar > * {
  flex-shrink: 0;
}

.toolbar-create {
  margin-left: auto;
}
</style>
