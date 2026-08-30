<template>
  <div class="space-y-4 fade-up">
    <PageHeader :title="$t('customers.title')" :description="$t('customers.description')">
      <template #actions>
        <Button variant="primary" @click="openCreate">
          <Plus class="size-4" /> {{ $t('customers.new') }}
        </Button>
      </template>
    </PageHeader>

    <Card class="overflow-hidden">
      <div v-if="loading" class="p-4 space-y-2">
        <Skeleton v-for="i in 6" :key="i" class="h-10 w-full" />
      </div>
      <div v-else-if="error" class="p-12 text-center">
        <p class="text-danger text-[13.5px] mb-3">{{ $t('error.loadFailedWith', { what: $t('customers.title'), msg: error }) }}</p>
        <Button variant="ghost" size="sm" @click="load(1)">
          <RefreshCw class="size-3.5" /> {{ $t('common.retry') }}
        </Button>
      </div>
      <template v-else>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{{ $t('customers.id') }}</TableHead>
              <TableHead>{{ $t('customers.name') }}</TableHead>
              <TableHead>{{ $t('customers.email') }}</TableHead>
              <TableHead>{{ $t('customers.status') }}</TableHead>
              <TableHead>{{ $t('customers.createdAt') }}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            <TableRow v-for="c in items" :key="c.id">
              <TableCell class="font-mono text-[12px] text-brand">{{ c.customer_id }}</TableCell>
              <TableCell class="font-medium text-text">{{ c.name }}</TableCell>
              <TableCell class="text-muted">{{ c.email || '-' }}</TableCell>
              <TableCell><StatusBadge :status="c.status" /></TableCell>
              <TableCell class="text-muted text-[12px]">{{ fmtDateTimeStr(c.created_at) }}</TableCell>
            </TableRow>
            <TableRow v-if="!items.length">
              <TableCell colspan="5">
                <EmptyState
                  :icon="Users"
                  :title="$t('customers.empty')"
                  :description="$t('customers.emptyDesc')"
                  :action-label="$t('customers.new')"
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

    <!-- 新建客户 Dialog -->
    <Dialog :open="showCreate" @update:open="(v) => !v && (showCreate = false)">
      <DialogContent class="sm:max-w-md">
        <DialogHeader>
          <DialogTitle class="flex items-center gap-2">
            <UserPlus class="size-4 text-brand" /> {{ $t('customers.new') }}
          </DialogTitle>
        </DialogHeader>
        <form class="px-5 py-4 space-y-4" @submit.prevent="create">
          <div>
            <Label>{{ $t('customers.nameLabel') }} <span class="text-danger">*</span></Label>
            <Input v-model="form.name" :placeholder="$t('customers.name')" required />
          </div>
          <div>
            <Label>{{ $t('customers.emailLabel') }} <span class="text-muted font-normal">{{ $t('customers.emailOptional') }}</span></Label>
            <Input v-model="form.email" type="email" placeholder="user@example.com" />
          </div>
          <p v-if="formErr" class="text-danger text-[12.5px]">{{ formErr }}</p>
          <DialogFooter>
            <Button type="button" variant="ghost" @click="showCreate = false">{{ $t('common.cancel') }}</Button>
            <Button type="submit" variant="primary" :disabled="busy || !form.name.trim()">
              <Loader2 v-if="busy" class="size-3.5 animate-spin" />
              {{ $t('customers.create') }}
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
import { Loader2, Plus, RefreshCw, UserPlus, Users } from '@lucide/vue'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Skeleton } from '@/components/ui/skeleton'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import EmptyState from '../components/EmptyState.vue'
import PageHeader from '../components/PageHeader.vue'
import PaginationBar from '../components/PaginationBar.vue'
import StatusBadge from '../components/StatusBadge.vue'
import { api, type Customer } from '../api'
import { fmtDateTimeStr } from '../lib/format'
import { toastErr, toastOk } from '../lib/toast'

const { t } = useI18n()
const items = ref<Customer[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = 20
const loading = ref(true)
const error = ref('')

const showCreate = ref(false)
const busy = ref(false)
const formErr = ref('')
const form = reactive({ name: '', email: '' })

async function load(p: number) {
  page.value = p
  loading.value = true
  error.value = ''
  try {
    const res = await api.get<{ items: Customer[]; total: number }>(
      `/api/v1/admin/customers?page=${p}&page_size=${pageSize}`,
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
  form.name = ''
  form.email = ''
  formErr.value = ''
  showCreate.value = true
}

async function create() {
  busy.value = true
  formErr.value = ''
  try {
    await api.post('/api/v1/admin/customers', {
      name: form.name.trim(),
      email: form.email.trim() || undefined,
    })
    toastOk(t('customers.successToast'))
    showCreate.value = false
    load(1)
  } catch (e: any) {
    formErr.value = e?.message || t('customers.failed')
  } finally {
    busy.value = false
  }
}

onMounted(() => load(1))
</script>
