<script setup lang="ts">
import { CheckCircle2, Info, XCircle } from '@lucide/vue'
import { toasts, dismiss } from '../lib/toast'

const icons = {
  success: CheckCircle2,
  error: XCircle,
  info: Info,
}
const colors = {
  success: 'text-ok',
  error: 'text-danger',
  info: 'text-info',
}
</script>

<template>
  <Teleport to="body">
    <div class="fixed top-4 right-4 z-[100] flex flex-col gap-2 w-[min(92vw,380px)]">
      <TransitionGroup
        enter-active-class="transition duration-200 ease-out"
        enter-from-class="opacity-0 translate-x-4"
        leave-active-class="transition duration-150 ease-in"
        leave-to-class="opacity-0 translate-x-4"
      >
        <div
          v-for="t in toasts"
          :key="t.id"
          class="flex items-start gap-2.5 rounded-lg border border-line bg-card px-4 py-3 shadow-lg shadow-black/20 fade-up"
        >
          <component :is="icons[t.type]" class="size-4 mt-0.5 shrink-0" :class="colors[t.type]" />
          <span class="text-[13px] leading-relaxed break-all flex-1">{{ t.message }}</span>
          <button
            class="text-muted hover:text-text transition-colors shrink-0"
            @click="dismiss(t.id)"
          >
            <XCircle class="size-3.5" />
          </button>
        </div>
      </TransitionGroup>
    </div>
  </Teleport>
</template>
