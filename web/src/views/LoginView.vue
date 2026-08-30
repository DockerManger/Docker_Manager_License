<template>
  <div class="login-app relative min-h-screen overflow-hidden bg-bg">
    <!-- 背景图 + 遮罩(1Panel 风格;内置 bg.jpg,与 Docker_Manager_Go 同一张) -->
    <img src="/bg.jpg" alt="" class="login-bg" @error="bgFailed = true" />
    <div v-if="!bgFailed" class="login-bg-overlay" />

    <!-- 右上角工具栏:语言切换 + 主题切换(shadcn-vue 组件,与管理页一致) -->
    <div class="login-toolbar">
      <ToggleLocale />
      <ThemeToggle />
    </div>

    <div class="login-wrapper">
      <Card class="login-card p-8 fade-up">
        <div class="flex flex-col items-center mb-7">
          <AppLogo :size="56" />
          <h1 class="font-semibold text-[17px] text-text mt-4">{{ $t('login.title') }}</h1>
          <p class="text-muted text-[12.5px] mt-1">{{ $t('login.subtitle') }}</p>
        </div>

        <form class="space-y-3.5" @submit.prevent="submit">
          <div>
            <Label for="login-username">{{ $t('login.username') }}</Label>
            <Input
              id="login-username"
              v-model="username"
              class="h-10"
              :placeholder="$t('login.username')"
              autocomplete="username"
              required
            />
          </div>
          <div>
            <Label for="login-password">{{ $t('login.password') }}</Label>
            <Input
              id="login-password"
              v-model="password"
              type="password"
              class="h-10"
              :placeholder="$t('login.password')"
              autocomplete="current-password"
              required
            />
          </div>
          <div>
            <Label for="login-totp">
              {{ $t('login.totp') }}
              <span class="text-muted font-normal">({{ $t('login.totpHint') }})</span>
            </Label>
            <Input
              id="login-totp"
              v-model="totp"
              class="h-10"
              :placeholder="$t('login.totpPlaceholder')"
              inputmode="numeric"
              autocomplete="one-time-code"
            />
          </div>

          <p v-if="err" class="text-danger text-[12.5px] flex items-center gap-1.5">
            <AlertCircle class="size-3.5 shrink-0" /> {{ err }}
          </p>

          <Button type="submit" variant="brand" size="lg" class="w-full h-10" :disabled="busy">
            <Loader2 v-if="busy" class="size-4 animate-spin" />
            {{ busy ? $t('login.loggingIn') : $t('login.loginBtn') }}
          </Button>
        </form>

        <p class="text-center text-[11.5px] text-muted mt-6 leading-relaxed">
          {{ $t('login.secureNote') }}
        </p>
      </Card>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { AlertCircle, Loader2 } from '@lucide/vue'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import AppLogo from '../components/AppLogo.vue'
import ThemeToggle from '../components/ThemeToggle.vue'
import ToggleLocale from '../components/ToggleLocale.vue'
import { api, setToken } from '../api'

const router = useRouter()
const username = ref('')
const password = ref('')
const totp = ref('')
const err = ref('')
const busy = ref(false)
const bgFailed = ref(false)

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

<style scoped>
/* ---------- 登录页:背景图 + 遮罩(1Panel 风格,同 Docker_Manager_Go) ---------- */
.login-bg {
  position: absolute;
  inset: 0;
  width: 100%;
  height: 100%;
  object-fit: cover;
  z-index: 0;
}
.login-bg-overlay {
  position: absolute;
  inset: 0;
  background:
    linear-gradient(180deg, rgba(5, 8, 14, 0.55) 0%, rgba(5, 8, 14, 0.75) 100%),
    radial-gradient(ellipse at center, transparent 0%, rgba(5, 8, 14, 0.35) 100%);
  z-index: 1;
}

/* ---------- 右上角工具栏(shadcn 组件:语言 DropdownMenu + 主题按钮) ---------- */
.login-toolbar {
  position: fixed;
  top: 16px;
  right: 16px;
  z-index: 10;
  display: inline-flex;
  align-items: center;
  gap: 10px;
}

/* 登录页工具栏按钮:放大(44px 圆 + 20px 图标)+ 背景醒目(背景图上清晰可见) */
.login-toolbar :deep(button) {
  width: 44px;
  height: 44px;
  min-width: 44px;
  padding: 0;
  border-radius: 50%;
  border: 1px solid rgba(255, 255, 255, 0.4);
  background: rgba(255, 255, 255, 0.18);
  color: #ffffff;
  box-shadow: 0 2px 10px rgba(0, 0, 0, 0.3);
  transition: all 0.2s;
}
.login-toolbar :deep(button:hover) {
  background: rgba(255, 255, 255, 0.3);
  border-color: var(--color-brand);
  color: var(--color-brand);
  transform: translateY(-1px);
}
.login-toolbar :deep(svg) {
  width: 20px;
  height: 20px;
}

/* ---------- 居中卡片(毛玻璃) ---------- */
.login-wrapper {
  position: relative;
  min-height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 24px 16px;
  z-index: 2;
}
.login-card {
  width: 100%;
  max-width: 400px;
  background: rgba(255, 255, 255, 0.72);
  border-color: rgba(255, 255, 255, 0.6);
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.04), 0 18px 50px rgba(0, 0, 0, 0.35);
  -webkit-backdrop-filter: blur(24px) saturate(180%);
  backdrop-filter: blur(24px) saturate(180%);
}
html[data-theme='dark'] .login-card {
  background: rgba(28, 26, 34, 0.55);
  border-color: rgba(255, 255, 255, 0.1);
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.4), 0 20px 60px rgba(0, 0, 0, 0.5);
}
</style>
