<script setup lang="ts">
import type { HTMLAttributes } from "vue"
import { cn } from "@/lib/utils"

const props = defineProps<{
  class?: HTMLAttributes["class"]
}>()
</script>

<template>
  <div
    data-slot="dialog-header"
    :class="cn('flex items-center justify-between gap-3 px-5 py-4 border-b border-line', props.class)"
  >
    <slot />
  </div>
</template>
