<script setup lang="ts">
import type { HTMLAttributes } from "vue"
import { cn } from "@/lib/utils"

const props = defineProps<{
  class?: HTMLAttributes["class"]
}>()
</script>

<template>
  <th
    data-slot="table-head"
    :class="
      cn(
        'text-left px-3 py-2.5 text-muted text-[11px] font-semibold uppercase tracking-wide border-b border-line whitespace-nowrap align-middle [&:has([role=checkbox])]:pr-0 *:[[role=checkbox]]:translate-y-0.5',
        props.class,
      )"
  >
    <slot />
  </th>
</template>
