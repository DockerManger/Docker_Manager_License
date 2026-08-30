<template>
  <div class="space-y-6 fade-up">
    <!-- 统计卡 -->
    <div v-if="loading && !stats" class="grid grid-cols-2 lg:grid-cols-4 gap-4">
      <Skeleton v-for="i in 4" :key="i" class="h-[104px] rounded-lg" />
    </div>
    <div v-else-if="error" class="rounded-lg border border-danger/30 bg-danger/10 p-6 text-center">
      <p class="text-danger text-[13.5px] mb-3">{{ $t('error.loadFailedWith', { what: $t('dashboard.title'), msg: error }) }}</p>
      <Button variant="ghost" size="sm" @click="load">
        <RefreshCw class="size-3.5" /> {{ $t('common.retry') }}
      </Button>
    </div>
    <template v-else-if="stats">
      <div class="grid grid-cols-2 lg:grid-cols-4 gap-4">
        <Card v-for="s in statCards" :key="s.label" class="p-5">
          <div class="flex items-center justify-between">
            <span class="text-[12px] text-muted">{{ s.label }}</span>
            <span class="w-8 h-8 rounded-lg flex items-center justify-center" :class="s.iconBg">
              <component :is="s.icon" class="size-4" :class="s.iconColor" />
            </span>
          </div>
          <div class="text-[26px] font-bold tracking-tight mt-2">{{ s.value }}</div>
          <div class="text-[11.5px] mt-1" :class="s.subClass">{{ s.sub }}</div>
        </Card>
      </div>

      <!-- 中部:状态分布 + 即将过期 -->
      <div class="grid grid-cols-1 lg:grid-cols-3 gap-4">
        <Card class="p-5 lg:col-span-1">
          <h3 class="text-[13.5px] font-semibold mb-4">{{ $t('dashboard.statusDistribution') }}</h3>
          <div class="flex h-2.5 rounded-full overflow-hidden bg-surface2">
            <div
              v-for="seg in statusSegments"
              :key="seg.status"
              class="h-full transition-all duration-500"
              :style="{ width: seg.pct + '%', background: seg.color }"
            />
          </div>
          <div class="mt-4 space-y-2.5">
            <div v-for="seg in statusSegments" :key="seg.status" class="flex items-center gap-2.5 text-[12.5px]">
              <span class="w-2.5 h-2.5 rounded-sm" :style="{ background: seg.color }" />
              <span class="text-muted">{{ seg.label }}</span>
              <span class="ml-auto font-medium text-text">{{ seg.count }}</span>
              <span class="text-[11px] text-muted w-10 text-right">{{ seg.pct.toFixed(1) }}%</span>
            </div>
          </div>
        </Card>

        <Card class="p-5 lg:col-span-2">
          <div class="flex items-center justify-between mb-4">
            <h3 class="text-[13.5px] font-semibold">{{ $t('dashboard.expiringSoon') }}</h3>
            <router-link to="/licenses" class="text-[12px] text-brand hover:underline">{{ $t('dashboard.viewAll') }} →</router-link>
          </div>
          <div v-if="!expiring.length" class="text-center text-muted text-[12.5px] py-8">
            {{ $t('dashboard.expiringDays') }}
          </div>
          <div class="space-y-2" v-else>
            <div
              v-for="l in expiring"
              :key="l.id"
              class="flex items-center gap-3 rounded-[0.5rem] border border-line px-3.5 py-2.5 hover:bg-surface2/50 transition-colors"
            >
              <router-link :to="`/licenses/${l.license_id}`" class="font-mono text-[12px] text-brand hover:underline shrink-0">
                {{ l.license_id }}
              </router-link>
              <span class="text-text text-[13px] truncate">{{ l.customer || '-' }}</span>
              <span class="ml-auto text-[12px] shrink-0" :class="expiringIn(l.expires_at) < 7 ? 'text-danger font-medium' : 'text-warn'">
                {{ $t('dashboard.expiringWithin', { days: expiringIn(l.expires_at) }) }}
              </span>
            </div>
          </div>
        </Card>
      </div>

      <!-- 最近签发 + 最近活动 -->
      <div class="grid grid-cols-1 lg:grid-cols-2 gap-4">
        <Card class="overflow-hidden">
          <div class="flex items-center justify-between px-5 py-4 border-b border-line">
            <h3 class="text-[13.5px] font-semibold">{{ $t('dashboard.recentIssued') }}</h3>
            <router-link to="/licenses" class="text-[12px] text-brand hover:underline">{{ $t('dashboard.viewAll') }} →</router-link>
          </div>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{{ $t('licenses.title') }}</TableHead>
                <TableHead>{{ $t('licenses.customer') }}</TableHead>
                <TableHead>{{ $t('licenses.status') }}</TableHead>
                <TableHead>{{ $t('licenses.expires') }}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              <TableRow v-for="l in recent" :key="l.id" class="cursor-pointer" @click="$router.push(`/licenses/${l.license_id}`)">
                <TableCell class="font-mono text-[12px]">
                  <span class="text-brand hover:underline">{{ l.license_id }}</span>
                </TableCell>
                <TableCell class="text-text">{{ l.customer || '-' }}</TableCell>
                <TableCell><StatusBadge :status="l.status" /></TableCell>
                <TableCell class="text-muted text-[12px]">{{ fmtDate(l.expires_at) }}</TableCell>
              </TableRow>
              <TableRow v-if="!recent.length">
                <TableCell colspan="4" class="text-center text-muted py-8">{{ $t('common.noData') }}</TableCell>
              </TableRow>
            </TableBody>
          </Table>
        </Card>

        <Card class="p-5">
          <div class="flex items-center justify-between mb-4">
            <h3 class="text-[13.5px] font-semibold">{{ $t('dashboard.recentActivity') }}</h3>
            <router-link to="/audit" class="text-[12px] text-brand hover:underline">{{ $t('dashboard.viewAll') }} →</router-link>
          </div>
          <div v-if="!activities.length" class="text-center text-muted text-[12.5px] py-8">{{ $t('dashboard.noActivity') }}</div>
          <div class="space-y-1" v-else>
            <div
              v-for="a in activities"
              :key="a.id"
              class="flex items-start gap-3 py-2.5 border-b border-line last:border-0"
            >
              <span class="w-6 h-6 rounded-md flex items-center justify-center shrink-0 mt-0.5" :class="actionMeta(a.action).bg">
                <component :is="actionMeta(a.action).icon" class="size-3" :class="actionMeta(a.action).color" />
              </span>
              <div class="min-w-0 flex-1">
                <div class="text-[13px] text-text leading-snug">
                  <span class="font-medium">{{ a.admin || 'admin' }}</span>
                  <span class="text-muted"> {{ actionLabel(a.action) }}</span>
                  <span v-if="a.resource_id" class="font-mono text-[12px] text-muted"> {{ a.resource_id }}</span>
                </div>
                <div class="text-[11.5px] text-muted mt-0.5">{{ fmtDateTimeStr(a.created_at) }}</div>
              </div>
            </div>
          </div>
        </Card>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  AlertTriangle, CheckCircle2, Clock3, KeyRound, RefreshCw, ShieldX, TicketX, UserPlus,
  type LucideIcon,
} from '@lucide/vue'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import StatusBadge from '../components/StatusBadge.vue'
import { api, type AuditLog, type License } from '../api'
import { fmtDate, fmtDateTimeStr } from '../lib/format'

const { t } = useI18n()
const stats = ref<any>(null)
const recent = ref<License[]>([])
const expiring = ref<License[]>([])
const activities = ref<AuditLog[]>([])
const loading = ref(true)
const error = ref('')

function byStatus(s: string): number {
  return stats.value?.by_status?.[s] || 0
}

const total = computed(() => stats.value?.total || 0)

const statCards = computed(() => {
  const active = byStatus('active')
  const expired = byStatus('expired')
  const revoked = byStatus('revoked')
  const activePct = total.value ? Math.round((active / total.value) * 100) : 0
  return [
    {
      label: t('dashboard.totalLicenses'),
      value: total.value,
      icon: KeyRound,
      iconBg: 'bg-brand/12',
      iconColor: 'text-brand',
      sub: t('dashboard.activePct', { pct: activePct }),
      subClass: 'text-ok',
    },
    {
      label: t('dashboard.active'),
      value: active,
      icon: CheckCircle2,
      iconBg: 'bg-ok/12',
      iconColor: 'text-ok',
      sub: t('dashboard.canActivate'),
      subClass: 'text-muted',
    },
    {
      label: t('dashboard.expired'),
      value: expired,
      icon: Clock3,
      iconBg: 'bg-warn/12',
      iconColor: 'text-warn',
      sub: expiring.value.length
        ? t('dashboard.nearExpiry', { count: expiring.value.length })
        : t('dashboard.noNearExpiry'),
      subClass: expiring.value.length ? 'text-warn' : 'text-muted',
    },
    {
      label: t('dashboard.revoked'),
      value: revoked,
      icon: ShieldX,
      iconBg: 'bg-danger/12',
      iconColor: 'text-danger',
      sub: t('dashboard.instantlyBlocked'),
      subClass: 'text-muted',
    },
  ]
})

const STATUS_META: Record<string, { label: string; color: string }> = {
  active: { label: 'active', color: '#34d399' },
  expired: { label: 'expired', color: '#fbbf24' },
  revoked: { label: 'revoked', color: '#f87171' },
  suspended: { label: 'suspended', color: '#fb923c' },
}

const statusSegments = computed(() => {
  const entries = Object.entries(stats.value?.by_status || {})
  const known = entries.filter(([s]) => STATUS_META[s])
  const totalCount = known.reduce((acc, [, n]) => acc + (n as number), 0) || 1
  return known.map(([status, count]) => ({
    status,
    count: count as number,
    label: t(`status.${STATUS_META[status].label}`),
    color: STATUS_META[status].color,
    pct: ((count as number) / totalCount) * 100,
  }))
})

function expiringIn(ts: number): number {
  return Math.ceil((ts * 1000 - Date.now()) / 86400000)
}

// 活动动作 -> 图标/颜色
const ACTION_META: Record<string, { icon: LucideIcon; bg: string; color: string }> = {
  'license.create': { icon: KeyRound, bg: 'bg-brand/12', color: 'text-brand' },
  'license.revoke': { icon: ShieldX, bg: 'bg-danger/12', color: 'text-danger' },
  'license.extend': { icon: Clock3, bg: 'bg-info/12', color: 'text-info' },
  'license.activate': { icon: CheckCircle2, bg: 'bg-ok/12', color: 'text-ok' },
  'customer.create': { icon: UserPlus, bg: 'bg-info/12', color: 'text-info' },
  'subscription.create': { icon: TicketX, bg: 'bg-brand/12', color: 'text-brand' },
}

function actionMeta(action: string) {
  return ACTION_META[action] || { icon: AlertTriangle, bg: 'bg-surface2', color: 'text-muted' }
}
function actionLabel(action: string) {
  const key = `action.${action}`
  return t(key) !== key ? t(key) : action
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    const [st, page, audits] = await Promise.all([
      api.get<any>('/api/v1/admin/stats'),
      api.get<{ items: License[] }>('/api/v1/admin/licenses?page=1&page_size=100'),
      api.get<{ items: AuditLog[] }>('/api/v1/admin/audit-logs?page=1&page_size=6'),
    ])
    stats.value = st
    recent.value = (page.items || []).slice(0, 8)
    expiring.value = (page.items || [])
      .filter((l) => l.status === 'active' && l.expires_at > 0 && expiringIn(l.expires_at) <= 30)
      .sort((a, b) => a.expires_at - b.expires_at)
      .slice(0, 6)
    activities.value = audits.items || []
  } catch (e: any) {
    error.value = e?.message || '加载失败'
  } finally {
    loading.value = false
  }
}

onMounted(load)
</script>
