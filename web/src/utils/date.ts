// 全部时间统一按北京时间（Asia/Shanghai）展示与计算。
// 设备/浏览器时区不再影响显示，避免在非中国时区的手机或电脑上看错时间。
export const BEIJING_TZ = 'Asia/Shanghai'

const dateFmt = new Intl.DateTimeFormat('en-CA', {
  timeZone: BEIJING_TZ, year: 'numeric', month: '2-digit', day: '2-digit',
})

const partsFmt = new Intl.DateTimeFormat('en-GB', {
  timeZone: BEIJING_TZ, year: 'numeric', month: '2-digit', day: '2-digit',
  hour: '2-digit', minute: '2-digit', hour12: false,
})

/** 北京时间的 YYYY-MM-DD */
export function localDate(d: Date = new Date()): string {
  return dateFmt.format(d)
}

/** 现在（北京时间）的 YYYY-MM-DD HH:MM */
export function nowLocalString(): string {
  const p = partsFmt.formatToParts(new Date())
  const get = (t: string) => p.find((x) => x.type === t)?.value || ''
  return `${get('year')}-${get('month')}-${get('day')} ${get('hour')}:${get('minute')}`
}

/** 在 YYYY-MM-DD 上加减天数（纯字符串运算，不受时区影响） */
export function addDays(date: string, days: number): string {
  const [y, m, d] = date.split('-').map(Number)
  const t = new Date(Date.UTC(y, m - 1, d))
  t.setUTCDate(t.getUTCDate() + days)
  const yy = t.getUTCFullYear()
  const mm = String(t.getUTCMonth() + 1).padStart(2, '0')
  const dd = String(t.getUTCDate()).padStart(2, '0')
  return `${yy}-${mm}-${dd}`
}

/** ISO/RFC3339 → 北京时间的 { date, time } */
export function beijingParts(iso: string): { date: string; time: string } {
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return { date: '', time: '' }
  const p = partsFmt.formatToParts(d)
  const get = (t: string) => p.find((x) => x.type === t)?.value || ''
  return { date: `${get('year')}-${get('month')}-${get('day')}`, time: `${get('hour')}:${get('minute')}` }
}

/** 北京时间的 HH:MM */
export function formatTime(iso: string): string {
  return beijingParts(iso).time
}

/** 北京时间的 YYYY/MM/DD */
export function formatDate(iso: string): string {
  const { date } = beijingParts(iso)
  return date ? date.replace(/-/g, '/') : ''
}

/** 北京时间的 M月D日 HH:MM */
export function formatDateTime(iso: string, withYear = false): string {
  const { date, time } = beijingParts(iso)
  if (!date) return ''
  const [y, m, d] = date.split('-')
  const day = `${Number(m)}月${Number(d)}日`
  return `${withYear ? y + '年' : ''}${day} ${time}`
}

/** 北京时间下的星期（短），如「周二」 */
export function weekdayShort(date: string): string {
  const d = new Date(`${date}T12:00:00+08:00`)
  return new Intl.DateTimeFormat('zh-CN', { timeZone: BEIJING_TZ, weekday: 'short' }).format(d)
}

/** 北京时间下的星期（长），如「星期二」 */
export function weekdayLong(date: string): string {
  const d = new Date(`${date}T12:00:00+08:00`)
  return new Intl.DateTimeFormat('zh-CN', { timeZone: BEIJING_TZ, weekday: 'long' }).format(d)
}

/** 把北京时间的「YYYY-MM-DD + HH:MM」转成 UTC ISO 字符串 */
export function beijingToISO(date: string, time: string): string {
  return new Date(`${date}T${time}:00+08:00`).toISOString()
}

/** 北京时间的月份信息，用于日历 */
export function beijingMonthOf(date: string): { year: number; month: number } {
  const [y, m] = date.split('-').map(Number)
  return { year: y, month: m }
}

/** 日历格子所需的「某年某月」的天数（按 UTC 计算，避免夏令时影响） */
export function daysInMonth(year: number, month: number): number {
  return new Date(Date.UTC(year, month, 0)).getUTCDate()
}

/** 某年某月 1 号是周几（返回 0=周一 … 6=周日） */
export function firstWeekdayOffset(year: number, month: number): number {
  const day = new Date(Date.UTC(year, month - 1, 1)).getUTCDay()
  return (day + 6) % 7
}
