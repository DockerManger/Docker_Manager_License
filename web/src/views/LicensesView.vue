<template>
  <div class="space-y-4 fade-up">
    <PageHeader :title="$t('licenses.title')" :description="$t('licenses.description')">
      <template #actions>
        <Button variant="primary" @click="openCreate">
          <Plus class="size-4" /> {{ $t('licenses.issue') }}
        </Button>
      </template>
    </PageHeader>

    <!-- 筛选栏 -->
    <div class="flex items-center gap-3 flex-wrap">
      <div class="flex items-center gap-1 rounded-[0.5rem] border border-line bg-surface p-1">
        <button
          v-for="s in statusFilters"
          :key="s.value"
          type="button"
          class="px-3 py-1.5 rounded-[0.35rem] text-[12.5px] font-medium transition-colors cursor-pointer"
          :class="status === s.value ? 'bg-surface2 text-text shadow-sm' : 'text-muted hover:text-text'"
          @click="load(1, s.value)"
        >
          {{ $t(s.labelKey) }}
        </button>
      </div>
      <div class="relative ml-auto">
        <Search class="absolute left-3 top-1/2 -translate-y-1/2 size-3.5 text-muted" />
        <Input
          v-model="keyword"
          class="!w-56 !pl-9"
          :placeholder="$t('licenses.searchPlaceholder')"
        />
      </div>
    </div>

    <!-- 表格 -->
    <Card class="overflow-hidden">
      <div v-if="loading" class="p-4 space-y-2">
        <Skeleton v-for="i in 8" :key="i" class="h-10 w-full" />
      </div>
      <template v-else-if="error">
        <div class="p-12 text-center">
          <p class="text-danger text-[13.5px] mb-3">{{ $t('error.loadFailedWith', { what: $t('licenses.title'), msg: error }) }}</p>
          <Button variant="ghost" size="sm" @click="load(page, status)">
            <RefreshCw class="size-3.5" /> {{ $t('common.retry') }}
          </Button>
        </div>
      </template>
      <template v-else>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>License</TableHead>
              <TableHead>{{ $t('licenses.customer') }}</TableHead>
              <TableHead>{{ $t('licenses.plan') }}</TableHead>
              <TableHead>{{ $t('licenses.status') }}</TableHead>
              <TableHead>{{ $t('licenses.devices') }}</TableHead>
              <TableHead>{{ $t('licenses.issued') }}</TableHead>
              <TableHead>{{ $t('licenses.expires') }}</TableHead>
              <TableHead class="w-12 text-right">{{ $t('common.actions') }}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            <TableRow v-for="l in filtered" :key="l.id" class="cursor-pointer" @click="$router.push(`/licenses/${l.license_id}`)">
              <TableCell class="font-mono text-[12px]">
                <span class="text-brand hover:underline">{{ l.license_id }}</span>
              </TableCell>
              <TableCell class="text-text">{{ l.customer || '-' }}</TableCell>
              <TableCell>
                <Badge variant="outline">{{ l.plan }}</Badge>
              </TableCell>
              <TableCell><StatusBadge :status="l.status" /></TableCell>
              <TableCell>
                <span class="inline-flex items-center gap-1.5 text-[12.5px]" :class="isFull(l) ? 'text-warn' : 'text-text'">
                  <MonitorSmartphone class="size-3.5 text-muted" />
                  {{ l.active_devices ?? 0 }} / {{ l.max_devices }}
                </span>
              </TableCell>
              <TableCell class="text-muted text-[12px]">{{ fmtDate(l.issued_at) }}</TableCell>
              <TableCell class="text-muted text-[12px]" :class="{ '!text-danger font-medium': isExpired(l) }">
                {{ fmtDate(l.expires_at) }}
              </TableCell>
              <TableCell @click.stop>
                <DropdownMenu>
                  <DropdownMenuTrigger as-child>
                    <Button variant="icon" size="sm">
                      <MoreHorizontal class="size-4" />
                    </Button>
                  </DropdownMenuTrigger>
                  <DropdownMenuContent align="end" class="w-44">
                    <DropdownMenuItem @select="$router.push(`/licenses/${l.license_id}`)">
                      <Eye class="size-3.5" /> {{ $t('common.view') }}
                    </DropdownMenuItem>
                    <DropdownMenuItem @select="exportKey(l)">
                      <Download class="size-3.5" /> {{ $t('detail.exportLic') }}
                    </DropdownMenuItem>
                    <DropdownMenuSeparator />
                    <DropdownMenuItem v-if="l.status === 'active'" variant="destructive" @select="openRevoke(l)">
                      <Ban class="size-3.5" /> {{ $t('detail.revoke') }}
                    </DropdownMenuItem>
                  </DropdownMenuContent>
                </DropdownMenu>
              </TableCell>
            </TableRow>
            <TableRow v-if="!filtered.length">
              <TableCell colspan="8">
                <EmptyState
                  :icon="KeyRound"
                  :title="keyword ? $t('licenses.noMatch') : $t('licenses.empty')"
                  :description="keyword ? $t('licenses.noMatchDesc') : $t('licenses.emptyDesc')"
                  :action-label="$t('licenses.issue')"
                  @action="openCreate"
                />
              </TableCell>
            </TableRow>
          </TableBody>
        </Table>
        <div class="px-4 py-3 border-t border-line">
          <PaginationBar :page="page" :total="total" :page-size="pageSize" @change="(p) => load(p, status)" />
        </div>
      </template>
    </Card>

    <!-- 签发 Dialog -->
    <Dialog :open="showCreate" @update:open="(v) => !v && closeCreate()">
      <DialogContent class="sm:max-w-xl">
        <DialogHeader>
          <DialogTitle class="flex items-center gap-2">
            <KeyRound class="size-4 text-brand" /> {{ $t('issue.title') }}
          </DialogTitle>
          <DialogDescription v-if="!issued" class="text-[12px] text-muted">
            {{ $t('issue.desc') }}
          </DialogDescription>
        </DialogHeader>

        <form v-if="!issued" class="px-5 py-4 space-y-4" @submit.prevent="create">
          <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
            <div>
              <Label>{{ $t('issue.customer') }}</Label>
              <Input v-model="form.customer" :placeholder="$t('issue.customer')" required />
            </div>
            <div>
              <Label>{{ $t('issue.plan') }}</Label>
              <Select v-model="form.plan">
                <SelectTrigger><SelectValue /></SelectTrigger>
                <SelectContent>
                  <SelectItem v-for="p in PLANS" :key="p" :value="p">{{ p }}</SelectItem>
                </SelectContent>
              </Select>
            </div>
          </div>

          <div>
            <Label>{{ $t('issue.features') }}</Label>
            <div class="flex flex-wrap gap-2">
              <label
                v-for="f in FEATURES"
                :key="f"
                class="flex items-center gap-1.5 px-3 py-1.5 rounded-[0.5rem] border cursor-pointer text-[12.5px] select-none transition-colors"
                :class="form.features.includes(f) ? 'border-brand/50 bg-brand/10 text-brand' : 'border-line text-muted hover:text-text'"
              >
                <Checkbox
                  :checked="form.features.includes(f)"
                  class="size-3.5"
                  @update:checked="(v: boolean | 'indeterminate') => toggleFeature(f, v === true)"
                />
                {{ FEATURE_LABELS[f] }}
              </label>
            </div>
          </div>

          <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
            <div>
              <Label>{{ $t('issue.expireDays') }}</Label>
              <Input v-model.number="form.expireDays" type="number" min="1" placeholder="365" required />
            </div>
            <div>
              <Label>{{ $t('issue.maxDevices') }}</Label>
              <Input v-model.number="form.maxDevices" type="number" min="1" value="1" />
            </div>
          </div>

          <div>
            <Label>{{ $t('issue.notes') }} <span class="text-muted font-normal">{{ $t('issue.notesOptional') }}</span></Label>
            <Input v-model="form.notes" :placeholder="$t('issue.notesHint')" />
          </div>

          <p v-if="formErr" class="text-danger text-[12.5px] flex items-center gap-1.5">
            <AlertCircle class="size-3.5 shrink-0" /> {{ formErr }}
          </p>

          <DialogFooter>
            <Button type="button" variant="ghost" @click="closeCreate">{{ $t('common.cancel') }}</Button>
            <Button type="submit" variant="primary" :disabled="busy || !form.customer">
              <Loader2 v-if="busy" class="size-3.5 animate-spin" />
              <KeyRound v-else class="size-3.5" />
              {{ busy ? $t('issue.busy') : $t('issue.submit') }}
            </Button>
          </DialogFooter>
        </form>

        <!-- 签发结果 -->
        <div v-else class="px-5 py-4">
          <div class="rounded-[0.5rem] border border-ok/30 bg-ok/10 p-3.5 mb-4 flex items-start gap-2.5">
            <CheckCircle2 class="size-4 text-ok shrink-0 mt-0.5" />
            <div class="text-[12.5px] text-text">
              {{ $t('issue.success') }}
              <span class="text-muted">{{ $t('issue.keyOnce') }}</span>
            </div>
          </div>
          <Label>{{ $t('issue.keyLabel') }}</Label>
          <div class="relative">
            <Textarea
              :model-value="issued.key"
              readonly
              rows="4"
              class="font-mono text-[11px] pr-10"
              @click="(e: any) => (e.target as HTMLTextAreaElement).select()"
            />
            <Button
              variant="ghost"
              size="icon-sm"
              class="absolute top-1.5 right-1.5"
              :title="$t('common.copy')"
              @click="copyKey"
            >
              <Copy class="size-3.5" />
            </Button>
          </div>
          <div class="flex gap-2 mt-4">
            <Button variant="ghost" size="sm" class="flex-1" @click="closeCreate">{{ $t('issue.done') }}</Button>
            <a
              :href="`data:text/plain;charset=utf-8,${encodeURIComponent(issued.key)}`"
              download="license.lic"
              class="inline-flex items-center justify-center gap-1.5 flex-1 rounded-[0.5rem] border border-line px-3 py-1.5 text-[13px] font-medium text-text hover:bg-surface2 transition-colors"
            >
              <Download class="size-3.5" /> {{ $t('issue.downloadLic') }}
            </a>
          </div>
        </div>
      </DialogContent>
    </Dialog>

    <!-- 吊销确认 -->
    <AlertDialog :open="!!revokeTarget" @update:open="(v) => !v && (revokeTarget = null)">
      <AlertDialogContent class="sm:max-w-md">
        <div class="px-5 py-4">
          <div class="flex items-center gap-3 mb-3">
            <span class="w-9 h-9 rounded-xl bg-danger/15 text-danger flex items-center justify-center shrink-0">
              <Ban class="size-4" />
            </span>
            <AlertDialogTitle class="text-[14.5px] font-semibold">{{ $t('revoke.title') }}</AlertDialogTitle>
          </div>
          <AlertDialogDescription class="text-[12.5px] text-muted leading-relaxed">
            {{ $t('revoke.desc', { id: revokeTarget?.license_id }) }}
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
          <AlertDialogAction variant="destructive" :disabled="revokeBusy" @click="confirmRevoke">
            <Loader2 v-if="revokeBusy" class="size-3.5 animate-spin" />
            {{ $t('revoke.confirm') }}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  AlertCircle, Ban, CheckCircle2, Copy, Download, Eye, KeyRound, Loader2,
  MonitorSmartphone, MoreHorizontal, Plus, RefreshCw, Search,
} from '@lucide/vue'
import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogTitle } from '@/components/ui/alert-dialog'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Checkbox } from '@/components/ui/checkbox'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Skeleton } from '@/components/ui/skeleton'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { Textarea } from '@/components/ui/textarea'
import EmptyState from '../components/EmptyState.vue'
import PageHeader from '../components/PageHeader.vue'
import PaginationBar from '../components/PaginationBar.vue'
import StatusBadge from '../components/StatusBadge.vue'
import { api, FEATURES, FEATURE_LABELS, PLANS, type License } from '../api'
import { fmtDate } from '../lib/format'
import { toastErr, toastOk } from '../lib/toast'

const items = ref<License[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = 20
const status = ref('')
const keyword = ref('')
const loading = ref(true)
const error = ref('')

const statusFilters = [
  { value: '', labelKey: 'licenses.all' },
  { value: 'active', labelKey: 'licenses.active' },
  { value: 'expired', labelKey: 'licenses.expired' },
  { value: 'revoked', labelKey: 'licenses.revoked' },
]

const filtered = computed(() => {
  if (!keyword.value) return items.value
  const k = keyword.value.toLowerCase()
  return items.value.filter(
    (l) => l.customer?.toLowerCase().includes(k) || l.license_id.toLowerCase().includes(k),
  )
})

function isFull(l: License) {
  return (l.active_devices ?? 0) >= l.max_devices
}
function isExpired(l: License) {
  return l.expires_at > 0 && l.expires_at * 1000 < Date.now() && l.status !== 'revoked'
}

async function load(p: number, s: string) {
  page.value = p
  status.value = s
  loading.value = true
  error.value = ''
  try {
    const res = await api.get<{ items: License[]; total: number }>(
      `/api/v1/admin/licenses?page=${p}&page_size=${pageSize}${s ? `&status=${s}` : ''}`,
    )
    items.value = res.items || []
    total.value = res.total
  } catch (e: any) {
    error.value = e?.message || '加载失败'
  } finally {
    loading.value = false
  }
}

// ---------- 签发 ----------
const showCreate = ref(false)
const busy = ref(false)
const formErr = ref('')
const issued = ref<{ key: string } | null>(null)
const form = reactive({
  customer: '',
  plan: 'pro',
  features: [...FEATURES] as string[],
  expireDays: 365,
  maxDevices: 1,
  notes: '',
})

function toggleFeature(f: string, on: boolean) {
  if (on) {
    if (!form.features.includes(f)) form.features.push(f)
  } else {
    form.features = form.features.filter((x) => x !== f)
  }
}

function openCreate() {
  showCreate.value = true
  issued.value = null
  formErr.value = ''
}

function closeCreate() {
  showCreate.value = false
  issued.value = null
}

async function create() {
  busy.value = true
  formErr.value = ''
  try {
    const res = await api.post<{ key: string }>('/api/v1/admin/licenses', {
      customer: form.customer,
      plan: form.plan,
      features: form.features,
      expire_days: form.expireDays,
      max_devices: form.maxDevices,
      notes: form.notes,
    })
    issued.value = res
    toastOk(t('issue.successToast'))
    load(1, status.value)
  } catch (e: any) {
    formErr.value = e?.message || '签发失败'
  } finally {
    busy.value = false
  }
}

async function copyKey() {
  if (!issued.value) return
  try {
    await navigator.clipboard.writeText(issued.value.key)
    toastOk(t('common.copied'))
  } catch {
    toastErr(t('common.copy'))
  }
}

async function exportKey(l: License) {
  try {
    const res = await fetch(`/api/v1/admin/licenses/${l.license_id}/export`, {
      headers: { Authorization: `Bearer ${localStorage.getItem('dml_token') || ''}` },
    })
    if (!res.ok) throw new Error((await res.text()) || '导出失败')
    const text = await res.text()
    const blob = new Blob([text], { type: 'text/plain' })
    const a = document.createElement('a')
    a.href = URL.createObjectURL(blob)
    a.download = `${l.license_id}.lic`
    a.click()
    URL.revokeObjectURL(a.href)
    toastOk(t('detail.exportSuccess'))
  } catch (e: any) {
    toastErr(e?.message || t('detail.exportFailed'))
  }
}

// ---------- 吊销 ----------
const revokeTarget = ref<License | null>(null)
const revokeReason = ref('Refund')
const revokeBusy = ref(false)

function openRevoke(l: License) {
  revokeTarget.value = l
  revokeReason.value = 'Refund'
}

async function confirmRevoke() {
  if (!revokeTarget.value) return
  revokeBusy.value = true
  try {
    await api.post(`/api/v1/admin/licenses/${revokeTarget.value.license_id}/revoke`, {
      reason: revokeReason.value,
    })
    toastOk(t('revoke.successToast'))
    revokeTarget.value = null
    load(page.value, status.value)
  } catch (e: any) {
    toastErr(e?.message || t('revoke.failed'))
  } finally {
    revokeBusy.value = false
  }
}

const { t } = useI18n()

onMounted(() => load(1, ''))
</script>
