<template>
  <div>
    <h2 class="text-lg font-semibold mb-4">概览</h2>
    <div v-if="stats" class="grid grid-cols-2 lg:grid-cols-4 gap-4">
      <div class="card p-5">
        <div class="text-muted text-[12px]">全部许可证</div>
        <div class="text-3xl font-bold mt-1">{{ stats.total }}</div>
      </div>
      <div class="card p-5">
        <div class="text-muted text-[12px]">有效</div>
        <div class="text-3xl font-bold mt-1 text-emerald-400">{{ byStatus('active') }}</div>
      </div>
      <div class="card p-5">
        <div class="text-muted text-[12px]">已吊销</div>
        <div class="text-3xl font-bold mt-1 text-red-400">{{ byStatus('revoked') }}</div>
      </div>
      <div class="card p-5">
        <div class="text-muted text-[12px]">已过期</div>
        <div class="text-3xl font-bold mt-1 text-amber-400">{{ byStatus('expired') }}</div>
      </div>
    </div>

    <div class="card p-5 mt-6">
      <div class="text-muted text-[12px] mb-3">最近签发的许可证</div>
      <table class="table">
        <thead>
          <tr>
            <th>License ID</th>
            <th>客户</th>
            <th>套餐</th>
            <th>状态</th>
            <th>过期时间</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="l in recent" :key="l.id">
            <td class="mono">
              <router-link class="link" :to="`/licenses/${l.license_id}`">{{ l.license_id }}</router-link>
            </td>
            <td>{{ l.customer }}</td>
            <td><span class="badge badge-muted">{{ l.plan }}</span></td>
            <td><StatusBadge :status="l.status" /></td>
            <td class="text-muted">{{ fmtDate(l.expires_at) }}</td>
          </tr>
          <tr v-if="!recent.length">
            <td colspan="5" class="text-center text-muted py-8">暂无许可证</td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { api, type License } from '../api'
import StatusBadge from '../components/StatusBadge.vue'

const stats = ref<any>(null)
const recent = ref<License[]>([])

function byStatus(s: string): number {
  return stats.value?.by_status?.[s] || 0
}

function fmtDate(ts: number): string {
  return new Date(ts * 1000).toLocaleDateString('zh-CN')
}

onMounted(async () => {
  try {
    stats.value = await api.get('/api/v1/admin/stats')
    const page = await api.get<{ items: License[] }>('/api/v1/admin/licenses?page=1&page_size=8')
    recent.value = page.items || []
  } catch (e: any) {
    console.error(e)
  }
})
</script>
