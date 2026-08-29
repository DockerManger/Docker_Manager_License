<template>
  <div class="h-full flex items-center justify-center">
    <div class="card p-8 w-full max-w-sm">
      <div class="flex flex-col items-center mb-6">
        <div
          class="w-12 h-12 rounded-xl flex items-center justify-center font-bold text-white text-xl mb-3"
          style="background: linear-gradient(135deg, #ec4899, #8b5cf6)"
        >
          D
        </div>
        <h1 class="font-semibold text-lg">Docker Manager License</h1>
        <p class="text-muted text-[12px] mt-1">许可证签发与管理</p>
      </div>

      <form class="space-y-3" @submit.prevent="submit">
        <input v-model="username" class="input" placeholder="用户名" autocomplete="username" required />
        <input
          v-model="password"
          type="password"
          class="input"
          placeholder="密码"
          autocomplete="current-password"
          required
        />
        <input v-model="totp" class="input" placeholder="TOTP 动态码(未启用 2FA 可留空)" inputmode="numeric" />
        <button class="btn btn-primary w-full justify-center" :disabled="busy">
          {{ busy ? '登录中…' : '登 录' }}
        </button>
        <p v-if="err" class="text-danger text-[12px] text-center">{{ err }}</p>
      </form>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { api, setToken } from '../api'

const router = useRouter()
const username = ref('')
const password = ref('')
const totp = ref('')
const err = ref('')
const busy = ref(false)

async function submit() {
  err.value = ''
  busy.value = true
  try {
    const res = await api.post<{ token: string; username: string }>('/api/v1/admin/login', {
      username: username.value,
      password: password.value,
      totp_code: totp.value || undefined,
    })
    setToken(res.token)
    localStorage.setItem('dml_username', res.username)
    router.push('/dashboard')
  } catch (e: any) {
    err.value = e?.message || '登录失败'
  } finally {
    busy.value = false
  }
}
</script>
