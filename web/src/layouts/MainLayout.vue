<template>
  <div class="flex h-full">
    <!-- 桌面侧边栏(hover 展开/收起:72px ↔ 220px) -->
    <aside
      class="hidden lg:flex flex-col border-r border-line bg-surface transition-[width] duration-200 ease-in-out shrink-0 overflow-hidden"
      :class="collapsed ? 'w-[72px]' : 'w-[220px]'"
      @mouseenter="collapsed = false"
      @mouseleave="collapsed = true"
    >
      <!-- Logo:只图标居中 -->
      <div class="h-16 shrink-0 flex items-center justify-center border-b border-line">
        <AppLogo :size="34" compact />
      </div>

      <nav class="flex-1 overflow-y-auto px-2.5 py-4 overflow-x-hidden">
        <template v-for="group in navGroups" :key="group.label">
          <div
            v-if="group.label"
            class="px-3 pt-4 pb-1.5 text-[10.5px] font-semibold uppercase tracking-wider text-muted/70 whitespace-nowrap transition-opacity"
            :class="collapsed ? 'opacity-0' : 'opacity-100'"
          >
            {{ group.label }}
          </div>
          <router-link
            v-for="item in group.items"
            :key="item.to"
            :to="item.to"
            class="flex items-center gap-2.5 px-3 py-2 rounded-[0.5rem] text-[13px] text-muted hover:bg-surface2 hover:text-text transition-colors mb-0.5 whitespace-nowrap"
            :class="[{ '!bg-brand/12 !text-brand font-medium': isActive(item) }, collapsed ? 'justify-center px-0' : '']"
            :title="collapsed ? item.label : undefined"
          >
            <component :is="item.icon" class="size-4 shrink-0" />
            <span v-if="!collapsed" class="truncate">{{ item.label }}</span>
          </router-link>
        </template>
      </nav>

      <div class="p-3 border-t border-line">
        <div class="flex items-center gap-2.5 px-1 mb-3" :class="collapsed ? 'justify-center px-0' : ''">
          <Avatar class="size-8 shrink-0">
            <AvatarFallback class="bg-brand/15 text-brand text-[12px] font-semibold">
              {{ initials }}
            </AvatarFallback>
          </Avatar>
          <div v-if="!collapsed" class="min-w-0">
            <div class="text-[13px] font-medium text-text truncate">{{ username }}</div>
            <div class="text-[11px] text-muted">{{ $t('common.admin') }}</div>
          </div>
        </div>
        <Button
          variant="ghost"
          size="sm"
          :class="collapsed ? 'w-9 px-0 justify-center' : 'w-full'"
          @click="logout"
        >
          <LogOut class="size-3.5 shrink-0" />
          <span v-if="!collapsed">{{ $t('common.logout') }}</span>
        </Button>
      </div>
    </aside>

    <!-- 主区域 -->
    <div class="flex-1 flex flex-col min-w-0">
      <header
        class="h-14 shrink-0 flex items-center gap-3 px-4 lg:px-6 border-b border-line bg-bg/80 backdrop-blur supports-[backdrop-filter]:bg-bg/60 sticky top-0 z-30"
      >
        <Button variant="icon" class="lg:hidden" @click="mobileOpen = true">
          <Menu class="size-4" />
        </Button>
        <div class="ml-auto flex items-center gap-1.5">
          <ToggleLocale />
          <ThemeToggle />
        </div>
      </header>

      <main class="flex-1 overflow-y-auto p-4 lg:p-6" :dir="$i18n.locale === 'ar' || $i18n.locale === 'fa' ? 'rtl' : 'ltr'">
        <router-view />
      </main>
    </div>

    <!-- 移动端 Sheet 导航 -->
    <Sheet :open="mobileOpen" @update:open="mobileOpen = $event">
      <SheetContent side="left" class="w-72 sm:max-w-sm p-0 gap-0">
        <div class="h-16 flex items-center justify-center border-b border-line">
          <AppLogo :size="30" compact />
        </div>
        <nav class="flex-1 overflow-y-auto px-3 py-4">
          <template v-for="group in navGroups" :key="group.label">
            <div v-if="group.label" class="px-3 pt-4 pb-1.5 text-[10.5px] font-semibold uppercase tracking-wider text-muted/70">
              {{ group.label }}
            </div>
            <router-link
              v-for="item in group.items"
              :key="item.to"
              :to="item.to"
              class="flex items-center gap-2.5 px-3 py-2 rounded-[0.5rem] text-[13px] text-muted hover:bg-surface2 hover:text-text transition-colors mb-0.5"
              :class="{ '!bg-brand/12 !text-brand font-medium': isActive(item) }"
              @click="mobileOpen = false"
            >
              <component :is="item.icon" class="size-4 shrink-0" />
              {{ item.label }}
            </router-link>
          </template>
        </nav>
        <div class="p-4 border-t border-line">
          <div class="flex items-center gap-2.5 px-1 mb-3">
            <Avatar class="size-8">
              <AvatarFallback class="bg-brand/15 text-brand text-[12px] font-semibold">
                {{ initials }}
              </AvatarFallback>
            </Avatar>
            <div class="min-w-0">
              <div class="text-[13px] font-medium text-text truncate">{{ username }}</div>
              <div class="text-[11px] text-muted">{{ $t('common.admin') }}</div>
            </div>
          </div>
          <Button variant="ghost" size="sm" class="w-full" @click="logout">
            <LogOut class="size-3.5" /> {{ $t('common.logout') }}
          </Button>
        </div>
      </SheetContent>
    </Sheet>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import {
  CreditCard, KeyRound, LayoutDashboard, LogOut, Menu, ScrollText,
  Settings, ShieldAlert, Users, type LucideIcon,
} from '@lucide/vue'
import { Avatar, AvatarFallback } from '@/components/ui/avatar'
import { Button } from '@/components/ui/button'
import { Sheet, SheetContent } from '@/components/ui/sheet'
import AppLogo from '../components/AppLogo.vue'
import ThemeToggle from '../components/ThemeToggle.vue'
import ToggleLocale from '../components/ToggleLocale.vue'
import { clearToken } from '../api'

const route = useRoute()
const router = useRouter()
const { t } = useI18n()
const mobileOpen = ref(false)
const collapsed = ref(true)
const username = ref(localStorage.getItem('dml_username') || 'admin')

const initials = computed(() => (username.value || 'A').slice(0, 1).toUpperCase())

interface NavItem {
  to: string
  label: string
  icon: LucideIcon
}
interface NavGroup {
  label: string
  items: NavItem[]
}

const navGroups = computed<NavGroup[]>(() => [
  {
    label: t('nav.groupMain'),
    items: [{ to: '/dashboard', label: t('nav.overview'), icon: LayoutDashboard }],
  },
  {
    label: t('nav.groupLicense'),
    items: [
      { to: '/licenses', label: t('nav.licenses'), icon: KeyRound },
      { to: '/customers', label: t('nav.customers'), icon: Users },
      { to: '/subscriptions', label: t('nav.subscriptions'), icon: CreditCard },
    ],
  },
  {
    label: t('nav.groupSystem'),
    items: [
      { to: '/security', label: t('nav.security'), icon: ShieldAlert },
      { to: '/audit', label: t('nav.audit'), icon: ScrollText },
    ],
  },
  {
    label: t('nav.groupSettings'),
    items: [{ to: '/settings', label: t('nav.settings'), icon: Settings }],
  },
])

function isActive(item: NavItem) {
  if (item.to === '/dashboard') return route.path === '/dashboard'
  return route.path.startsWith(item.to)
}

function logout() {
  clearToken()
  localStorage.removeItem('dml_username')
  router.push('/login')
}
</script>
