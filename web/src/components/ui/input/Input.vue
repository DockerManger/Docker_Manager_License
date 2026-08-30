<script setup lang="ts">
import type { HTMLAttributes } from "vue"
import { computed } from "vue"
import { cn } from "@/lib/utils"

const props = withDefaults(
  defineProps<{
    defaultValue?: string | number
    modelValue?: string | number
    modelModifiers?: { number?: boolean; trim?: boolean }
    class?: HTMLAttributes["class"]
  }>(),
  {
    modelModifiers: () => ({}),
  },
)

const emits = defineEmits<{
  (e: "update:modelValue", payload: string | number): void
}>()

// 支持 v-model.number / v-model.trim(与原生行为一致:parseFloat 失败保留原值)
const modelValue = computed({
  get: () => props.modelValue ?? props.defaultValue ?? "",
  set: (v: string | number) => {
    if (props.modelModifiers?.number) {
      const n = parseFloat(String(v))
      v = Number.isNaN(n) ? v : n
    }
    if (props.modelModifiers?.trim) v = String(v).trim()
    emits("update:modelValue", v)
  },
})
</script>

<template>
  <input
    v-model="modelValue"
    data-slot="input"
    :class="cn(
      'file:text-foreground placeholder:text-muted-foreground selection:bg-primary selection:text-primary-foreground border-input bg-input h-9 w-full min-w-0 rounded-[0.5rem] border px-3 py-1.5 text-[13px] text-text shadow-none transition-[color,box-shadow] outline-none file:inline-flex file:h-7 file:border-0 file:bg-transparent file:text-sm file:font-medium disabled:pointer-events-none disabled:cursor-not-allowed disabled:opacity-50',
      'focus-visible:border-brand focus-visible:ring-brand/30 focus-visible:ring-2',
      'aria-invalid:ring-danger/20 aria-invalid:border-danger',
      props.class,
    )"
  >
</template>
