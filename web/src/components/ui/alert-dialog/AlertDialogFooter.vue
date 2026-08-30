<script setup lang="ts">
import type { HTMLAttributes } from "vue"
import { cn } from "@/lib/utils"

const props = defineProps<{
  class?: HTMLAttributes["class"]
}>()
</script>

<template>
  <div
    data-slot="alert-dialog-footer"
    :class="
      cn(
        'flex flex-col-reverse gap-3 px-5 pb-5 sm:flex-row sm:justify-between sm:px-6',
        props.class,
      )
    "
  >
    <slot />
  </div>
</template>
