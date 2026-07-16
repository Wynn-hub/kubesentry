<!-- web/src/components/HintMark.vue -->
<template>
  <el-tooltip :content="content" placement="right">
    <span class="hint-mark">?</span>
  </el-tooltip>
</template>

<script setup lang="ts">
defineProps<{ content: string }>()
</script>

<style scoped>
.hint-mark {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 16px;
  height: 16px;
  margin-left: 6px;
  border-radius: 50%;
  background: var(--el-color-info-light-5);
  color: var(--el-color-white);
  font-size: 12px;
  cursor: help;
  flex-shrink: 0;
}
</style>
