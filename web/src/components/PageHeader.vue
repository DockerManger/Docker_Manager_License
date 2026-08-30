<script setup lang="ts">
defineProps<{
  title: string
  description?: string
}>()
</script>

<template>
  <div class="flex items-start justify-between gap-4 mb-5 flex-wrap">
    <div>
      <h1 class="text-[18px] font-semibold text-text tracking-tight">{{ title }}</h1>
      <p v-if="description" class="text-[12.5px] text-muted mt-1">{{ description }}</p>
    </div>
    <div class="flex items-center gap-2 shrink-0">
      <slot name="actions" />
    </div>
  </div>
</template>
