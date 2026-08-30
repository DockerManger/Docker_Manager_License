<template>
  <div class="h-full flex items-center justify-center relative overflow-hidden bg-bg">
    <!-- 背景氛围(克制) -->
    <div class="absolute inset-0 pointer-events-none">
      <div
        class="absolute -top-32 -right-32 w-96 h-96 rounded-full opacity-[0.07] dark:opacity-[0.12]"
        style="background: radial-gradient(circle, #ec4899 0%, transparent 70%)"
      />
      <div
        class="absolute -bottom-32 -left-32 w-96 h-96 rounded-full opacity-[0.05] dark:opacity-[0.1]"
        style="background: radial-gradient(circle, #8b5cf6 0%, transparent 70%)"
      />
    </div>

    <div class="relative w-full max-w-sm px-4">
      <Card class="p-8 shadow-2xl shadow-black/20 fade-up">
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
