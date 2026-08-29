<template>
  <div class="max-w-xl">
    <h2 class="text-lg font-semibold mb-5">设置</h2>

    <!-- 修改密码 -->
    <div class="card p-5 mb-5">
      <h3 class="font-semibold text-[14px] mb-3">修改密码</h3>
      <p class="text-[12px] text-muted mb-3">修改成功后所有会话将失效,需重新登录。</p>
      <form class="space-y-3" @submit.prevent="changePassword">
        <input v-model="pw.old" type="password" class="input" placeholder="当前密码" required />
        <input v-model="pw.new1" type="password" class="input" placeholder="新密码(至少 8 位)" required />
        <input v-model="pw.new2" type="password" class="input" placeholder="确认新密码" required />
        <p v-if="pwErr" class="text-danger text-[12px]">{{ pwErr }}</p>
        <div class="flex justify-end">
          <button class="btn btn-primary" :disabled="pwBusy">修改密码</button>
        </div>
      </form>
    </div>

    <!-- 两步验证 -->
    <div class="card p-5">
      <h3 class="font-semibold text-[14px] mb-3">两步验证(2FA)</h3>
      <p class="text-[12px] text-muted mb-3">启用后登录需输入 TOTP 动态码。</p>

      <template v-if="!totpSecret">
        <div class="flex justify-end">
          <button class="btn btn-ghost" @click="setupTOTP">生成二维码密钥</button>
        </div>
      </template>

      <template v-if="totpSecret">
        <div class="rounded-lg border border-[var(--line)] p-4 bg-[var(--surface2)] space-y-3">
          <div class="text-[12px] text-muted">
            1. 用 Authenticator 应用扫描或手动输入密钥
            <div class="mono text-[var(--fg)] mt-1 select-all">{{ totpSecret }}</div>
          </div>
          <div class="text-[12px] text-muted">2. 输入当前动态码确认启用</div>
          <div class="flex gap-2">
            <input v-model="totpCode" class="input" placeholder="6 位动态码" inputmode="numeric" />
            <button class="btn btn-primary" @click="confirmTOTP">确认启用</button>
          </div>
        </div>
      </template>
    </div>
  </div>
</template>

<script setup lang="ts">
import { reactive, ref } from 'vue'
import { api } from '../api'

const pw = reactive({ old: '', new1: '', new2: '' })
const pwErr = ref('')
const pwBusy = ref(false)

const totpSecret = ref('')
const totpCode = ref('')

async function changePassword() {
  pwErr.value = ''
  if (pw.new1.length < 8) {
    pwErr.value = '新密码至少 8 位'
    return
  }
  if (pw.new1 !== pw.new2) {
    pwErr.value = '两次输入的新密码不一致'
    return
  }
  pwBusy.value = true
  try {
    await api.post('/api/v1/admin/change-password', {
      old_password: pw.old,
      new_password: pw.new1,
    })
    pwErr.value = '密码已修改,请重新登录'
    setTimeout(() => {
      localStorage.clear()
      location.href = '/login'
    }, 1200)
  } catch (e: any) {
    pwErr.value = e?.message || '修改失败'
  } finally {
    pwBusy.value = false
  }
}

async function setupTOTP() {
  try {
    const res = await api.post<{ secret: string }>('/api/v1/admin/setup-totp')
    totpSecret.value = res.secret
  } catch (e: any) {
    alert(e?.message || '生成失败')
  }
}

async function confirmTOTP() {
  try {
    await api.post('/api/v1/admin/confirm-totp', {
      secret: totpSecret.value,
      code: totpCode.value,
    })
    totpSecret.value = ''
    totpCode.value = ''
    alert('2FA 已启用')
  } catch (e: any) {
    alert(e?.message || '动态码无效')
  }
}
</script>
