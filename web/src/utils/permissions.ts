// 成员权限矩阵，与后端 server/medcare/permissions.go 的权威清单一一对应。
// 规则很简单：管理员逐格勾选，勾了才有，不勾就没有（包括「查看」）。
// 用药人可能是老人、孕妇、初中生或大学生，家里谁负责哪一格差别很大：
// 有人只负责看今天吃了没，有人负责买药入库，有人不该看到处方单和检查报告。
export type PermAction = 'view' | 'create' | 'edit' | 'delete' | 'remind' | 'record' | 'invite'

export interface PermCell {
  key: string
  action: PermAction
  /** 格子的表头文案 */
  label: string
}

export interface PermModule {
  module: string
  label: string
  /** 一行说明：这一行覆盖哪些操作 */
  hint: string
  actions: PermCell[]
}

const cell = (module: string, action: PermAction, label: string): PermCell => ({ key: module + ':' + action, action, label })

const crud = (module: string, labels: [string, string, string]): PermCell[] => [
  cell(module, 'view', '查看'),
  cell(module, 'create', labels[0]),
  cell(module, 'edit', labels[1]),
  cell(module, 'delete', labels[2]),
]

export const PERMISSION_MODULES: PermModule[] = [
  {
    module: 'today',
    label: '用药记录',
    hint: '今日与日历的用药安排、提醒、打卡与备注',
    actions: [cell('today', 'view', '查看'), cell('today', 'remind', '点提醒'), cell('today', 'record', '点完成')],
  },
  { module: 'medicine', label: '药品与库存', hint: '药品档案、批次、出入库与盘点', actions: crud('medicine', ['新增', '修改', '删除']) },
  { module: 'plan', label: '用药计划', hint: '计划的新建、改期、暂停终止，含地点库', actions: crud('plan', ['新建', '修改', '删除']) },
  { module: 'prescription', label: '处方识别', hint: '拍照识别处方/药盒/缴费单与确认入库', actions: crud('prescription', ['识别', '确认录入', '删除']) },
  { module: 'image', label: '图片资料', hint: '药品照片、处方单、治疗单、检查报告', actions: crud('image', ['上传', '编辑', '删除']) },
  { module: 'stats', label: '统计与日志', hint: '依从性统计、操作日志、库存流水', actions: [cell('stats', 'view', '查看')] },
  { module: 'member', label: '成员与邀请', hint: '查看成员、邀请家人；改角色与设权限只限管理员', actions: [cell('member', 'view', '查看'), cell('member', 'invite', '邀请')] },
]

export const ALL_PERMISSION_KEYS: string[] = PERMISSION_MODULES.flatMap((m) => m.actions.map((a) => a.key))
export const VIEW_KEYS: string[] = PERMISSION_MODULES.flatMap((m) => m.actions.filter((a) => a.action === 'view').map((a) => a.key))

/** 角色默认可见范围，与后端 memberViewKeys 一致：能看能催，不能改。 */
export const MEMBER_DEFAULT_KEYS = [
  'today:view', 'today:remind',
  'medicine:view', 'plan:view', 'prescription:view', 'image:view',
  'stats:view', 'member:view', 'member:invite',
]

/** 常用预设：管理员一键给新人一套合理权限，再手动微调。 */
export interface PermissionPreset {
  key: string
  label: string
  hint: string
  keys: string[]
}

export const PERMISSION_PRESETS: PermissionPreset[] = [
  { key: 'viewer', label: '只读', hint: '只能看安排，什么都不能改', keys: [...VIEW_KEYS, 'member:view'] },
  { key: 'carer', label: '帮忙照看', hint: '看今日 + 点提醒 + 点完成（替老人打卡）', keys: ['today:view', 'today:remind', 'today:record', 'medicine:view', 'plan:view', 'stats:view', 'member:view'] },
  { key: 'pharmacy', label: '管药与计划', hint: '买药入库、排计划改计划、传图片', keys: [...crud('medicine', ['新增', '修改', '删除']).map((a) => a.key), ...crud('plan', ['新建', '修改', '删除']).map((a) => a.key), ...crud('prescription', ['识别', '确认录入', '删除']).map((a) => a.key), ...crud('image', ['上传', '编辑', '删除']).map((a) => a.key), 'today:view', 'today:remind', 'stats:view', 'member:view'] },
  { key: 'all', label: '全部', hint: '除改角色与设权限外都能动', keys: [...ALL_PERMISSION_KEYS] },
]

export function moduleOf(key: string): string {
  return String(key || '').split(':')[0]
}

export function cellLabel(key: string): string {
  for (const module of PERMISSION_MODULES) {
    const hit = module.actions.find((a) => a.key === key)
    if (hit) return module.label + ' · ' + hit.label
  }
  return key
}

/** 紧凑标签，用于成员行上的能力徽标。 */
export function shortCaps(keys: string[]): string[] {
  const out: string[] = []
  for (const module of PERMISSION_MODULES) {
    const owned = module.actions.filter((a) => keys.includes(a.key))
    if (!owned.length) continue
    if (owned.length === module.actions.length) out.push(module.label.replace('与库存', '').replace('与邀请', '').replace('与日志', ''))
    else if (owned.every((a) => a.action === 'view')) out.push('看' + module.label.replace('与库存', '').replace('与邀请', '').replace('与日志', ''))
    else out.push(module.label.replace('与库存', '').replace('与邀请', '').replace('与日志', '') + '部分')
  }
  return out
}

/** 后端没下发 permissions 时的兜底（连到旧版服务时不至于全灰）。 */
export function fallbackKeysFor(role: string | undefined, isCareRecipient: boolean | undefined): string[] {
  if (role === 'owner' || role === 'manager') return [...ALL_PERMISSION_KEYS]
  const keys = [...MEMBER_DEFAULT_KEYS]
  if (isCareRecipient) keys.push('today:record')
  return keys
}

/** 只认认识的键，顺序按矩阵走；未知键丢掉，避免脏数据把界面搞乱。 */
export function normalizeKeys(input: unknown): string[] {
  const raw = Array.isArray(input) ? input.filter((k): k is string => typeof k === 'string') : []
  return ALL_PERMISSION_KEYS.filter((key) => raw.includes(key))
}
