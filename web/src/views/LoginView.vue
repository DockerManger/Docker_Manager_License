<template>
  <div class="login-app relative min-h-screen overflow-hidden bg-bg">
    <!-- 背景图 + 遮罩(1Panel 风格;内置 bg.jpg,与 Docker_Manager_Go 同一张) -->
    <img src="/bg.jpg" alt="" class="login-bg" @error="bgFailed = true" />
    <div v-if="!bgFailed" class="login-bg-overlay" />

    <!-- 右上角工具栏:主题切换 + 语言切换(仿 3x-ui / Docker_Manager_Go 登录页) -->
    <div class="login-toolbar">
      <button
        type="button"
        class="toolbar-btn"
        :title="theme === 'dark' ? $t('theme.switchToLight') : $t('theme.switchToDark')"
        :aria-label="theme === 'dark' ? $t('theme.switchToLight') : $t('theme.switchToDark')"
        @click="toggleThemeWithTransition($event)"
      >
        <Sun v-if="theme === 'dark'" class="size-[18px]" />
        <Moon v-else class="size-[18px]" />
      </button>
      <div class="lang-pop-wrap" ref="langWrapRef">
        <button
          type="button"
          class="toolbar-btn"
          :title="$t('lang.switch')"
          :aria-label="$t('lang.switch')"
          @click.stop="langOpen = !langOpen"
        >
          <Languages class="size-[18px]" />
        </button>
        <Transition name="dm-drop">
          <div v-if="langOpen" class="lang-pop" @click.stop>
            <button
              v-for="l in LANGS"
              :key="l.code"
              type="button"
              class="lang-item"
              :class="{ active: locale === l.code }"
              @click="onLang(l.code)"
            >
              <span aria-hidden="true">{{ l.flag }}</span>
              <span>{{ l.label }}</span>
              <Check v-if="locale === l.code" class="lang-check" />
            </button>
          </div>
        </Transition>
      </div>
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
import { onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { AlertCircle, Check, Languages, Loader2, Moon, Sun } from '@lucide/vue'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import AppLogo from '../components/AppLogo.vue'
import { api, setToken } from '../api'
import { LANGS, setLang } from '../i18n'
import { theme, toggleThemeWithTransition } from '../lib/theme'

const router = useRouter()
const { locale } = useI18n()
const username = ref('')
const password = ref('')
const totp = ref('')
const err = ref('')
const busy = ref(false)
const langOpen = ref(false)
const langWrapRef = ref<HTMLElement | null>(null)
const bgFailed = ref(false)

function onLang(code: string) {
  setLang(code)
  langOpen.value = false
}

function onDocClick(e: MouseEvent) {
  if (langOpen.value && langWrapRef.value && !langWrapRef.value.contains(e.target as Node)) {
    langOpen.value = false
  }
}
onMounted(() => {
  document.addEventListener('click', onDocClick)
})

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

/* ---------- 右上角工具栏 ---------- */
.login-toolbar {
  position: fixed;
  top: 16px;
  right: 16px;
  z-index: 10;
  display: inline-flex;
  align-items: center;
  gap: 8px;
}
.toolbar-btn {
  width: 40px;
  height: 40px;
  min-width: 40px;
  border-radius: 50%;
  padding: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  border: 1px solid rgba(255, 255, 255, 0.35);
  background: rgba(255, 255, 255, 0.55);
  color: rgba(0, 0, 0, 0.75);
  cursor: pointer;
  transition: all 0.2s;
}
html[data-theme='dark'] .toolbar-btn {
  background: rgba(255, 255, 255, 0.08);
  border-color: rgba(255, 255, 255, 0.14);
  color: rgba(255, 255, 255, 0.87);
}
.toolbar-btn:hover {
  transform: translateY(-1px);
  border-color: #ec4899;
  color: #ec4899;
}

/* 语言下拉 */
.lang-pop-wrap {
  position: relative;
}
.lang-pop {
  position: absolute;
  top: 48px;
  right: 0;
  z-index: 30;
  min-width: 180px;
  max-height: 60vh;
  overflow-y: auto;
  padding: 4px;
  border-radius: 12px;
  border: 1px solid rgba(0, 0, 0, 0.08);
  background: #ffffff;
  box-shadow: 0 8px 28px rgba(0, 0, 0, 0.16);
}
html[data-theme='dark'] .lang-pop {
  background: #1c1a22;
  border-color: rgba(255, 255, 255, 0.12);
}
.lang-item {
  display: flex;
  align-items: center;
  gap: 8px;
  width: 100%;
  padding: 8px 12px;
  border: none;
  border-radius: 8px;
  background: transparent;
  color: rgba(0, 0, 0, 0.88);
  font-size: 13px;
  cursor: pointer;
  text-align: left;
  transition: background 0.15s;
}
html[data-theme='dark'] .lang-item {
  color: rgba(255, 255, 255, 0.87);
}
.lang-item:hover {
  background: rgba(236, 72, 153, 0.08);
}
.lang-item.active {
  background: rgba(236, 72, 153, 0.12);
  color: #ec4899;
  font-weight: 600;
}
.lang-check {
  margin-left: auto;
  font-size: 12px;
}

/* 语言菜单动画 */
.dm-drop-enter-active,
.dm-drop-leave-active {
  transition: opacity 0.15s ease, transform 0.15s ease;
}
.dm-drop-enter-from,
.dm-drop-leave-to {
  opacity: 0;
  transform: translateY(-6px) scale(0.98);
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
