<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { Badge, type BadgeVariants } from '@/components/ui/badge'

const props = defineProps<{
  status: string
}>()

const { t } = useI18n()

// 语义化状态映射:License / Activation / Subscription 通用
const MAP: Record<string, { variant: BadgeVariants['variant']; labelKey: string }> = {
  active: { variant: 'success', labelKey: 'status.active' },
  trial: { variant: 'info', labelKey: 'status.trial' },
  expired: { variant: 'warning', labelKey: 'status.expired' },
  revoked: { variant: 'destructive', labelKey: 'status.revoked' },
  suspended: { variant: 'warning', labelKey: 'status.suspended' },
  cancelled: { variant: 'default', labelKey: 'status.cancelled' },
  deactivated: { variant: 'default', labelKey: 'status.deactivated' },
  ok: { variant: 'success', labelKey: 'status.ok' },
  success: { variant: 'success', labelKey: 'status.success' },
  failed: { variant: 'destructive', labelKey: 'status.failed' },
  error: { variant: 'destructive', labelKey: 'status.error' },
}

const meta = computed(() => MAP[props.status] || { variant: 'default' as const, labelKey: '' })
const label = computed(() => (meta.value.labelKey ? t(meta.value.labelKey) : props.status))

const dotColor = computed(() => {
  const v = meta.value.variant || 'default'
  return {
    success: '#34d399',
    warning: '#fbbf24',
    destructive: '#f87171',
    info: '#60a5fa',
    brand: '#ec4899',
    default: '#8b93a3',
    outline: '#8b93a3',
    secondary: '#8b93a3',
  }[v]
})
</script>

<template>
  <Badge :variant="meta.variant">
    <span class="w-1.5 h-1.5 rounded-full shrink-0" :style="{ background: dotColor }" />
    {{ label }}
  </Badge>
</template>
