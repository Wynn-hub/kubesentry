<template>
  <svg :width="size" :height="size" viewBox="0 0 64 64" aria-hidden="true">
    <path
      d="M32 6 L52 14 V29 C52 45 43 54 32 58 C21 54 12 45 12 29 V14 Z"
      fill="none"
      stroke="currentColor"
      stroke-width="4"
      stroke-linejoin="round"
    />
    <circle cx="32" cy="30" r="9" fill="#33d6c0" />
    <path d="M25 37 A9 9 0 0 0 39 37" fill="none" stroke="#ff6b4d" stroke-width="2.5" stroke-linecap="round" />
  </svg>
</template>

<script setup lang="ts">
withDefaults(defineProps<{ size?: number }>(), { size: 24 })
</script>
