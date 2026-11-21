// 药品基础信息的常用选项：剂型 / 库存单位 / 规格模板。
// 统一走下拉（可搜索 + 允许自定义），避免自由输入把同一件东西写成十几种叫法。

const toOptions = (list: string[]) => list.map((v) => ({ value: v, label: v }))

export const FORM_OPTIONS = toOptions([
  '片剂', '胶囊', '缓释片', '控释片', '分散片', '咀嚼片', '颗粒剂', '干混悬剂',
  '口服液', '口服溶液', '注射液', '粉针剂', '输液', '软膏', '乳膏', '凝胶',
  '滴眼液', '滴耳液', '滴鼻液', '喷雾剂', '气雾剂', '贴剂', '栓剂', '其他',
])

export const UNIT_OPTIONS = toOptions([
  '片', '粒', '颗', '袋', '支', '瓶', '盒', '板', '包', '管', '贴', '喷', '滴', '次',
  'ml', 'g', 'mg', 'IU',
])

// 规格模板：含量规格与常见包装写法混排，选不到就自定义。
export const SPEC_OPTIONS = toOptions([
  '5mg', '10mg', '20mg', '25mg', '50mg', '75mg', '100mg', '150mg', '200mg', '250mg', '500mg',
  '0.1g', '0.2g', '0.25g', '0.5g', '1g', '2g',
  '5ml', '10ml', '20ml', '50ml', '100ml', '250ml', '500ml',
  '10片/盒', '12片/盒', '20片/盒', '24片/盒', '30片/盒', '100片/瓶',
  '1支/盒', '1瓶/盒', '10袋/盒',
])

export const PACKAGE_SPEC_OPTIONS = toOptions([
  '10片/盒', '12片/盒', '20片/盒', '24片/盒', '30片/盒', '36片/盒', '48片/盒',
  '60片/瓶', '100片/瓶', '1支/盒', '5支/盒', '10支/盒', '1瓶/盒', '1袋/盒', '10袋/盒', '1板/盒',
])

// 管理员在「偏好设置 → 药品字典」里维护的自定义候选值（逗号分隔，存系统配置）。
// 非空时覆盖内置默认值；为空表示用内置默认。
export interface MedicineDict {
  form: string
  unit: string
  spec: string
  packageSpec: string
}

let dict: MedicineDict = { form: '', unit: '', spec: '', packageSpec: '' }
// 用配置值覆盖默认字典（应用启动后由设置页 / 全局初始化调用）。
export function applyMedicineDict(next: Partial<MedicineDict>) {
  dict = { ...dict, ...next }
}
export function getMedicineDict(): MedicineDict { return dict }

function splitList(raw: string): string[] {
  return String(raw || '')
    .split(/[,，\n]/)
    .map((v) => v.trim())
    .filter(Boolean)
}

// 字典以数据库为准：库里有值就只用库里的（已去重），库里为空才回落到内置默认，
// 这样管理员在设置页删掉的项不会又被内置默认塞回来。
function mergeOptions(builtin: { value: string; label: string }[], custom: string) {
  const list = dedupe(splitList(custom))
  if (!list.length) return builtin
  return list.map((v) => ({ value: v, label: v }))
}

function dedupe(list: string[]): string[] {
  const seen = new Set<string>()
  return list.filter((v) => (seen.has(v) ? false : (seen.add(v), true)))
}

// 页面用这四个取当前候选值（内置 + 管理员自定义）。
export function formOptions() { return mergeOptions(FORM_OPTIONS, dict.form) }
export function unitOptions() { return mergeOptions(UNIT_OPTIONS, dict.unit) }
export function specOptions() { return mergeOptions(SPEC_OPTIONS, dict.spec) }
export function packageSpecOptions() { return mergeOptions(PACKAGE_SPEC_OPTIONS, dict.packageSpec) }

// defaultUnitFor：模型没给库存单位时按剂型/分类推一个合理默认，和前端下拉保持一致。
// 与服务端 defaultDoseUnit 同口径（注射剂→支、输液→瓶、颗粒→袋、片剂胶囊→盒、外用膏→支）。
export function defaultUnitFor(form?: string, category?: string): string {
  const f = String(form || '')
  const rules: [string[], string][] = [
    [['注射液', '注射剂', '粉针', '冻干'], '支'],
    [['输液', '大容量'], '瓶'],
    [['滴眼', '滴耳', '滴鼻'], '支'],
    [['颗粒', '散剂', '冲剂', '干混悬'], '袋'],
    [['口服液', '溶液', '糖浆', '合剂'], '瓶'],
    [['喷雾', '气雾'], '瓶'],
    [['软膏', '乳膏', '凝胶', '搽剂'], '支'],
    [['贴'], '贴'],
    [['片', '胶囊', '丸', '栓'], '盒'],
  ]
  for (const [keys, unit] of rules) {
    if (keys.some((k) => f.includes(k))) return unit
  }
  if (category === 'injection') return '支'
  if (category === 'infusion') return '瓶'
  if (category === 'topical') return '支'
  return '盒'
}

// 从规格末尾读库存单位：2mg*28片→片、0.2g×10片→片、10mg/支→支、20片/盒→片。
// 只认可数的最小单位，不把 g/ml/mg 当库存单位（与服务端 unitFromSpec 同口径）。
const COUNT_UNITS = ['片', '粒', '颗', '丸', '枚', '袋', '支', '瓶', '包', '管', '贴', '喷', '滴', '揿', '罐', '板', '盒']
function tailUnit(raw: string): string | null {
  const s = String(raw || '').trim()
  if (!s) return null
  for (const u of COUNT_UNITS) if (s.endsWith(u)) return u
  return null
}
export function unitFromSpec(spec?: string): string | null {
  const s = String(spec || '').trim()
  if (!s) return null
  let dosePart = s
  let packPart = ''
  const slash = s.lastIndexOf('/')
  if (slash >= 0) { dosePart = s.slice(0, slash).trim(); packPart = s.slice(slash + 1).trim() }
  for (const sep of ['*', '×', 'x', 'X']) {
    const idx = dosePart.lastIndexOf(sep)
    if (idx >= 0) {
      const u = tailUnit(dosePart.slice(idx + sep.length))
      if (u) return u
    }
  }
  const du = tailUnit(dosePart)
  if (du) return du
  if (packPart) return tailUnit(packPart)
  return null
}

// fixUnit：统一决定一条药品的库存单位。
// 规格能读出可数单位、且当前为空或只是包装单位（盒/箱/板）时，以规格为准；否则用当前值；仍为空按剂型兜底。
export function fixUnit(unit?: string, form?: string, category?: string, spec?: string): string {
  const cur = String(unit || '').trim()
  const packaging = !cur || cur === '盒' || cur === '箱' || cur === '板'
  const fromSpec = unitFromSpec(spec)
  if (fromSpec && packaging) return fromSpec
  if (packaging) {
    // 规格读不出：按剂型换成更贴切的最小单位（注射→支、输液→瓶、颗粒→袋…），否则保留盒。
    const d = defaultUnitFor(form, category)
    return d || cur || '盒'
  }
  return cur
}
