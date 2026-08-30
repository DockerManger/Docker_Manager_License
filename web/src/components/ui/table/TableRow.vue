<script setup lang="ts">
import type { HTMLAttributes } from "vue"
import { cn } from "@/lib/utils"

const props = defineProps<{
  class?: HTMLAttributes["class"]
}>()
</script>

<template>
  <tr
    data-slot="table-row"
    :class="
      cn(
        'hover:bg-surface2/50 data-[state=selected]:bg-surface2 transition-colors',
        props.class,
      )"
  >
    <slot />
  </tr>
</template>
