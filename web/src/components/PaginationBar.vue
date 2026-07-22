<template>
  <div v-if="total > 0" class="pagination-bar">
    <span class="pagination-total mono-num">{{ $t('common.paginationTotal', { total }) }}</span>
    <el-pagination
      v-if="total > pageSize"
      class="pagination-controls"
      background
      layout="sizes, prev, pager, next"
      :current-page="page"
      :page-size="pageSize"
      :page-sizes="pageSizes"
      :total="total"
      @update:current-page="$emit('update:page', $event)"
      @update:page-size="$emit('update:pageSize', $event)"
    />
  </div>
</template>

<script setup lang="ts">
withDefaults(
  defineProps<{
    total: number
    page: number
    pageSize: number
    pageSizes?: number[]
  }>(),
  { pageSizes: () => [10, 20, 50, 100] },
)

defineEmits<{
  'update:page': [page: number]
  'update:pageSize': [pageSize: number]
}>()
</script>

<style scoped>
.pagination-bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  min-height: 32px;
  margin-top: 16px;
}
.pagination-total {
  color: var(--steel-300);
  font-size: 12px;
  white-space: nowrap;
}
@media (max-width: 720px) {
  .pagination-bar {
    align-items: flex-start;
    flex-direction: column;
  }
  .pagination-controls {
    max-width: 100%;
    overflow-x: auto;
  }
}
</style>
