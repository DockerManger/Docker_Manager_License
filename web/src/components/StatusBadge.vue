<template>
  <span class="badge" :class="cls">{{ label }}</span>
</template>

<script setup lang="ts">
import { computed } from 'vue'

const props = defineProps<{ status: string }>()

const map: Record<string, { cls: string; label: string }> = {
  active: { cls: 'badge-ok', label: '有效' },
  expired: { cls: 'badge-warn', label: '已过期' },
  revoked: { cls: 'badge-danger', label: '已吊销' },
  suspended: { cls: 'badge-warn', label: '已挂起' },
}

const cls = computed(() => map[props.status]?.cls || 'badge-muted')
const label = computed(() => map[props.status]?.label || props.status)
</script>
