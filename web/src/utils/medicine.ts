// 删除药品前的统一检查：先问后端这个药品绑定了什么（计划 / 库存 / 出入库流水），
// 有进行中计划或还有库存就不让删，只提示的项（历史流水）写进确认文案。
// 口径与后端 handleMedicineRelations / handleArchiveMedicine 完全一致。
import { medicineApi } from '../api/medcare'

export interface MedicineRelations {
  medicine_id: number
  name: string
  unit: string
  active_plan_count: number
  plan_count: number
  batch_count: number
  stock_quantity: number
  movement_count: number
  can_delete: boolean
  blockers: { key: string; label: string; hint?: string }[]
  notes: string[]
}

interface UIConfirm {
  confirm: (o: { title: string; message: string; confirmText?: string; danger?: boolean; hideCancel?: boolean }) => Promise<boolean>
  showToast: (msg: string, type?: 'success' | 'error' | 'info', durationMs?: number) => void
}

/** 返回 true 表示已经删除成功；false 表示用户取消或被绑定关系挡住。 */
export async function tryDeleteMedicine(ui: UIConfirm, bookId: number, m: { id: number; name: string }): Promise<boolean> {
  let rel: MedicineRelations
  try {
    const res = await medicineApi.relations(bookId, m.id)
    rel = res.data?.data as MedicineRelations
  } catch (e: any) {
    ui.showToast(e?.response?.data?.message || '读取药品关联失败', 'error')
    return false
  }
  if (!rel) { ui.showToast('读取药品关联失败', 'error'); return false }

  const bindingLines = (rel.blockers || []).map((b) => '· ' + b.label + (b.hint ? '——' + b.hint : ''))
  const noteLines = (rel.notes || []).map((n) => '· ' + n)

  // 绑定关系只做提示，不阻止删除：删除会把进行中的计划一并结束，库存与流水保留在库里。
  const warnBlock = bindingLines.length || noteLines.length
    ? ['这个药品还有这些关联，删除后会这样处理：', ...bindingLines, ...noteLines, '']
    : []
  const ok = await ui.confirm({
    title: '删除「' + m.name + '」？',
    message: [...warnBlock, '删除后该药品从列表消失，历史出入库与用药记录保留，操作会记进审计日志。'].join('\n').trim(),
    confirmText: bindingLines.length ? '仍要删除' : '删除',
  })
  if (!ok) return false
  try {
    await medicineApi.archive(bookId, m.id)
    return true
  } catch (e: any) {
    ui.showToast(e?.response?.data?.message || '删除失败', 'error')
    return false
  }
}
