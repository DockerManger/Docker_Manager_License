<template>
  <div class="space-y-4 fade-up">
    <PageHeader :title="$t('security.title')" :description="$t('security.description')" />

    <!-- 类型筛选 -->
    <div class="flex items-center gap-1 rounded-[0.5rem] border border-line bg-surface p-1 w-fit flex-wrap">
      <button
        v-for="f in typeFilters"
        :key="f.value"
        type="button"
        class="px-3 py-1.5 rounded-[0.35rem] text-[12.5px] font-medium transition-colors cursor-pointer"
        :class="eventType === f.value ? 'bg-surface2 text-text shadow-sm' : 'text-muted hover:text-text'"
        @click="load(1, f.value)"
      >
        {{ f.label }}
      </button>
    </div>

    <Card class="overflow-hidden">
      <div v-if="loading" class="p-4 space-y-2">
        <Skeleton v-for="i in 8" :key="i" class="h-10 w-full" />
      </div>
      <div v-else-if="error" class="p-12 text-center">
        <p class="text-danger text-[13.5px] mb-3">{{ $t('error.loadFailedWith', { what: $t('security.title'), msg: error }) }}</p>
        <Button variant="ghost" size="sm" @click="load(page, eventType)">
          <RefreshCw class="size-3.5" /> {{ $t('common.retry') }}
        </Button>
      </div>
      <template v-else>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{{ $t('security.time') }}</TableHead>
              <TableHead>{{ $t('security.type') }}</TableHead>
              <TableHead>{{ $t('security.license') }}</TableHead>
              <TableHead>{{ $t('security.device') }}</TableHead>
              <TableHead>{{ $t('security.ip') }}</TableHead>
              <TableHead>{{ $t('security.details') }}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            <TableRow v-for="e in items" :key="e.id">
              <TableCell class="text-muted text-[12px] whitespace-nowrap">{{ fmtDateTimeStr(e.created_at) }}</TableCell>
              <TableCell>
                <Badge :variant="eventVariant(e.event_type)">
                  <ShieldAlert class="size-3" />
                  {{ $t(`event.${e.event_type}`) }}
                </Badge>
              </TableCell>
              <TableCell class="font-mono text-[12px] text-muted">{{ e.license_id || '-' }}</TableCell>
              <TableCell class="font-mono text-[12px] text-muted">{{ shortId(e.device_id) || '-' }}</TableCell>
              <TableCell class="font-mono text-[12px] text-muted">{{ e.ip || '-' }}</TableCell>
              <TableCell class="text-muted text-[12px] max-w-[280px] truncate">{{ e.details || '-' }}</TableCell>
            </TableRow>
            <TableRow v-if="!items.length">
              <TableCell colspan="6">
                <EmptyState
                  :icon="ShieldCheck"
                  :title="$t('security.empty')"
                  :description="$t('security.emptyDesc')"
                />
              </TableCell>
            </TableRow>
          </TableBody>
        </Table>
        <div class="px-4 py-3 border-t border-line">
          <PaginationBar :page="page" :total="total" :page-size="pageSize" @change="(p) => load(p, eventType)" />
        </div>
      </template>
    </Card>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { RefreshCw, ShieldAlert, ShieldCheck } from '@lucide/vue'
import { Badge, type BadgeVariants } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import EmptyState from '../components/EmptyState.vue'
import PageHeader from '../components/PageHeader.vue'
import PaginationBar from '../components/PaginationBar.vue'
import { api, type SecurityEvent } from '../api'
import { fmtDateTimeStr, shortId } from '../lib/format'

const { t } = useI18n()
const items = ref<SecurityEvent[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = 20
const eventType = ref('')
const loading = ref(true)
const error = ref('')

const EVENT_TYPES = [
  'invalid_signature',
  'invalid_token',
  'rate_limit_exceeded',
  'replay_detected',
  'tampered_timestamp',
  'device_limit_exceeded',
  'client_version_blocked',
]

const typeFilters = computed(() => [
  { value: '', label: t('security.all') },
  ...EVENT_TYPES.map((v) => ({ value: v, label: t(`event.${v}`) })),
])

// 危险程度:重放/时间戳篡改 -> destructive;签名/token 无效 -> warning;限流/设备超限 -> info;版本封禁 -> destructive
function eventVariant(type: string): BadgeVariants['variant'] {
  switch (type) {
    case 'replay_detected':
    case 'tampered_timestamp':
    case 'client_version_blocked':
      return 'destructive'
    case 'invalid_signature':
    case 'invalid_token':
      return 'warning'
    default:
      return 'info'
  }
}

async function load(p: number, t: string) {
  page.value = p
  eventType.value = t
  loading.value = true
  error.value = ''
  try {
    const res = await api.get<{ items: SecurityEvent[]; total: number }>(
      `/api/v1/admin/security-events?page=${p}&page_size=${pageSize}${t ? `&type=${t}` : ''}`,
    )
    items.value = res.items || []
    total.value = res.total
  } catch (e: any) {
    error.value = e?.message || '加载失败'
  } finally {
    loading.value = false
  }
}

onMounted(() => load(1, ''))
</script>
