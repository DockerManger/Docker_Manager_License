<script setup lang="ts">
import type { LucideIcon } from '@lucide/vue'

defineProps<{
  icon?: LucideIcon
  title: string
  description?: string
  actionLabel?: string
}>()
defineEmits<{ action: [] }>()
</script>

<template>
  <div class="flex flex-col items-center justify-center py-16 px-6 text-center">
    <div
      v-if="icon"
      class="w-12 h-12 rounded-xl bg-surface2 border border-line flex items-center justify-center text-muted mb-4"
    >
      <component :is="icon" class="size-5" />
    </div>
    <h3 class="text-[14.5px] font-semibold text-text">{{ title }}</h3>
    <p v-if="description" class="text-[12.5px] text-muted mt-1.5 max-w-sm leading-relaxed">
      {{ description }}
    </p>
    <slot />
    <button
      v-if="actionLabel"
      type="button"
      class="mt-5 inline-flex items-center gap-1.5 rounded-[0.5rem] bg-brand px-3.5 py-1.5 text-[13px] font-medium text-white hover:bg-brand-strong transition-colors"
      @click="$emit('action')"
    >
      {{ actionLabel }}
    </button>
  </div>
</template>
