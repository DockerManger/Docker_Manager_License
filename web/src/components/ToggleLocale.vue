<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { Check, ChevronDown, Languages } from '@lucide/vue'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { Button } from '@/components/ui/button'
import { LANGS, setLang } from '../i18n'

const { locale } = useI18n()

const current = computed(() => LANGS.find((l) => l.code === locale.value) || LANGS[0])

function pick(code: string) {
  setLang(code)
}
</script>

<template>
  <DropdownMenu>
    <DropdownMenuTrigger as-child>
      <Button variant="ghost" size="icon" :title="$t('lang.switch')" :aria-label="$t('lang.switch')">
        <Languages class="size-4" />
      </Button>
    </DropdownMenuTrigger>
    <DropdownMenuContent align="end" class="w-52 max-h-80 overflow-y-auto">
      <DropdownMenuItem
        v-for="l in LANGS"
        :key="l.code"
        :class="l.code === current.code ? 'text-brand' : ''"
        @select="pick(l.code)"
      >
        <span class="mr-2">{{ l.flag }}</span>
        {{ l.label }}
        <Check v-if="l.code === current.code" class="size-3.5 ml-auto" />
      </DropdownMenuItem>
    </DropdownMenuContent>
  </DropdownMenu>
</template>
