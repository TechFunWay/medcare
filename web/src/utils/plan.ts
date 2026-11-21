// 用药计划的展示文案：计划列表页与药品详情里的「关联计划」共用一套，避免文案不一致。
import type { Plan } from '../api/medcare'

export function planStatusLabel(s: string): string {
  return s === 'active' ? '进行中' : s === 'paused' ? '已暂停' : s === 'completed' ? '已完成' : s === 'ended' ? '已结束' : '已归档'
}

export function planStatusBadge(s: string): string {
  if (s === 'active') return 'done'
  if (s === 'completed') return 'done'
  if (s === 'paused') return 'warn'
  return 'pending'
}

export function frequencyLabel(p: Plan): string {
  const f = p.frequency || ({} as Plan['frequency'])
  if (f.type === 'daily') return '每天 ' + (f.times || []).join('、')
  if (f.type === 'every_n_days') return '每 ' + (f.interval_days || 1) + ' 天 ' + (f.times || []).join('、')
  if (f.type === 'weekly') return '每周 ' + (f.weekdays || []).join('、') + ' ' + (f.times || []).join('、')
  if (f.type === 'custom_dates') {
    const days = f.custom_schedule || []
    if (!days.length) return '指定日期'
    const first = days[0]
    return '指定日期 ' + days.length + ' 天（' + first.date.slice(5) + ' ' + (first.times || []).join('、') + (days.length > 1 ? ' 等' : '') + '）'
  }
  return '每 ' + (f.interval_hours || 0) + ' 小时'
}

export function planProgressText(p: Plan): string {
  const total = Number(p.total_count || 0)
  if (!total) return ''
  return '已完成 ' + Number(p.completed_count || 0) + ' / ' + total
}
