<script setup lang="ts">
import { Moon, Sun } from '@lucide/vue'
import { theme, toggleThemeWithTransition } from '../lib/theme'

defineProps<{ compact?: boolean }>()
</script>

<template>
  <button
    type="button"
    class="inline-flex items-center justify-center rounded-md p-2 text-muted transition-colors hover:bg-surface2 hover:text-text"
    :title="theme === 'dark' ? '切换到亮色' : '切换到暗色'"
    :aria-label="theme === 'dark' ? '切换到亮色' : '切换到暗色'"
    @click="toggleThemeWithTransition($event)"
  >
    <Sun v-if="theme === 'dark'" class="size-4" />
    <Moon v-else class="size-4" />
  </button>
</template>
