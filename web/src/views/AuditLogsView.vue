<template>
  <div class="space-y-4 fade-up">
    <PageHeader :title="$t('audit.title')" :description="$t('audit.description')" />

    <Card class="overflow-hidden">
      <div v-if="loading" class="p-4 space-y-2">
        <Skeleton v-for="i in 8" :key="i" class="h-10 w-full" />
      </div>
      <div v-else-if="error" class="p-12 text-center">
        <p class="text-danger text-[13.5px] mb-3">{{ $t('error.loadFailedWith', { what: $t('audit.title'), msg: error }) }}</p>
        <Button variant="ghost" size="sm" @click="load(page)">
          <RefreshCw class="size-3.5" /> {{ $t('common.retry') }}
        </Button>
      </div>
      <template v-else>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{{ $t('audit.time') }}</TableHead>
              <TableHead>{{ $t('audit.operator') }}</TableHead>
              <TableHead>{{ $t('audit.action') }}</TableHead>
              <TableHead>{{ $t('audit.resource') }}</TableHead>
              <TableHead>{{ $t('audit.ip') }}</TableHead>
              <TableHead>{{ $t('audit.details') }}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            <TableRow v-for="e in items" :key="e.id">
              <TableCell class="text-muted text-[12px] whitespace-nowrap">{{ fmtDateTimeStr(e.created_at) }}</TableCell>
              <TableCell class="text-text">{{ e.admin || '-' }}</TableCell>
              <TableCell>
                <Badge :variant="actionVariant(e.action)">
                  {{ e.action }}
                </Badge>
              </TableCell>
              <TableCell class="font-mono text-[12px] text-muted">{{ e.resource_id || '-' }}</TableCell>
              <TableCell class="font-mono text-[12px] text-muted">{{ e.ip || '-' }}</TableCell>
              <TableCell class="text-muted text-[12px] max-w-[300px] truncate">{{ prettyMeta(e.metadata) }}</TableCell>
            </TableRow>
            <TableRow v-if="!items.length">
              <TableCell colspan="6">
                <EmptyState
                  :icon="ScrollText"
                  :title="$t('audit.empty')"
                  :description="$t('audit.emptyDesc')"
                />
              </TableCell>
            </TableRow>
          </TableBody>
        </Table>
        <div class="px-4 py-3 border-t border-line">
          <PaginationBar :page="page" :total="total" :page-size="pageSize" @change="load" />
        </div>
      </template>
    </Card>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { RefreshCw, ScrollText } from '@lucide/vue'
import { Badge, type BadgeVariants } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import EmptyState from '../components/EmptyState.vue'
import PageHeader from '../components/PageHeader.vue'
import PaginationBar from '../components/PaginationBar.vue'
import { api, type AuditLog } from '../api'
import { fmtDateTimeStr } from '../lib/format'

const items = ref<AuditLog[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = 30
const loading = ref(true)
const error = ref('')

function actionVariant(action: string): BadgeVariants['variant'] {
  if (action.includes('revoke') || action.includes('deactivate')) return 'destructive'
  if (action.includes('create')) return 'success'
  if (action.includes('extend') || action.includes('status') || action.includes('update')) return 'info'
  return 'default'
}

function prettyMeta(m: string): string {
  if (!m) return '-'
  try {
    const obj = JSON.parse(m)
    return Object.entries(obj)
      .map(([k, v]) => `${k}=${v}`)
      .join(' ')
  } catch {
    return m
  }
}

async function load(p: number) {
  page.value = p
  loading.value = true
  error.value = ''
  try {
    const res = await api.get<{ items: AuditLog[]; total: number }>(
      `/api/v1/admin/audit-logs?page=${p}&page_size=${pageSize}`,
    )
    items.value = res.items || []
    total.value = res.total
  } catch (e: any) {
    error.value = e?.message || '加载失败'
  } finally {
    loading.value = false
  }
}

onMounted(() => load(1))
</script>
