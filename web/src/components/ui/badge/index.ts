import type { VariantProps } from "class-variance-authority"
import { cva } from "class-variance-authority"

export { default as Badge } from "./Badge.vue"

/** License 面板徽章:语义色(ok/warn/danger/info)+ 中性 default/outline */
export const badgeVariants = cva(
  "inline-flex items-center gap-1 rounded-full border px-2 py-0.5 text-[11px] font-semibold leading-none w-fit whitespace-nowrap shrink-0 transition-colors",
  {
    variants: {
      variant: {
        default: "border-transparent bg-surface2 text-muted",
        secondary: "border-transparent bg-surface2 text-muted",
        brand: "border-brand/30 bg-brand/12 text-brand",
        success: "border-ok/30 bg-ok/12 text-ok",
        warning: "border-warn/30 bg-warn/12 text-warn",
        destructive: "border-danger/30 bg-danger/12 text-danger",
        info: "border-info/30 bg-info/12 text-info",
        outline: "border-line text-muted",
      },
    },
    defaultVariants: {
      variant: "default",
    },
  },
)
export type BadgeVariants = VariantProps<typeof badgeVariants>
