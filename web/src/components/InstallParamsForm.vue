<!-- web/src/components/InstallParamsForm.vue -->
<template>
  <el-form :model="modelValue" label-width="140px">
    <el-form-item :label="$t('cluster.namespace')" required>
      <el-input v-model="modelValue.namespace" />
    </el-form-item>
    <el-form-item :label="$t('cluster.webhookImage')" required>
      <el-input v-model="modelValue.webhookImage.repository" :placeholder="$t('cluster.repository')" style="width: 45%; margin-right: 8px" />
      <el-input v-model="modelValue.webhookImage.tag" :placeholder="$t('cluster.tag')" style="width: 45%" />
    </el-form-item>
    <el-form-item :label="$t('cluster.operatorImage')" required>
      <el-input v-model="modelValue.operatorImage.repository" :placeholder="$t('cluster.repository')" style="width: 45%; margin-right: 8px" />
      <el-input v-model="modelValue.operatorImage.tag" :placeholder="$t('cluster.tag')" style="width: 45%" />
    </el-form-item>
    <el-form-item :label="$t('cluster.highAvailability')">
      <el-switch v-model="modelValue.highAvailability" />
      <span class="ha-hint">{{ $t('cluster.highAvailabilityHint') }}</span>
    </el-form-item>
    <el-form-item :label="$t('cluster.gatewayAddress')" required>
      <el-input v-model="modelValue.gatewayAddress" placeholder="<节点IP>:30443" />
    </el-form-item>
    <el-collapse>
      <el-collapse-item :title="$t('cluster.advancedOptions')">
        <el-form-item :label="$t('cluster.gatewayTlsServerName')">
          <el-input v-model="modelValue.gatewayTlsServerName" />
        </el-form-item>
        <el-form-item :label="$t('cluster.gatewayCaBundle')">
          <el-input v-model="modelValue.gatewayCaBundle" type="textarea" :rows="3" />
        </el-form-item>
      </el-collapse-item>
    </el-collapse>
  </el-form>
</template>

<script setup lang="ts">
import type { InstallParams } from '../api/cluster'

defineProps<{ modelValue: InstallParams }>()
</script>

<style scoped>
.ha-hint {
  margin-left: 10px;
  color: var(--el-text-color-secondary);
  font-size: 12px;
}
</style>
