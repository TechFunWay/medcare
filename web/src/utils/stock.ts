// 出入库相关的公共文案与颜色，详情页、全局明细页、识别导入共用。
export const STOCK_CHANNEL_OPTIONS = [
  { value: 'pharmacy', label: '药店购买' },
  { value: 'hospital', label: '医院开具' },
  { value: 'other', label: '其他' },
]

export function channelLabel(code?: string): string {
  switch (code) {
    case 'pharmacy': return '药店购买'
    case 'hospital': return '医院开具'
    case 'other': return '其他'
    default: return '未标注'
  }
}

export function entrySourceLabel(code?: string): string {
  switch (code) {
    case 'manual': return '手动录入'
    case 'box': return '药盒识别'
    case 'receipt': return '缴费单识别'
    case 'prescription': return '处方识别'
    case 'completion': return '完成用药'
    case 'return': return '撤销退回'
    case 'adjust': return '库存调整'
    case 'waste': return '丢弃报损'
    case 'stocktake': return '盘点'
    default: return '手工记录'
  }
}

export function movementTypeLabel(type: string): string {
  switch (type) {
    case 'in': return '入库'
    case 'out': return '用药扣减'
    case 'return': return '撤销退回'
    case 'adjust': return '库存调整'
    case 'waste': return '丢弃报损'
    default: return '变动'
  }
}

export function movementBadge(type: string): string {
  switch (type) {
    case 'in': return 'done'
    case 'out': return 'missed'
    case 'return': return 'done'
    case 'waste': return 'warn'
    default: return 'pending'
  }
}
// 手动入库的表单草稿：详情页与库存页共用同一套字段与默认值，避免两处表单不一致。
export interface StockInDraft {
  channel: string
  packages: number
  units_per_package: number
  quantity: number
  batch_no: string
  expiry_date: string
  supplier: string
  note: string
}

export function emptyStockInDraft(): StockInDraft {
  return { channel: 'pharmacy', packages: 0, units_per_package: 0, quantity: 0, batch_no: '', expiry_date: '', supplier: '', note: '' }
}

// 实际入库数量：优先直接填写的数量，否则用「包装数 × 每包装含量」。
export function stockInQuantity(d: StockInDraft): number {
  const q = Number(d.quantity) || 0
  if (q > 0) return q
  const p = Number(d.packages) || 0
  const u = Number(d.units_per_package) || 0
  return p > 0 && u > 0 ? Math.round(p * u * 1000) / 1000 : 0
}
export interface StockOutDraft { quantity: number; channel: string; reason: string }
export function emptyStockOutDraft(): StockOutDraft { return { quantity: 0, channel: 'other', reason: '' } }

export interface StocktakeDraft { target_quantity: number; reason: string }
export function emptyStocktakeDraft(current = 0): StocktakeDraft { return { target_quantity: current, reason: '' } }
