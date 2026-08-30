// 日期 / 数值格式化工具

export function fmtDate(ts: number): string {
  if (!ts) return '-'
  return new Date(ts * 1000).toLocaleDateString('zh-CN')
}

export function fmtDateTime(ts: number): string {
  if (!ts) return '-'
  return new Date(ts * 1000).toLocaleString('zh-CN', { hour12: false })
}

export function fmtDateTimeStr(s: string | null | undefined): string {
  if (!s) return '-'
  const d = new Date(s)
  return isNaN(d.getTime()) ? '-' : d.toLocaleString('zh-CN', { hour12: false })
}

export function fmtRelative(ts: number): string {
  if (!ts) return '-'
  const diff = ts * 1000 - Date.now()
  const abs = Math.abs(diff)
  const day = 86400000
  if (abs < day) return diff >= 0 ? '今天' : '今天'
  const days = Math.round(abs / day)
  if (abs < 30 * day) return diff >= 0 ? `${days} 天后` : `${days} 天前`
  if (abs < 365 * day) return diff >= 0 ? `${Math.round(abs / (30 * day))} 个月后` : `${Math.round(abs / (30 * day))} 个月前`
  return diff >= 0 ? `${(abs / (365 * day)).toFixed(1)} 年后` : `${(abs / (365 * day)).toFixed(1)} 年前`
}

export function fmtBytes(n: number): string {
  if (n == null || isNaN(n)) return '-'
  if (n < 1024) return `${n} B`
  const units = ['KB', 'MB', 'GB', 'TB']
  let v = n
  let i = -1
  do {
    v /= 1024
    i++
  } while (v >= 1024 && i < units.length - 1)
  return `${v.toFixed(1)} ${units[i]}`
}

export function shortId(s: string, len = 8): string {
  if (!s) return '-'
  return s.length > len ? s.slice(0, len) + '…' : s
}
