<template>
  <div class="flex h-full">
    <!-- 侧边栏 -->
    <aside class="w-52 shrink-0 border-r border-[var(--line)] bg-[var(--surface)] flex flex-col">
      <div class="px-5 py-5">
        <div class="flex items-center gap-2.5">
          <div
            class="w-8 h-8 rounded-lg flex items-center justify-center font-bold text-white"
            style="background: linear-gradient(135deg, #ec4899, #8b5cf6)"
          >
            D
          </div>
          <div>
            <div class="font-semibold text-[14px] leading-tight">License</div>
            <div class="text-[11px] text-muted">Docker Manager</div>
          </div>
        </div>
      </div>
      <nav class="flex-1 px-3 space-y-1">
        <router-link
          v-for="item in navs"
          :key="item.to"
          :to="item.to"
          class="flex items-center gap-2.5 px-3 py-2 rounded-lg text-[13px] text-muted hover:bg-[var(--surface2)] hover:text-[var(--fg)] transition-colors"
          active-class="!text-[var(--fg)] !bg-[var(--surface2)] font-medium"
        >
          <span class="text-[15px] w-5 text-center">{{ item.icon }}</span>
          {{ item.label }}
        </router-link>
      </nav>
      <div class="p-4 border-t border-[var(--line)]">
        <div class="flex items-center gap-2 text-[12px] text-muted mb-3">
          <span class="w-2 h-2 rounded-full bg-emerald-400 inline-block" />
          {{ username }}
        </div>
        <button class="btn btn-ghost btn-sm w-full" @click="logout">退出登录</button>
      </div>
    </aside>

    <!-- 内容 -->
    <main class="flex-1 overflow-y-auto p-6">
      <router-view />
    </main>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { clearToken } from '../api'

const router = useRouter()
const username = ref(localStorage.getItem('dml_username') || 'admin')

const navs = [
  { to: '/dashboard', icon: '📊', label: '概览' },
  { to: '/licenses', icon: '🔑', label: '许可证' },
  { to: '/audit', icon: '📜', label: '审计日志' },
  { to: '/settings', icon: '⚙️', label: '设置' },
]

function logout() {
  clearToken()
  localStorage.removeItem('dml_username')
  router.push('/login')
}
</script>
