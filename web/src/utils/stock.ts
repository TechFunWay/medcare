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
