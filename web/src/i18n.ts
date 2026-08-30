import { createI18n } from 'vue-i18n'
import zhCN from './locales/zh-CN'
import zhTW from './locales/zh-TW'
import en from './locales/en'
import ja from './locales/ja'
import ko from './locales/ko'
import ar from './locales/ar'
import es from './locales/es'
import fa from './locales/fa'
import id from './locales/id'
import ptBR from './locales/pt-BR'
import ru from './locales/ru'
import tr from './locales/tr'
import uk from './locales/uk'
import vi from './locales/vi'

const LANG_KEY = 'dml_lang'

/** 14 种语言:与 Docker_Manager_Go 完全一致(顺序即语言菜单展示顺序) */
export const LANGS = [
  { code: 'zh-CN', label: '简体中文', flag: '🇨🇳', rtl: false },
  { code: 'en', label: 'English', flag: '🇺🇸', rtl: false },
  { code: 'zh-TW', label: '繁體中文', flag: '🇹🇼', rtl: false },
  { code: 'ja', label: '日本語', flag: '🇯🇵', rtl: false },
  { code: 'ko', label: '한국어', flag: '🇰🇷', rtl: false },
  { code: 'ru', label: 'Русский', flag: '🇷🇺', rtl: false },
  { code: 'vi', label: 'Tiếng Việt', flag: '🇻🇳', rtl: false },
  { code: 'es', label: 'Español', flag: '🇪🇸', rtl: false },
  { code: 'id', label: 'Bahasa Indonesia', flag: '🇮🇩', rtl: false },
  { code: 'uk', label: 'Українська', flag: '🇺🇦', rtl: false },
  { code: 'tr', label: 'Türkçe', flag: '🇹🇷', rtl: false },
  { code: 'pt-BR', label: 'Português', flag: '🇧🇷', rtl: false },
  { code: 'ar', label: 'العربية', flag: '🇪🇬', rtl: true },
  { code: 'fa', label: 'فارسی', flag: '🇮🇷', rtl: true },
]

const SUPPORTED = LANGS.map((l) => l.code)
const DEFAULT_LANG = 'zh-CN'

function applyDir(code: string) {
  const lang = LANGS.find((l) => l.code === code)
  document.documentElement.dir = lang?.rtl ? 'rtl' : 'ltr'
}

/** 浏览器语言自动检测:精确匹配 → 语言前缀匹配 → 默认 */
export function detectLang(): string {
  try {
    const saved = localStorage.getItem(LANG_KEY)
    if (saved && SUPPORTED.includes(saved)) return saved
    const browserLangs = navigator.languages || [navigator.language || DEFAULT_LANG]
    for (const bl of browserLangs) {
      if (SUPPORTED.includes(bl)) return bl
    }
    for (const bl of browserLangs) {
      const prefix = bl.split('-')[0]
      const matched = SUPPORTED.find((l) => l.startsWith(prefix))
      if (matched) return matched
    }
    return DEFAULT_LANG
  } catch {
    return DEFAULT_LANG
  }
}

const i18n = createI18n({
  legacy: false,
  locale: detectLang(),
  fallbackLocale: 'en',
  messages: {
    'zh-CN': zhCN,
    'zh-TW': zhTW,
    en,
    ja,
    ko,
    ar,
    es,
    fa,
    id,
    'pt-BR': ptBR,
    ru,
    tr,
    uk,
    vi,
  },
})

applyDir(i18n.global.locale.value)

export function setLang(code: string) {
  if (!SUPPORTED.includes(code)) return
  i18n.global.locale.value = code as never
  localStorage.setItem(LANG_KEY, code)
  document.documentElement.lang = code
  applyDir(code)
}

export default i18n
