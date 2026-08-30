<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { ChevronLeft, ChevronRight } from '@lucide/vue'
import { Button } from '@/components/ui/button'

const props = defineProps<{
  page: number
  total: number
  pageSize: number
}>()

const emit = defineEmits<{ change: [page: number] }>()

const { t } = useI18n()

const totalPages = Math.max(1, Math.ceil(props.total / props.pageSize))
const from = props.total === 0 ? 0 : (props.page - 1) * props.pageSize + 1
const to = Math.min(props.page * props.pageSize, props.total)

function go(p: number) {
  if (p < 1 || p > totalPages || p === props.page) return
  emit('change', p)
}
</script>

<template>
  <div class="flex items-center justify-between gap-3 flex-wrap">
    <span class="text-[12.5px] text-muted">{{ t('common.totalItems', { total, from, to }) }}</span>
    <div class="flex items-center gap-1.5">
      <Button variant="ghost" size="sm" :disabled="page <= 1" @click="go(page - 1)">
        <ChevronLeft class="size-3.5" /> {{ t('common.prevPage') }}
      </Button>
      <span class="text-[12.5px] text-muted px-1.5">{{ t('common.pageOf', { page, pages: totalPages }) }}</span>
      <Button variant="ghost" size="sm" :disabled="page >= totalPages" @click="go(page + 1)">
        {{ t('common.nextPage') }} <ChevronRight class="size-3.5" />
      </Button>
    </div>
  </div>
</template>
