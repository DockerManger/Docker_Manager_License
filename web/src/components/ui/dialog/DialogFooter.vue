<script setup lang="ts">
import type { HTMLAttributes } from "vue"
import { DialogClose } from "reka-ui"
import { cn } from "@/lib/utils"
import { Button } from '@/components/ui/button'

const props = defineProps<{
  class?: HTMLAttributes["class"]
  showCloseButton?: boolean
}>()
</script>

<template>
  <div
    data-slot="dialog-footer"
    :class="
      cn('flex items-center justify-between gap-3 px-5 py-3.5 border-t border-line bg-surface/60 rounded-b-lg', props.class)
    "
  >
    <slot />
    <DialogClose v-if="showCloseButton" as-child>
      <Button variant="ghost"> Close </Button>
    </DialogClose>
  </div>
</template>
