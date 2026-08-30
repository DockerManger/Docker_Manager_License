<script setup lang="ts">
import type { VariantProps } from "class-variance-authority"
import type { HTMLAttributes } from "vue"
import { Primitive } from "reka-ui"
import { cn } from "@/lib/utils"
import { buttonVariants } from "."

const props = withDefaults(
  defineProps<{
    variant?: VariantProps<typeof buttonVariants>["variant"]
    size?: VariantProps<typeof buttonVariants>["size"]
    class?: HTMLAttributes["class"]
    asChild?: boolean
    as?: string
    // 默认 type="button" 防止在 <form> 内误触发提交
    type?: string
  }>(),
  {
    type: "button",
    as: "button",
  },
)
</script>

<template>
  <Primitive
    data-slot="button"
    :data-variant="variant"
    :data-size="size"
    :as="as"
    :as-child="asChild"
    :type="type"
    :class="cn(buttonVariants({ variant, size }), props.class)"
  >
    <slot />
  </Primitive>
</template>
