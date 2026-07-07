<!-- web/src/views/OverviewView.vue -->
<template>
  <div v-if="data">
    <el-row :gutter="16">
      <el-col :span="8">
        <el-card @click="$router.push('/policies')" style="cursor: pointer">
          <div>{{ $t('overview.policies') }}</div>
          <div style="font-size: 32px; font-weight: 700">{{ data.totals.policies }}</div>
        </el-card>
      </el-col>
      <el-col :span="8">
        <el-card @click="$router.push('/policygroups')" style="cursor: pointer">
          <div>{{ $t('overview.groups') }}</div>
          <div style="font-size: 32px; font-weight: 700">{{ data.totals.policygroups }}</div>
        </el-card>
      </el-col>
      <el-col :span="8">
        <el-card @click="$router.push('/exceptions')" style="cursor: pointer">
          <div>{{ $t('overview.exceptions') }}</div>
          <div style="font-size: 32px; font-weight: 700">{{ data.totals.exceptions }}</div>
        </el-card>
      </el-col>
    </el-row>

    <el-row :gutter="16" style="margin-top: 16px">
      <el-col :span="12">
        <el-card>
          <template #header>{{ $t('overview.byPhase') }}</template>
          <p v-for="(n, phase) in data.policyPhases" :key="phase">
            <PhaseTag :phase="String(phase)" /> × {{ n }}
          </p>
        </el-card>
      </el-col>
      <el-col :span="12">
        <el-card>
          <template #header>{{ $t('overview.byMode') }}</template>
          <p v-for="(n, mode) in data.policyModes" :key="mode">
            <ModeTag :mode="String(mode)" /> × {{ n }}
          </p>
        </el-card>
      </el-col>
    </el-row>

    <el-card style="margin-top: 16px">
      <template #header>{{ $t('overview.attention') }}</template>
      <template v-if="invalidPolicies.length">
        <p v-for="p in invalidPolicies" :key="p.name">
          <router-link :to="`/policies/${p.name}`">{{ p.name }}</router-link>
          <PhaseTag :phase="p.phase" style="margin-left: 8px" />
        </p>
      </template>
      <p v-else>{{ $t('overview.noIssues') }}</p>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { getSummary, type SummaryData } from '../api/summary'
import { listPolicies, type PolicyListItem } from '../api/policy'
import PhaseTag from '../components/PhaseTag.vue'
import ModeTag from '../components/ModeTag.vue'

const data = ref<SummaryData>()
const invalidPolicies = ref<PolicyListItem[]>([])

onMounted(async () => {
  try {
    data.value = await getSummary()
    invalidPolicies.value = await listPolicies({ phase: 'Invalid' })
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
})
</script>
