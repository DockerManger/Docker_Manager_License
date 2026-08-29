<template>
  <div>
    <div class="flex items-center justify-between mb-4">
      <h2 class="text-lg font-semibold">许可证</h2>
      <button class="btn btn-primary" @click="openCreate">
        <span class="text-[14px]">＋</span> 签发 License
      </button>
    </div>

    <!-- 状态筛选 -->
    <div class="flex gap-2 mb-4">
      <button
        v-for="s in statusFilter"
        :key="s.value"
        class="btn btn-sm"
        :class="s.value === status ? 'btn-primary' : 'btn-ghost'"
        @click="load(1, s.value)"
      >
        {{ s.label }}
      </button>
    </div>

    <div class="card overflow-hidden">
      <table class="table">
        <thead>
          <tr>
            <th>License ID</th>
            <th>客户</th>
            <th>套餐</th>
            <th>状态</th>
            <th>过期时间</th>
            <th>设备数</th>
            <th class="w-40">操作</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="l in items" :key="l.id">
            <td class="mono">
              <router-link class="link" :to="`/licenses/${l.license_id}`">{{ l.license_id }}</router-link>
            </td>
            <td>{{ l.customer }}</td>
            <td><span class="badge badge-muted">{{ l.plan }}</span></td>
            <td><StatusBadge :status="l.status" /></td>
            <td class="text-muted">{{ fmtDate(l.expires_at) }}</td>
            <td class="text-muted">{{ l.max_devices }}</td>
            <td>
              <div class="flex gap-1.5">
                <router-link class="btn btn-ghost btn-sm" :to="`/licenses/${l.license_id}`">详情</router-link>
                <button v-if="l.status === 'active'" class="btn btn-danger btn-sm" @click="revoke(l)">吊销</button>
              </div>
            </td>
          </tr>
          <tr v-if="!items.length">
            <td colspan="7" class="text-center text-muted py-10">暂无许可证</td>
          </tr>
        </tbody>
      </table>
    </div>

    <!-- 分页 -->
    <div class="flex items-center justify-between mt-4 text-[13px] text-muted">
      <span>共 {{ total }} 条</span>
      <div class="flex gap-2 items-center">
        <button class="btn btn-ghost btn-sm" :disabled="page <= 1" @click="load(page - 1, status)">上一页</button>
        <span>第 {{ page }} 页</span>
        <button class="btn btn-ghost btn-sm" :disabled="page * pageSize >= total" @click="load(page + 1, status)">
          下一页
        </button>
      </div>
    </div>

    <!-- 签发弹窗 -->
    <div v-if="showCreate" class="modal-mask" @click.self="showCreate = false">
      <div class="card p-6 w-full max-w-lg">
        <h3 class="font-semibold mb-4">签发 License</h3>
        <form class="space-y-3" @submit.prevent="create">
          <div>
            <label class="text-[12px] text-muted block mb-1">客户</label>
            <input v-model="form.customer" class="input" placeholder="如 Zhao" required />
          </div>
          <div>
            <label class="text-[12px] text-muted block mb-1">套餐</label>
            <select v-model="form.plan" class="input">
              <option v-for="p in PLANS" :key="p" :value="p">{{ p }}</option>
            </select>
          </div>
          <div>
            <label class="text-[12px] text-muted block mb-1.5">功能</label>
            <div class="flex flex-wrap gap-2">
              <label
                v-for="f in FEATURES"
                :key="f"
                class="flex items-center gap-1.5 px-3 py-1.5 rounded-lg border cursor-pointer text-[12px] select-none"
                :class="form.features.includes(f) ? 'border-[var(--brand)] text-[var(--brand)]' : 'border-[var(--line)] text-muted'"
              >
                <input v-model="form.features" type="checkbox" :value="f" class="hidden" />
                {{ FEATURE_LABELS[f] }}
              </label>
            </div>
          </div>
          <div class="grid grid-cols-2 gap-3">
            <div>
              <label class="text-[12px] text-muted block mb-1">有效期(天)</label>
              <input v-model.number="form.expireDays" type="number" min="1" class="input" placeholder="如 365" required />
            </div>
            <div>
              <label class="text-[12px] text-muted block mb-1">最大设备数</label>
              <input v-model.number="form.maxDevices" type="number" min="1" class="input" value="1" />
            </div>
          </div>
          <div>
            <label class="text-[12px] text-muted block mb-1">备注</label>
            <input v-model="form.notes" class="input" placeholder="可选" />
          </div>
          <p v-if="formErr" class="text-danger text-[12px]">{{ formErr }}</p>

          <!-- 签发结果 -->
          <div v-if="issued" class="rounded-lg border border-[var(--line)] p-3 bg-[var(--surface2)]">
            <div class="text-[12px] text-muted mb-1.5">License Key(发给用户):</div>
            <div class="mono text-[11px] break-all leading-relaxed max-h-28 overflow-y-auto">{{ issued.key }}</div>
            <div class="flex gap-2 mt-3">
              <button type="button" class="btn btn-primary btn-sm" @click="copyKey">复制 Key</button>
              <a
                :href="`data:text/plain;charset=utf-8,${encodeURIComponent(issued.key)}`"
                download="license.lic"
                class="btn btn-ghost btn-sm"
              >下载 .lic 文件</a>
            </div>
          </div>

          <div class="flex justify-end gap-2 pt-2">
            <button type="button" class="btn btn-ghost" @click="closeCreate">取消</button>
            <button v-if="!issued" type="submit" class="btn btn-primary" :disabled="busy">
              {{ busy ? '签发中…' : '签发' }}
            </button>
            <button v-else type="button" class="btn btn-primary" @click="closeCreate">完成</button>
          </div>
        </form>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { api, FEATURES, FEATURE_LABELS, PLANS, type License } from '../api'
import StatusBadge from '../components/StatusBadge.vue'

const items = ref<License[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = 20
const status = ref('')

const statusFilter = [
  { value: '', label: '全部' },
  { value: 'active', label: '有效' },
  { value: 'expired', label: '已过期' },
  { value: 'revoked', label: '已吊销' },
]

const showCreate = ref(false)
const busy = ref(false)
const formErr = ref('')
const issued = ref<{ key: string } | null>(null)
const form = reactive({
  customer: '',
  plan: 'pro',
  features: ['compose', 'container_create', 'appstore'] as string[],
  expireDays: 365,
  maxDevices: 1,
  notes: '',
})

function fmtDate(ts: number): string {
  return new Date(ts * 1000).toLocaleDateString('zh-CN')
}

async function load(p: number, s: string) {
  page.value = p
  status.value = s
  const res = await api.get<{ items: License[]; total: number }>(
    `/api/v1/admin/licenses?page=${p}&page_size=${pageSize}${s ? `&status=${s}` : ''}`,
  )
  items.value = res.items
  total.value = res.total
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
    load(1, status.value)
  } catch (e: any) {
    formErr.value = e?.message || '签发失败'
  } finally {
    busy.value = false
  }
}

async function copyKey() {
  if (!issued.value) return
  await navigator.clipboard.writeText(issued.value.key)
  formErr.value = '已复制到剪贴板'
}

async function revoke(l: License) {
  const reason = window.prompt(`吊销 ${l.license_id} 的原因(Refund/Fraud/Abuse 等):`, 'Refund')
  if (reason === null) return
  try {
    await api.post(`/api/v1/admin/licenses/${l.license_id}/revoke`, { reason })
    load(page.value, status.value)
  } catch (e: any) {
    alert(e?.message || '吊销失败')
  }
}

onMounted(() => load(1, ''))
</script>
