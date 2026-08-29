<template>
  <div v-if="license" class="max-w-4xl">
    <router-link class="text-muted text-[13px] hover:text-[var(--fg)]" to="/licenses">← 返回列表</router-link>

    <div class="flex items-center justify-between mt-3 mb-5">
      <div class="flex items-center gap-3">
        <h2 class="text-lg font-semibold mono">{{ license.license_id }}</h2>
        <StatusBadge :status="license.status" />
      </div>
      <div class="flex gap-2">
        <button v-if="license.status === 'active'" class="btn btn-ghost btn-sm" @click="openExtend">延期</button>
        <button class="btn btn-ghost btn-sm" @click="exportKey">导出 Key</button>
        <button v-if="license.status === 'active'" class="btn btn-danger btn-sm" @click="revoke">吊销</button>
      </div>
    </div>

    <div class="grid grid-cols-2 lg:grid-cols-4 gap-4 mb-5">
      <div class="card p-4">
        <div class="text-muted text-[12px]">客户</div>
        <div class="font-medium mt-1">{{ license.customer || '-' }}</div>
      </div>
      <div class="card p-4">
        <div class="text-muted text-[12px]">套餐</div>
        <div class="font-medium mt-1">{{ license.plan }}</div>
      </div>
      <div class="card p-4">
        <div class="text-muted text-[12px]">签发时间</div>
        <div class="font-medium mt-1">{{ fmtDateTime(license.issued_at) }}</div>
      </div>
      <div class="card p-4">
        <div class="text-muted text-[12px]">过期时间</div>
        <div class="font-medium mt-1" :class="{ 'text-danger': isExpired }">{{ fmtDateTime(license.expires_at) }}</div>
      </div>
      <div class="card p-4">
        <div class="text-muted text-[12px]">密钥标识</div>
        <div class="mono mt-1">{{ license.key_id }}</div>
      </div>
      <div class="card p-4">
        <div class="text-muted text-[12px]">最大设备数</div>
        <div class="font-medium mt-1">{{ license.max_devices }}</div>
      </div>
      <div class="card p-4">
        <div class="text-muted text-[12px]">功能</div>
        <div class="mt-1.5 flex flex-wrap gap-1.5">
          <span v-for="f in license.features" :key="f" class="badge badge-muted">{{ FEATURE_LABELS[f] || f }}</span>
        </div>
      </div>
      <div class="card p-4">
        <div class="text-muted text-[12px]">吊销信息</div>
        <div class="mt-1 text-[12px]" :class="license.status === 'revoked' ? 'text-danger' : 'text-muted'">
          {{ license.revoked_reason || (license.status === 'revoked' ? '已吊销' : '-') }}
        </div>
      </div>
    </div>

    <!-- 修订历史 -->
    <div class="card p-5">
      <h3 class="font-semibold text-[14px] mb-3">修订历史({{ revisions.length }})</h3>
      <table class="table">
        <thead>
          <tr>
            <th class="w-16">#</th>
            <th>原因</th>
            <th>操作人</th>
            <th>时间</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="r in revisions" :key="r.id">
            <td class="mono text-muted">R{{ r.revision }}</td>
            <td>{{ r.reason || 'issue' }}</td>
            <td>{{ r.created_by }}</td>
            <td class="text-muted">{{ fmtDateTimeStr(r.created_at) }}</td>
          </tr>
        </tbody>
      </table>
    </div>

    <!-- 延期弹窗 -->
    <div v-if="showExtend" class="modal-mask" @click.self="showExtend = false">
      <div class="card p-6 w-full max-w-sm">
        <h3 class="font-semibold mb-4">延期 License</h3>
        <div class="text-[13px] text-muted mb-3">
          当前过期:{{ fmtDate(license.expires_at) }}
        </div>
        <input v-model.number="extendDays" type="number" min="1" class="input mb-2" placeholder="延期天数(如 365)" />
        <input v-model="extendReason" class="input mb-4" placeholder="原因(如 renewal)" />
        <div class="flex justify-end gap-2">
          <button class="btn btn-ghost" @click="showExtend = false">取消</button>
          <button class="btn btn-primary" :disabled="busy" @click="extend">确认延期</button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { api, FEATURE_LABELS, type License, type LicenseRevision } from '../api'
import StatusBadge from '../components/StatusBadge.vue'

const route = useRoute()
const license = ref<License | null>(null)
const revisions = ref<LicenseRevision[]>([])
const showExtend = ref(false)
const extendDays = ref(365)
const extendReason = ref('renewal')
const busy = ref(false)

const isExpired = computed(() => !!license.value && license.value.expires_at * 1000 < Date.now())

function fmtDate(ts: number) {
  return new Date(ts * 1000).toLocaleDateString('zh-CN')
}
function fmtDateTime(ts: number) {
  return new Date(ts * 1000).toLocaleString('zh-CN')
}
function fmtDateTimeStr(s: string) {
  return new Date(s).toLocaleString('zh-CN')
}

async function load() {
  const id = route.params.id as string
  const res = await api.get<{ license: License }>(`/api/v1/admin/licenses/${id}`)
  license.value = res.license
  const revs = await api.get<{ items: LicenseRevision[] }>(`/api/v1/admin/licenses/${id}/revisions`)
  revisions.value = revs.items
}

function openExtend() {
  showExtend.value = true
}

async function extend() {
  busy.value = true
  try {
    await api.post(`/api/v1/admin/licenses/${license.value!.license_id}/extend`, {
      days: extendDays.value,
      reason: extendReason.value,
    })
    showExtend.value = false
    load()
  } catch (e: any) {
    alert(e?.message || '延期失败')
  } finally {
    busy.value = false
  }
}

async function revoke() {
  const reason = window.prompt('吊销原因:', 'Refund')
  if (reason === null) return
  try {
    await api.post(`/api/v1/admin/licenses/${license.value!.license_id}/revoke`, { reason })
    load()
  } catch (e: any) {
    alert(e?.message || '吊销失败')
  }
}

async function exportKey() {
  const res = await fetch(`/api/v1/admin/licenses/${license.value!.license_id}/export`, {
    headers: { Authorization: `Bearer ${localStorage.getItem('dml_token') || ''}` },
  })
  const text = await res.text()
  const blob = new Blob([text], { type: 'text/plain' })
  const a = document.createElement('a')
  a.href = URL.createObjectURL(blob)
  a.download = `${license.value!.license_id}.lic`
  a.click()
  URL.revokeObjectURL(a.href)
}

onMounted(load)
</script>
