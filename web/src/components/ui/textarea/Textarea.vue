<script setup lang="ts">
import type { TextareaHTMLAttributes } from "vue"
import { useVModel } from "@vueuse/core"
import { cn } from "@/lib/utils"

const props = defineProps<{
  class?: TextareaHTMLAttributes["class"]
  defaultValue?: string | number
  modelValue?: string | number
}>()

const emits = defineEmits<{
  (e: "update:modelValue", payload: string | number): void
}>()

const modelValue = useVModel(props, "modelValue", emits, {
  passive: true,
  defaultValue: props.defaultValue,
})
</script>

<template>
  <textarea
    v-model="modelValue"
    data-slot="textarea"
    :class="
      cn(
        'border-input bg-input placeholder:text-muted-foreground focus-visible:border-brand focus-visible:ring-brand/30 focus-visible:ring-2 aria-invalid:ring-danger/20 aria-invalid:border-danger flex field-sizing-content min-h-16 w-full rounded-[0.5rem] border px-3 py-2 text-[13px] leading-relaxed shadow-none transition-[color,box-shadow] outline-none disabled:cursor-not-allowed disabled:opacity-50',
        props.class,
      )"
  />
</template>
