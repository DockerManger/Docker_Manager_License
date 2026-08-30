<template>
  <div class="space-y-4 fade-up">
    <PageHeader :title="$t('subscriptions.title')" :description="$t('subscriptions.description')">
      <template #actions>
        <Button variant="primary" @click="openCreate">
          <Plus class="size-4" /> {{ $t('subscriptions.new') }}
        </Button>
      </template>
    </PageHeader>

    <Card class="overflow-hidden">
      <div v-if="loading" class="p-4 space-y-2">
        <Skeleton v-for="i in 6" :key="i" class="h-10 w-full" />
      </div>
      <div v-else-if="error" class="p-12 text-center">
        <p class="text-danger text-[13.5px] mb-3">{{ $t('error.loadFailedWith', { what: $t('subscriptions.title'), msg: error }) }}</p>
        <Button variant="ghost" size="sm" @click="load(1)">
          <RefreshCw class="size-3.5" /> {{ $t('common.retry') }}
        </Button>
      </div>
      <template v-else>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{{ $t('subscriptions.id') }}</TableHead>
              <TableHead>{{ $t('subscriptions.customer') }}</TableHead>
              <TableHead>{{ $t('subscriptions.plan') }}</TableHead>
              <TableHead>{{ $t('subscriptions.status') }}</TableHead>
              <TableHead>{{ $t('subscriptions.starts') }}</TableHead>
              <TableHead>{{ $t('subscriptions.expires') }}</TableHead>
              <TableHead>{{ $t('subscriptions.autoRenew') }}</TableHead>
              <TableHead class="w-16 text-right">{{ $t('common.actions') }}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            <TableRow v-for="s in items" :key="s.id">
              <TableCell class="font-mono text-[12px] text-brand">{{ s.subscription_id }}</TableCell>
              <TableCell class="font-mono text-[12px] text-muted">{{ s.customer_id }}</TableCell>
              <TableCell><Badge variant="outline">{{ s.plan }}</Badge></TableCell>
              <TableCell><StatusBadge :status="s.status" /></TableCell>
              <TableCell class="text-muted text-[12px]">{{ fmtDate(s.starts_at) }}</TableCell>
              <TableCell class="text-muted text-[12px]" :class="{ '!text-danger font-medium': isExpiredSub(s) }">
                {{ fmtDate(s.expires_at) }}
              </TableCell>
              <TableCell>
                <span v-if="s.auto_renew" class="inline-flex items-center gap-1 text-[12px] text-ok">
                  <RefreshCw class="size-3" /> {{ $t('subscriptions.on') }}
                </span>
                <span v-else class="text-[12px] text-muted">{{ $t('subscriptions.off') }}</span>
              </TableCell>
              <TableCell class="text-right" @click.stop>
                <DropdownMenu>
                  <DropdownMenuTrigger as-child>
                    <Button variant="icon" size="sm">
                      <MoreHorizontal class="size-4" />
                    </Button>
                  </DropdownMenuTrigger>
                  <DropdownMenuContent align="end" class="w-44">
                    <DropdownMenuItem v-if="s.status === 'active'" @select="changeStatus(s, 'suspended')">
                      <PauseCircle class="size-3.5" /> {{ $t('subscriptions.suspend') }}
                    </DropdownMenuItem>
                    <DropdownMenuItem v-if="s.status === 'suspended'" @select="changeStatus(s, 'active')">
                      <PlayCircle class="size-3.5" /> {{ $t('subscriptions.resume') }}
                    </DropdownMenuItem>
                    <DropdownMenuItem v-if="s.status === 'active' || s.status === 'suspended'" variant="destructive" @select="changeStatus(s, 'cancelled')">
                      <Ban class="size-3.5" /> {{ $t('subscriptions.cancel') }}
                    </DropdownMenuItem>
                  </DropdownMenuContent>
                </DropdownMenu>
              </TableCell>
            </TableRow>
            <TableRow v-if="!items.length">
              <TableCell colspan="8">
                <EmptyState
                  :icon="CreditCard"
                  :title="$t('subscriptions.empty')"
                  :description="$t('subscriptions.emptyDesc')"
                  :action-label="$t('subscriptions.new')"
                  @action="openCreate"
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

    <!-- 新建订阅 Dialog -->
    <Dialog :open="showCreate" @update:open="(v) => !v && (showCreate = false)">
      <DialogContent class="sm:max-w-md">
        <DialogHeader>
          <DialogTitle class="flex items-center gap-2">
            <CreditCard class="size-4 text-brand" /> {{ $t('subscriptions.new') }}
          </DialogTitle>
        </DialogHeader>
        <form class="px-5 py-4 space-y-4" @submit.prevent="create">
          <div>
            <Label>{{ $t('subscriptions.customerLabel') }} <span class="text-danger">*</span></Label>
            <Select v-model="form.customerId">
              <SelectTrigger><SelectValue :placeholder="$t('subscriptions.selectCustomer')" /></SelectTrigger>
              <SelectContent>
                <SelectItem v-for="c in customers" :key="c.customer_id" :value="c.customer_id">
                  {{ c.name }} ({{ c.customer_id }})
                </SelectItem>
              </SelectContent>
            </Select>
          </div>
          <div class="grid grid-cols-2 gap-4">
            <div>
              <Label>{{ $t('subscriptions.planLabel') }}</Label>
              <Select v-model="form.plan">
                <SelectTrigger><SelectValue /></SelectTrigger>
                <SelectContent>
                  <SelectItem value="pro">pro</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div class="flex items-end pb-1">
              <label class="flex items-center gap-2 text-[12.5px] text-text cursor-pointer select-none">
                <Checkbox v-model="form.autoRenew" /> {{ $t('subscriptions.autoRenewLabel') }}
              </label>
            </div>
          </div>
          <div class="grid grid-cols-2 gap-4">
            <div>
              <Label>{{ $t('subscriptions.startsAt') }} <span class="text-muted font-normal">{{ $t('subscriptions.secondsHint') }}</span></Label>
              <Input v-model.number="form.startsAt" type="number" :placeholder="String(Math.floor(Date.now() / 1000))" />
            </div>
            <div>
              <Label>{{ $t('subscriptions.endsAt') }} <span class="text-danger">*</span></Label>
              <Input v-model.number="form.expiresAt" type="number" required :placeholder="String(Math.floor(Date.now() / 1000) + 31536000)" />
            </div>
          </div>
          <p class="text-[11.5px] text-muted leading-relaxed">
            {{ $t('subscriptions.tsHint') }}
          </p>
          <p v-if="formErr" class="text-danger text-[12.5px]">{{ formErr }}</p>
          <DialogFooter>
            <Button type="button" variant="ghost" @click="showCreate = false">{{ $t('common.cancel') }}</Button>
            <Button type="submit" variant="primary" :disabled="busy || !form.customerId || !form.expiresAt">
              <Loader2 v-if="busy" class="size-3.5 animate-spin" />
              {{ $t('subscriptions.create') }}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  </div>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  Ban, CreditCard, Loader2, MoreHorizontal, PauseCircle, PlayCircle, Plus, RefreshCw,
} from '@lucide/vue'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Checkbox } from '@/components/ui/checkbox'
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Skeleton } from '@/components/ui/skeleton'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import EmptyState from '../components/EmptyState.vue'
import PageHeader from '../components/PageHeader.vue'
import PaginationBar from '../components/PaginationBar.vue'
import StatusBadge from '../components/StatusBadge.vue'
import { api, type Customer, type Subscription } from '../api'
import { fmtDate } from '../lib/format'
import { toastErr, toastOk } from '../lib/toast'

const { t } = useI18n()
const items = ref<Subscription[]>([])
const customers = ref<Customer[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = 20
const loading = ref(true)
const error = ref('')

const showCreate = ref(false)
const busy = ref(false)
const formErr = ref('')
const form = reactive({
  customerId: '',
  plan: 'pro',
  autoRenew: false,
  startsAt: Math.floor(Date.now() / 1000),
  expiresAt: 0,
})

function isExpiredSub(s: Subscription) {
  return s.expires_at > 0 && s.expires_at * 1000 < Date.now() && s.status === 'active'
}

async function load(p: number) {
  page.value = p
  loading.value = true
  error.value = ''
  try {
    const res = await api.get<{ items: Subscription[]; total: number }>(
      `/api/v1/admin/subscriptions?page=${p}&page_size=${pageSize}`,
    )
    items.value = res.items || []
    total.value = res.total
  } catch (e: any) {
    error.value = e?.message || '加载失败'
  } finally {
    loading.value = false
  }
}

function openCreate() {
  formErr.value = ''
  form.customerId = ''
  form.plan = 'pro'
  form.autoRenew = false
  form.startsAt = Math.floor(Date.now() / 1000)
  form.expiresAt = 0
  showCreate.value = true
  // 加载客户下拉
  api.get<{ items: Customer[] }>('/api/v1/admin/customers?page=1&page_size=100').then((r) => {
    customers.value = r.items || []
  }).catch(() => {
    customers.value = []
  })
}

async function create() {
  busy.value = true
  formErr.value = ''
  try {
    await api.post('/api/v1/admin/subscriptions', {
      customer_id: form.customerId,
      plan: form.plan,
      starts_at: form.startsAt || Math.floor(Date.now() / 1000),
      expires_at: form.expiresAt,
      auto_renew: form.autoRenew,
    })
    toastOk(t('subscriptions.successToast'))
    showCreate.value = false
    load(1)
  } catch (e: any) {
    formErr.value = e?.message || t('subscriptions.failed')
  } finally {
    busy.value = false
  }
}

async function changeStatus(s: Subscription, status: string) {
  try {
    await api.post(`/api/v1/admin/subscriptions/${s.subscription_id}/status`, { status })
    toastOk(t('subscriptions.statusSuccess'))
    load(page.value)
  } catch (e: any) {
    toastErr(e?.message || t('subscriptions.failed'))
  }
}

onMounted(() => load(1))
</script>
