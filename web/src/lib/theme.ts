import { computed, ref } from 'vue'

const THEME_KEY = 'dml_theme'
export const theme = ref(localStorage.getItem(THEME_KEY) || 'dark')
export const isDark = computed(() => theme.value === 'dark')

export function applyTheme(t: string) {
  theme.value = t
  localStorage.setItem(THEME_KEY, t)
  document.documentElement.dataset.theme = t
}

export function initTheme() {
  applyTheme(localStorage.getItem(THEME_KEY) || 'dark')
}

/**
 * 主题切换 + 圆形扩散过渡(复刻 Valaxy toggleDarkWithTransition):
 * 从点击位置圆形展开/收缩,不支持 View Transitions API 时直接切换。
 */
export function toggleThemeWithTransition(event?: MouseEvent) {
  const next = theme.value === 'dark' ? 'light' : 'dark'
  if (!document.startViewTransition) {
    applyTheme(next)
    return
  }
  const x = event?.clientX ?? innerWidth / 2
  const y = event?.clientY ?? innerHeight / 2
  const endRadius = Math.hypot(Math.max(x, innerWidth - x), Math.max(y, innerHeight - y))

  const transition = document.startViewTransition(() => {
    applyTheme(next)
  })

  transition.ready.then(() => {
    const clipPath = [
      `circle(0px at ${x}px ${y}px)`,
      `circle(${endRadius}px at ${x}px ${y}px)`,
    ]
    document.documentElement.animate(
      {
        clipPath: theme.value === 'dark' ? clipPath.reverse() : clipPath,
      },
      {
        duration: 300,
        easing: 'ease-in',
        pseudoElement: theme.value === 'dark' ? '::view-transition-old(root)' : '::view-transition-new(root)',
      },
    )
  })
}
