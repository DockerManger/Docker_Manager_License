<script setup lang="ts">
import { Moon, Sun } from '@lucide/vue'
import { Button } from '@/components/ui/button'
import { theme, toggleThemeWithTransition } from '../lib/theme'
</script>

<template>
  <Button
    variant="ghost"
    size="icon"
    :title="theme === 'dark' ? $t('theme.switchToLight') : $t('theme.switchToDark')"
    :aria-label="theme === 'dark' ? $t('theme.switchToLight') : $t('theme.switchToDark')"
    @click="toggleThemeWithTransition($event)"
  >
    <Sun v-if="theme === 'dark'" class="size-4" />
    <Moon v-else class="size-4" />
  </Button>
</template>
