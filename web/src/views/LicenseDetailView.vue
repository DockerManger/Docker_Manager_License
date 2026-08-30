<template>
  <div v-if="loading && !license" class="space-y-4">
    <Skeleton class="h-8 w-64" />
    <div class="grid grid-cols-2 lg:grid-cols-4 gap-4">
      <Skeleton v-for="i in 8" :key="i" class="h-24 rounded-lg" />
    </div>
  </div>

  <div v-else-if="error" class="rounded-lg border border-danger/30 bg-danger/10 p-10 text-center">
    <p class="text-danger text-[13.5px] mb-3">{{ $t('error.loadFailedWith', { what: $t('detail.title'), msg: error }) }}</p>
    <Button variant="ghost" size="sm" @click="load">
      <RefreshCw class="size-3.5" /> {{ $t('common.retry') }}
    </Button>
  </div>

  <div v-else-if="license" class="space-y-5 fade-up">
    <!-- 头部 -->
    <div class="flex items-center gap-3 flex-wrap">
      <Button variant="ghost" size="sm" @click="$router.push('/licenses')">
        <ChevronLeft class="size-4" /> {{ $t('detail.back') }}
      </Button>
      <div class="flex items-center gap-2.5 min-w-0">
        <h1 class="text-[17px] font-semibold font-mono text-text truncate">{{ license.license_id }}</h1>
        <StatusBadge :status="license.status" />
      </div>
      <div class="ml-auto flex items-center gap-2 flex-wrap">
        <DropdownMenu>
          <DropdownMenuTrigger as-child>
            <Button variant="ghost" size="sm">
              <Download class="size-3.5" /> {{ $t('common.export') }}
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end" class="w-44">
            <DropdownMenuItem @select="exportKey">
              <FileKey class="size-3.5" /> {{ $t('detail.exportLic') }}
            </DropdownMenuItem>
            <DropdownMenuItem @select="exportJson">
              <FileJson class="size-3.5" /> {{ $t('detail.exportJson') }}
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
        <Button v-if="license.status === 'active'" variant="ghost" size="sm" @click="showExtend = true">
          <Clock3 class="size-3.5" /> {{ $t('detail.extend') }}
        </Button>
        <Button v-if="license.status === 'active'" variant="destructive" size="sm" @click="openRevoke">
          <Ban class="size-3.5" /> {{ $t('detail.revoke') }}
        </Button>
      </div>
    </div>

    <!-- 信息卡 -->
    <div class="grid grid-cols-2 lg:grid-cols-4 gap-4">
      <Card v-for="info in infoCards" :key="info.label" class="p-4">
        <div class="flex items-center gap-1.5 text-[11.5px] text-muted mb-2">
          <component :is="info.icon" class="size-3.5" />
          {{ info.label }}
        </div>
        <div v-if="info.mono" class="font-mono text-[12.5px] text-text break-all">{{ info.value }}</div>
        <div v-else class="text-[14px] font-medium text-text" :class="info.valueClass">{{ info.value }}</div>
        <div v-if="info.sub" class="text-[11px] mt-1" :class="info.subClass">{{ info.sub }}</div>
      </Card>
    </div>

    <!-- 功能 -->
    <Card class="p-5">
      <h3 class="text-[13.5px] font-semibold mb-3">{{ $t('detail.features') }}</h3>
      <div class="flex flex-wrap gap-2">
        <Badge v-for="f in license.features" :key="f" variant="brand">
          <CheckCircle2 class="size-3" /> {{ FEATURE_LABELS[f] || f }}
        </Badge>
        <span v-if="!license.features?.length" class="text-[12.5px] text-muted">{{ $t('detail.noFeatures') }}</span>
      </div>
      <div v-if="license.notes" class="mt-4 pt-4 border-t border-line text-[12.5px] text-muted">
        <span class="text-text font-medium mr-2">{{ $t('detail.notes') }}:</span>{{ license.notes }}
      </div>
      <div v-if="license.revoked_reason" class="mt-4 pt-4 border-t border-line text-[12.5px] text-danger">
        <span class="font-medium mr-2">{{ $t('detail.revokedReason') }}:</span>{{ license.revoked_reason }}
      </div>
    </Card>

    <!-- 设备管理 -->
    <Card class="overflow-hidden">
      <div class="flex items-center justify-between px-5 py-4 border-b border-line">
        <h3 class="text-[13.5px] font-semibold flex items-center gap-2">
          <MonitorSmartphone class="size-4 text-muted" /> {{ $t('detail.devicesTitle') }}
          <span class="text-muted font-normal text-[12px]">{{ $t('detail.devicesCount', { active: activeCount, max: license.max_devices }) }}</span>
        </h3>
        <Button
          v-if="license.status === 'active' && activations.length"
          variant="ghost"
          size="sm"
          :disabled="busy"
          @click="openReset"
        >
          <RotateCcw class="size-3.5" /> {{ $t('detail.resetDevices') }}
        </Button>
      </div>
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>{{ $t('detail.deviceId') }}</TableHead>
            <TableHead>{{ $t('detail.deviceName') }}</TableHead>
            <TableHead>{{ $t('detail.version') }}</TableHead>
            <TableHead>{{ $t('detail.ip') }}</TableHead>
            <TableHead>{{ $t('detail.status') }}</TableHead>
            <TableHead>{{ $t('detail.activatedAt') }}</TableHead>
            <TableHead>{{ $t('detail.lastSeen') }}</TableHead>
            <TableHead class="w-16 text-right">{{ $t('common.actions') }}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          <TableRow v-for="a in activations" :key="a.id">
            <TableCell class="font-mono text-[12px] text-text">{{ a.device_id }}</TableCell>
            <TableCell class="text-text">{{ a.device_name || '-' }}</TableCell>
            <TableCell class="font-mono text-[12px] text-muted">{{ a.product_version || '-' }}</TableCell>
            <TableCell class="font-mono text-[12px] text-muted">{{ a.ip || '-' }}</TableCell>
            <TableCell><StatusBadge :status="a.status" /></TableCell>
            <TableCell class="text-muted text-[12px]">{{ fmtDateTimeStr(a.activated_at) }}</TableCell>
            <TableCell class="text-muted text-[12px]">{{ fmtDateTimeStr(a.last_seen_at) }}</TableCell>
            <TableCell class="text-right">
              <Button
                v-if="a.status === 'active'"
                variant="icon"
                size="sm"
                class="text-danger"
                :title="$t('detail.deactivate')"
                :disabled="busy"
                @click="openDeactivate(a)"
              >
                <Unlink class="size-3.5" />
              </Button>
            </TableCell>
          </TableRow>
          <TableRow v-if="!activations.length">
            <TableCell colspan="8">
              <EmptyState
                :icon="MonitorSmartphone"
                :title="$t('detail.noDevices')"
                :description="$t('detail.noDevicesDesc')"
              />
            </TableCell>
          </TableRow>
        </TableBody>
      </Table>
    </Card>

    <!-- 修订历史 -->
    <Card class="overflow-hidden">
      <div class="px-5 py-4 border-b border-line">
        <h3 class="text-[13.5px] font-semibold flex items-center gap-2">
          <History class="size-4 text-muted" /> {{ $t('detail.revisions') }}
          <span class="text-muted font-normal text-[12px]">{{ $t('detail.revisionsCount', { count: revisions.length }) }}</span>
        </h3>
      </div>
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead class="w-16">{{ $t('detail.revision') }}</TableHead>
            <TableHead>{{ $t('detail.reason') }}</TableHead>
            <TableHead>{{ $t('detail.by') }}</TableHead>
            <TableHead>{{ $t('detail.time') }}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          <TableRow v-for="r in revisions" :key="r.id">
            <TableCell class="font-mono text-[12px] text-muted">R{{ r.revision }}</TableCell>
            <TableCell class="text-text">{{ r.reason || 'issue' }}</TableCell>
            <TableCell class="text-muted">{{ r.created_by }}</TableCell>
            <TableCell class="text-muted text-[12px]">{{ fmtDateTimeStr(r.created_at) }}</TableCell>
          </TableRow>
          <TableRow v-if="!revisions.length">
            <TableCell colspan="4" class="text-center text-muted py-8">{{ $t('detail.noRevisions') }}</TableCell>
          </TableRow>
        </TableBody>
      </Table>
    </Card>

    <!-- 延期 Dialog -->
    <Dialog :open="showExtend" @update:open="(v) => !v && (showExtend = false)">
      <DialogContent class="sm:max-w-sm">
        <DialogHeader>
          <DialogTitle class="flex items-center gap-2">
            <Clock3 class="size-4 text-brand" /> {{ $t('detail.extendTitle') }}
          </DialogTitle>
        </DialogHeader>
        <div class="px-5 py-4 space-y-3">
          <div class="rounded-[0.5rem] border border-line bg-surface2/50 px-3.5 py-2.5 text-[12.5px]">
            <span class="text-muted mr-2">{{ $t('detail.currentExpiry') }}</span>
            <span class="text-text font-medium">{{ fmtDateTime(license.expires_at) }}</span>
          </div>
          <div>
            <Label>{{ $t('detail.extendDays') }}</Label>
            <Input v-model.number="extendDays" type="number" min="1" placeholder="365" />
          </div>
          <div>
            <Label>{{ $t('detail.extendReason') }} <span class="text-muted font-normal">({{ $t('detail.extendReasonHint') }})</span></Label>
            <Input v-model="extendReason" placeholder="renewal" />
          </div>
        </div>
        <DialogFooter>
          <Button variant="ghost" @click="showExtend = false">{{ $t('common.cancel') }}</Button>
          <Button variant="primary" :disabled="busy" @click="extend">
            <Loader2 v-if="busy" class="size-3.5 animate-spin" />
            {{ $t('detail.confirmExtend') }}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>

    <!-- 吊销确认 -->
    <AlertDialog :open="showRevoke" @update:open="(v) => !v && (showRevoke = false)">
      <AlertDialogContent class="sm:max-w-md">
        <div class="px-5 py-4">
          <div class="flex items-center gap-3 mb-3">
            <span class="w-9 h-9 rounded-xl bg-danger/15 text-danger flex items-center justify-center shrink-0">
              <Ban class="size-4" />
            </span>
            <AlertDialogTitle class="text-[14.5px] font-semibold">{{ $t('revoke.title') }}</AlertDialogTitle>
          </div>
          <AlertDialogDescription class="text-[12.5px] text-muted leading-relaxed">
            {{ $t('revoke.desc', { id: license.license_id }) }}
            <br />
            {{ $t('revoke.desc2') }}
          </AlertDialogDescription>
          <div class="mt-4">
            <Label>{{ $t('revoke.reason') }}</Label>
            <Select v-model="revokeReason">
              <SelectTrigger><SelectValue /></SelectTrigger>
              <SelectContent>
                <SelectItem value="Refund">{{ $t('revoke.reasonRefund') }}</SelectItem>
                <SelectItem value="Fraud">{{ $t('revoke.reasonFraud') }}</SelectItem>
                <SelectItem value="Abuse">{{ $t('revoke.reasonAbuse') }}</SelectItem>
                <SelectItem value="Other">{{ $t('revoke.reasonOther') }}</SelectItem>
              </SelectContent>
            </Select>
          </div>
        </div>
        <AlertDialogFooter>
          <AlertDialogCancel>{{ $t('common.cancel') }}</AlertDialogCancel>
          <AlertDialogAction variant="destructive" :disabled="busy" @click="revoke">
            <Loader2 v-if="busy" class="size-3.5 animate-spin" />
            {{ $t('revoke.confirm') }}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>

    <!-- 重置设备确认 -->
    <AlertDialog :open="showReset" @update:open="(v) => !v && (showReset = false)">
      <AlertDialogContent class="sm:max-w-md">
        <div class="px-5 py-4">
          <div class="flex items-center gap-3 mb-3">
            <span class="w-9 h-9 rounded-xl bg-warn/15 text-warn flex items-center justify-center shrink-0">
              <RotateCcw class="size-4" />
            </span>
            <AlertDialogTitle class="text-[14.5px] font-semibold">{{ $t('detail.resetTitle') }}</AlertDialogTitle>
          </div>
          <AlertDialogDescription class="text-[12.5px] text-muted leading-relaxed">
            {{ $t('detail.resetDesc', { count: activeCount }) }}
          </AlertDialogDescription>
        </div>
        <AlertDialogFooter>
          <AlertDialogCancel>{{ $t('common.cancel') }}</AlertDialogCancel>
          <AlertDialogAction variant="warning" :disabled="busy" @click="resetDevices">
            <Loader2 v-if="busy" class="size-3.5 animate-spin" />
            {{ $t('detail.resetConfirm') }}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>

    <!-- 解绑确认 -->
    <AlertDialog :open="!!deactivateTarget" @update:open="(v) => !v && (deactivateTarget = null)">
      <AlertDialogContent class="sm:max-w-md">
        <div class="px-5 py-4">
          <div class="flex items-center gap-3 mb-3">
            <span class="w-9 h-9 rounded-xl bg-danger/15 text-danger flex items-center justify-center shrink-0">
              <Unlink class="size-4" />
            </span>
            <AlertDialogTitle class="text-[14.5px] font-semibold">{{ $t('detail.deactivateTitle') }}</AlertDialogTitle>
          </div>
          <AlertDialogDescription class="text-[12.5px] text-muted leading-relaxed">
            {{ $t('detail.deactivateDesc', { id: deactivateTarget?.device_id }) }}
            <br />
            {{ $t('detail.deactivateDesc2') }}
          </AlertDialogDescription>
        </div>
        <AlertDialogFooter>
          <AlertDialogCancel>{{ $t('common.cancel') }}</AlertDialogCancel>
          <AlertDialogAction variant="destructive" :disabled="busy" @click="deactivate">
            <Loader2 v-if="busy" class="size-3.5 animate-spin" />
            {{ $t('detail.deactivateConfirm') }}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import {
  Ban, CalendarClock, ChevronLeft, Clock3, Download, FileJson, FileKey, Fingerprint,
  History, KeyRound, Loader2, MonitorSmartphone, RotateCcw, RefreshCw, Server, Unlink,
  User, type LucideIcon,
} from '@lucide/vue'
import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogTitle } from '@/components/ui/alert-dialog'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Skeleton } from '@/components/ui/skeleton'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import EmptyState from '../components/EmptyState.vue'
import StatusBadge from '../components/StatusBadge.vue'
import { api, FEATURE_LABELS, type Activation, type License, type LicenseRevision } from '../api'
import { fmtDateTime, fmtDateTimeStr } from '../lib/format'
import { toastErr, toastOk } from '../lib/toast'

const { t } = useI18n()
const route = useRoute()
const license = ref<License | null>(null)
const revisions = ref<LicenseRevision[]>([])
const activations = ref<Activation[]>([])
const loading = ref(true)
const error = ref('')
const busy = ref(false)

const showExtend = ref(false)
const extendDays = ref(365)
const extendReason = ref('renewal')

const showRevoke = ref(false)
const revokeReason = ref('Refund')

const showReset = ref(false)
const deactivateTarget = ref<Activation | null>(null)
// 非响应式"待解绑目标":reka-ui AlertDialogAction 点击时内部先关闭对话框
// (@update:open 同步把 deactivateTarget 清为 null),若 deactivate() 读响应式
// ref 会拿到 null 直接 return → 解绑请求从未发出(设备永远"有效")。
// 用普通变量承载目标,闭包捕获,不受对话框关闭影响。
let pendingDeactivate: Activation | null = null

const isExpired = computed(() => !!license.value && license.value.expires_at > 0 && license.value.expires_at * 1000 < Date.now())
const activeCount = computed(() => activations.value.filter((a) => a.status === 'active').length)

const infoCards = computed(() => {
  const l = license.value!
  return [
    { label: t('detail.customer'), icon: User, value: l.customer || '-', mono: false },
    { label: t('detail.plan'), icon: KeyRound, value: l.plan, mono: false },
    { label: t('detail.issuedAt'), icon: CalendarClock, value: fmtDateTime(l.issued_at), mono: false },
    {
      label: t('detail.expiresAt'), icon: Clock3, value: fmtDateTime(l.expires_at), mono: false,
      valueClass: isExpired.value && l.status !== 'revoked' ? 'text-danger' : undefined,
      sub: isExpired.value && l.status !== 'revoked' ? t('detail.expiredTag') : undefined,
      subClass: 'text-danger',
    },
    { label: t('detail.keyId'), icon: Fingerprint, value: l.key_id, mono: true },
    {
      label: t('detail.devices'), icon: MonitorSmartphone, value: `${l.active_devices ?? 0} / ${l.max_devices}`,
      mono: false,
      sub: (l.active_devices ?? 0) >= l.max_devices && l.max_devices > 0 ? t('detail.fullTag') : undefined,
      subClass: 'text-warn',
    },
    { label: t('detail.product'), icon: Server, value: l.product, mono: true },
    {
      label: t('detail.status'), icon: History, value: l.status === 'revoked' ? (l.revoked_reason || t('status.revoked')) : '-',
      mono: false,
      valueClass: l.status === 'revoked' ? 'text-danger' : undefined,
    },
  ] as Array<{ label: string; icon: LucideIcon; value: string; mono?: boolean; valueClass?: string; sub?: string; subClass?: string }>
})

async function load() {
  loading.value = true
  error.value = ''
  const id = route.params.id as string
  try {
    const [lic, revs, acts] = await Promise.all([
      api.get<{ license: License }>(`/api/v1/admin/licenses/${id}`),
      api.get<{ items: LicenseRevision[] }>(`/api/v1/admin/licenses/${id}/revisions`),
      api.get<{ items: Activation[] }>(`/api/v1/admin/licenses/${id}/activations`),
    ])
    license.value = lic.license
    revisions.value = revs.items || []
    activations.value = acts.items || []
  } catch (e: any) {
    error.value = e?.message || '加载失败'
  } finally {
    loading.value = false
  }
}

async function extend() {
  busy.value = true
  try {
    await api.post(`/api/v1/admin/licenses/${license.value!.license_id}/extend`, {
      days: extendDays.value,
      reason: extendReason.value,
    })
    toastOk(t('detail.extendSuccess'))
    showExtend.value = false
    load()
  } catch (e: any) {
    toastErr(e?.message || t('detail.extendFailed'))
  } finally {
    busy.value = false
  }
}

function openRevoke() {
  revokeReason.value = 'Refund'
  showRevoke.value = true
}

async function revoke() {
  busy.value = true
  try {
    await api.post(`/api/v1/admin/licenses/${license.value!.license_id}/revoke`, {
      reason: revokeReason.value,
    })
    toastOk(t('revoke.successToast'))
    showRevoke.value = false
    load()
  } catch (e: any) {
    toastErr(e?.message || t('revoke.failed'))
  } finally {
    busy.value = false
  }
}

function openReset() {
  showReset.value = true
}

async function resetDevices() {
  busy.value = true
  try {
    await api.post(`/api/v1/admin/licenses/${license.value!.license_id}/reset-devices`)
    toastOk(t('detail.resetSuccess'))
    showReset.value = false
    load()
  } catch (e: any) {
    toastErr(e?.message || t('detail.extendFailed'))
  } finally {
    busy.value = false
  }
}

function openDeactivate(a: Activation) {
  pendingDeactivate = a
  deactivateTarget.value = a
}

async function deactivate() {
  const target = pendingDeactivate
  if (!target) return
  busy.value = true
  try {
    await api.post(
      `/api/v1/admin/licenses/${license.value!.license_id}/activations/${target.id}/deactivate`,
    )
    toastOk(t('detail.deactivateSuccess'))
    pendingDeactivate = null
    deactivateTarget.value = null
    load()
  } catch (e: any) {
    toastErr(e?.message || t('detail.extendFailed'))
  } finally {
    busy.value = false
  }
}

async function exportKey() {
  try {
    const res = await fetch(`/api/v1/admin/licenses/${license.value!.license_id}/export`, {
      headers: { Authorization: `Bearer ${localStorage.getItem('dml_token') || ''}` },
    })
    if (!res.ok) throw new Error((await res.text()) || t('detail.exportFailed'))
    const blob = new Blob([await res.text()], { type: 'text/plain' })
    const a = document.createElement('a')
    a.href = URL.createObjectURL(blob)
    a.download = `${license.value!.license_id}.lic`
    a.click()
    URL.revokeObjectURL(a.href)
    toastOk(t('detail.exportSuccess'))
  } catch (e: any) {
    toastErr(e?.message || t('detail.exportFailed'))
  }
}

async function exportJson() {
  try {
    const res = await fetch(`/api/v1/admin/licenses/${license.value!.license_id}/export-json`, {
      headers: { Authorization: `Bearer ${localStorage.getItem('dml_token') || ''}` },
    })
    if (!res.ok) throw new Error((await res.text()) || t('detail.exportFailed'))
    const blob = new Blob([await res.text()], { type: 'application/json' })
    const a = document.createElement('a')
    a.href = URL.createObjectURL(blob)
    a.download = `${license.value!.license_id}.json`
    a.click()
    URL.revokeObjectURL(a.href)
    toastOk(t('detail.exportJsonSuccess'))
  } catch (e: any) {
    toastErr(e?.message || t('detail.exportFailed'))
  }
}

onMounted(load)
</script>
