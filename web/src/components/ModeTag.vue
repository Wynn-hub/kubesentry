<template>
  <el-tag :type="mode === 'enforce' ? 'danger' : 'warning'" size="small">
    {{ mode === 'enforce' ? $t('policy.enforce') : $t('policy.audit') }}
  </el-tag>
</template>

<script setup lang="ts">
defineProps<{ mode: string }>()
</script>
