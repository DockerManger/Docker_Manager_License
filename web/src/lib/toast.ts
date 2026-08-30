import { reactive } from 'vue'

export interface ToastItem {
  id: number
  type: 'success' | 'error' | 'info'
  message: string
}

let seq = 0
export const toasts = reactive<ToastItem[]>([])

export function toast(message: string, type: ToastItem['type'] = 'info') {
  const id = ++seq
  toasts.push({ id, type, message })
  setTimeout(() => dismiss(id), 3200)
}

export function toastOk(message: string) {
  toast(message, 'success')
}

export function toastErr(message: string) {
  toast(message, 'error')
}

export function dismiss(id: number) {
  const i = toasts.findIndex((t) => t.id === id)
  if (i !== -1) toasts.splice(i, 1)
}
