import type { VariantProps } from "class-variance-authority"
import { cva } from "class-variance-authority"

export { default as Button } from "./Button.vue"

/**
 * License 面板风格按钮:紧凑 0.5rem 圆角 / 13px 字号,变体语义化。
 * 品牌粉只用于 primary(brand),危险操作用 destructive,中性操作 ghost。
 */
export const buttonVariants = cva(
  "inline-flex items-center justify-center gap-1.5 whitespace-nowrap rounded-[0.5rem] text-[13px] font-medium border border-transparent transition-all duration-150 outline-none select-none cursor-pointer disabled:cursor-not-allowed disabled:opacity-45 disabled:pointer-events-none hover:-translate-y-px active:translate-y-0 focus-visible:ring-2 focus-visible:ring-brand/40 [&_svg]:pointer-events-none [&_svg]:shrink-0",
  {
    variants: {
      variant: {
        default: "text-text bg-transparent",
        primary: "bg-brand text-white hover:bg-brand-strong",
        brand: "bg-brand text-white hover:bg-brand-strong",
        ghost: "border-line text-muted bg-transparent hover:border-line hover:text-text hover:bg-surface2",
        outline: "border-line text-text bg-transparent hover:bg-surface2",
        secondary: "bg-surface2 text-text hover:bg-surface2/80 border-transparent",
        ok: "bg-ok/12 text-ok border-ok/30 hover:bg-ok/25",
        warning: "bg-warn/12 text-warn border-warn/30 hover:bg-warn/25",
        destructive: "bg-danger/12 text-danger border-danger/30 hover:bg-danger/25",
        icon: "p-1.5 rounded-[0.5rem] border-line text-muted bg-transparent hover:border-line hover:text-text hover:bg-surface2",
      },
      size: {
        default: "px-3 py-1.5",
        sm: "px-2.5 py-1 text-[12px] rounded-[0.4rem]",
        xs: "px-2 py-0.5 text-[11px] rounded-[0.35rem]",
        icon: "p-1.5 rounded-[0.5rem]",
        "icon-sm": "p-1 rounded-[0.4rem]",
        lg: "px-5 py-2.5 text-[14px]",
      },
    },
    defaultVariants: {
      variant: "default",
      size: "default",
    },
  },
)
export type ButtonVariants = VariantProps<typeof buttonVariants>
