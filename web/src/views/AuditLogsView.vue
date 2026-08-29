<template>
  <div>
    <h2 class="text-lg font-semibold mb-4">审计日志</h2>
    <div class="card overflow-hidden">
      <table class="table">
        <thead>
          <tr>
            <th>时间</th>
            <th>操作人</th>
            <th>动作</th>
            <th>资源</th>
            <th>IP</th>
            <th>详情</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="e in items" :key="e.id">
            <td class="text-muted whitespace-nowrap">{{ fmt(e.created_at) }}</td>
            <td>{{ e.admin || '-' }}</td>
            <td><span class="badge badge-muted mono">{{ e.action }}</span></td>
            <td class="mono">{{ e.resource_id || '-' }}</td>
            <td class="mono text-muted">{{ e.ip }}</td>
            <td class="text-muted text-[12px]">{{ prettyMeta(e.metadata) }}</td>
          </tr>
          <tr v-if="!items.length">
            <td colspan="6" class="text-center text-muted py-10">暂无审计日志</td>
          </tr>
        </tbody>
      </table>
    </div>
    <div class="flex items-center justify-between mt-4 text-[13px] text-muted">
      <span>共 {{ total }} 条</span>
      <div class="flex gap-2 items-center">
        <button class="btn btn-ghost btn-sm" :disabled="page <= 1" @click="load(page - 1)">上一页</button>
        <span>第 {{ page }} 页</span>
        <button class="btn btn-ghost btn-sm" :disabled="page * pageSize >= total" @click="load(page + 1)">下一页</button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { api, type AuditLog } from '../api'

const items = ref<AuditLog[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = 30

function fmt(s: string) {
  return new Date(s).toLocaleString('zh-CN')
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
  const res = await api.get<{ items: AuditLog[]; total: number }>(
    `/api/v1/admin/audit-logs?page=${p}&page_size=${pageSize}`,
  )
  items.value = res.items || []
  total.value = res.total
}

onMounted(() => load(1))
</script>
