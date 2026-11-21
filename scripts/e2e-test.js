#!/usr/bin/env node
// 用药管理 · 端到端测试用例执行器
//
// 用法：node scripts/e2e-test.js [基地址]
// 默认基地址 http://127.0.0.1:18980
//
// 说明：
// - 接口用例（TC-01 ~ TC-65）只用 Node 内置 fetch，无第三方依赖；
// - 界面用例（TC-66 ~ TC-76）需要 playwright，未安装时标记「跳过」而不是失败；
// - 飞牛真机用例（TC-77 ~ TC-81）需人工执行，不在本脚本范围。
// - 2026-11-12 新增 TC-89~TC-127：AI 服务商、识别记录删除、图片管理；
//   其中 TC-117 外部真实 AI 识别需环境变量 E2E_AI_BASE_URL / E2E_AI_API_KEY / E2E_AI_MODEL，
//   未提供时自动跳过；TC-116 用本地 mock OpenAI 兼容服务验证真实调用链路。
// - 建议通过 scripts/e2e-test.sh 运行：它会起一个全新数据目录的临时实例，
//   否则「首个注册用户即管理员」等用例会因历史数据而失败。
//
// 每条用例输出 PASS / FAIL / SKIP，结尾汇总并列出失败明细。

const { execFileSync } = require('node:child_process')
const http = require('node:http')

const BASE = (process.argv[2] || 'http://127.0.0.1:18980').replace(/\/$/, '')

let pass = 0
let fail = 0
let skip = 0
const failures = []

function log(line) { process.stdout.write(line + '\n') }
function ok(id, name) { pass++; log('PASS ' + id + ' ' + name) }
function bad(id, name, detail) {
  fail++
  failures.push(id + ' ' + name + ' — ' + detail)
  log('FAIL ' + id + ' ' + name + ' :: ' + detail)
}
function skipped(id, name, why) { skip++; log('SKIP ' + id + ' ' + name + ' :: ' + why) }

async function req(method, path, { token, body } = {}) {
  const headers = {}
  if (token) headers['Authorization'] = 'Bearer ' + token
  if (body !== undefined) headers['Content-Type'] = 'application/json'
  const res = await fetch(BASE + path, {
    method,
    headers,
    body: body === undefined ? undefined : JSON.stringify(body),
  })
  let json = null
  try { json = await res.json() } catch (e) { /* 非 JSON 响应 */ }
  return { status: res.status, json }
}

const code = (r) => (r.json && typeof r.json.code === 'number' ? r.json.code : null)
const data = (r) => (r.json && r.json.data !== undefined ? r.json.data : null)

function pngBuffer() {
  return Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==', 'base64')
}

// 直接种入识别记录（临时实例的数据库路径由 e2e-test.sh 通过 E2E_DB 传入）。
function seedPrescriptions(bookId, userId) {
  if (!process.env.E2E_DB) return false
  const py = [
    'import sqlite3,sys,json,datetime',
    'db=sqlite3.connect(sys.argv[1]); bid=int(sys.argv[2]); uid=int(sys.argv[3])',
    'now=datetime.datetime.now(datetime.timezone.utc).isoformat()',
    'rows=[("recognized","测试医院"),("failed","坏掉的医院")]',
    'for status,hosp in rows:',
    '    parsed=json.dumps({"doc_type":"prescription","hospital":hosp,"doctor":"张医生","prescription_date":"2026-11-12","items":[{"name":"测试药片","dose_amount":1,"dose_unit":"片","frequency_type":"daily","times":["08:00"],"duration_days":7,"instructions":"饭后"}]},ensure_ascii=False)',
    '    db.execute("INSERT INTO prescription_imports (book_id,created_by,status,kind,provider_type,model,image_mime,parsed_json,error_code,error_message,created_at,updated_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?)",(bid,uid,status,"prescription","","","image/png",parsed,"","",now,now))',
    'db.commit()',
  ].join('\n')
  execFileSync('python3', ['-c', py, process.env.E2E_DB, String(bookId), String(userId)])
  return true
}

function imageEncryptedHex() {
  if (!process.env.E2E_DB) return ''
  const py = 'import sqlite3,sys;d=sqlite3.connect(sys.argv[1]).execute("select image_encrypted from book_images order by id limit 1").fetchone();print(d[0][:8].hex() if d else "")'
  return execFileSync('python3', ['-c', py, process.env.E2E_DB]).toString().trim()
}

async function uploadImages(bookId, token, title, count = 2, category = 'medicine') {
  const fd = new FormData()
  for (let i = 0; i < count; i++) fd.append('files', new Blob([pngBuffer()], { type: 'image/png' }), 'test-' + i + '.png')
  fd.append('title', title)
  fd.append('category', category)
  fd.append('tags', '降压药,早饭后')
  fd.append('note', '自动化测试备注')
  const res = await fetch(BASE + '/api/medcare/books/' + bookId + '/images', { method: 'POST', headers: { Authorization: 'Bearer ' + token }, body: fd })
  return { status: res.status, json: await res.json() }
}

// —— 北京时间工具（与应用口径一致）——
function bjParts(d = new Date()) {
  const p = new Intl.DateTimeFormat('en-CA', { timeZone: 'Asia/Shanghai', year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', hour12: false }).formatToParts(d)
  const g = (t) => p.find((x) => x.type === t).value
  return { date: g('year') + '-' + g('month') + '-' + g('day'), time: g('hour') + ':' + g('minute') }
}
function bjDate(d) { return bjParts(d).date }
function hhmm(offsetMinutes) {
  const p = bjParts(new Date(Date.now() + offsetMinutes * 60000))
  return p.time
}
function sameDay(offsetMinutes) {
  return bjDate(new Date(Date.now() + offsetMinutes * 60000)) === bjDate()
}

async function main() {
  log('目标实例：' + BASE)
  const ver = await req('GET', '/api/version')
  log('版本：' + JSON.stringify(data(ver)))
  log('')

  // 全新实例判断：注册的第一个账号必须是管理员
  const setup = await req('GET', '/api/auth/setup-required')
  log('是否全新实例：' + JSON.stringify(data(setup)))
  log('')

  // ================= 1. 账号与登录 =================
  const uniq = Date.now().toString().slice(-6)
  const adminName = 'tc_admin_' + uniq
  const reg = await req('POST', '/api/auth/register', { body: { username: adminName, password: 'smoke-Passw0rd' } })
  if (code(reg) === 0 && data(reg) && data(reg).token) ok('TC-01', '首次注册成功')
  else { bad('TC-01', '首次注册成功', JSON.stringify(reg.json)); return }
  let adminToken = data(reg).token
  const adminUser = data(reg).user || {}
  if (adminUser.role === 'admin' || adminUser.role === 'owner') ok('TC-01b', '注册者角色为管理员')
  else bad('TC-01b', '注册者角色为管理员', 'role=' + adminUser.role + '（实例可能不是全新库，请用 scripts/e2e-test.sh 启动）')

  const login = await req('POST', '/api/auth/login', { body: { username: adminName, password: 'smoke-Passw0rd' } })
  if (code(login) === 0 && data(login) && data(login).token) ok('TC-02', '账号密码登录')
  else bad('TC-02', '账号密码登录', JSON.stringify(login.json))

  const badLogin = await req('POST', '/api/auth/login', { body: { username: adminName, password: 'smoke-WrongPwd1' } })
  if (code(badLogin) !== 0) ok('TC-03', '错误密码登录被拒')
  else bad('TC-03', '错误密码登录被拒', '返回成功')

  const me = await req('GET', '/api/auth/me', { token: adminToken })
  const meD = data(me) || {}
  if (code(me) === 0 && 'fnos_bound' in meD && 'has_password' in meD && 'has_security_questions' in meD) ok('TC-04', '获取当前账号信息')
  else bad('TC-04', '获取当前账号信息', JSON.stringify(me.json))

  const pwd = await req('PUT', '/api/auth/password', { token: adminToken, body: { old_password: 'smoke-Passw0rd', new_password: 'smoke-NewPass1' } })
  const login2 = await req('POST', '/api/auth/login', { body: { username: adminName, password: 'smoke-NewPass1' } })
  const login3 = await req('POST', '/api/auth/login', { body: { username: adminName, password: 'smoke-Passw0rd' } })
  if (code(pwd) === 0 && code(login2) === 0 && code(login3) !== 0) ok('TC-05', '修改密码（旧密码失效）')
  else bad('TC-05', '修改密码（旧密码失效）', '改密=' + code(pwd) + ' 新密码登录=' + code(login2) + ' 旧密码登录=' + code(login3))
  // 改密会提升 auth_version，旧令牌失效
  if (code(login2) === 0 && data(login2) && data(login2).token) adminToken = data(login2).token

  const sq = await req('POST', '/api/security/questions', {
    token: adminToken,
    body: { question1: '小学班主任姓什么', answer1: '王', question2: '母亲生日月日', answer2: '0512', question3: '第一辆自行车品牌', answer3: '捷安特' },
  })
  const sqGet = await req('GET', '/api/security/questions', { token: adminToken })
  const sqD = data(sqGet) || {}
  const leak = 'answer1' in sqD || 'answer2' in sqD || 'answer3' in sqD
  if (code(sq) === 0 && sqD.has_questions === true && !leak) ok('TC-06', '设置安全问题（答案不回显）')
  else bad('TC-06', '设置安全问题（答案不回显）', 'set=' + code(sq) + ' get=' + JSON.stringify(sqD))

  const forgot = await req('POST', '/api/security/forgot/username', { body: { username: adminName } })
  const forgotD = data(forgot) || {}
  if (code(forgot) === 0 && forgotD.question1) ok('TC-07', '忘记密码取回问题')
  else bad('TC-07', '忘记密码取回问题', JSON.stringify(forgot.json))

  const wrongReset = await req('POST', '/api/security/forgot/reset', {
    body: { username: adminName, answer1: '错', answer2: '0512', answer3: '捷安特', new_password: 'smoke-ResetA1' },
  })
  const afterWrong = await req('POST', '/api/auth/login', { body: { username: adminName, password: 'smoke-ResetA1' } })
  if (code(wrongReset) !== 0 && code(afterWrong) !== 0) ok('TC-09', '错误答案重置被拒')
  else bad('TC-09', '错误答案重置被拒', 'reset=' + code(wrongReset) + ' 可登录=' + code(afterWrong))

  const reset = await req('POST', '/api/security/forgot/reset', {
    body: { username: adminName, answer1: '王', answer2: '0512', answer3: '捷安特', new_password: 'smoke-ResetB1' },
  })
  const afterReset = await req('POST', '/api/auth/login', { body: { username: adminName, password: 'smoke-ResetB1' } })
  if (code(reset) === 0 && code(afterReset) === 0) ok('TC-08', '通过安全问题重置密码')
  else bad('TC-08', '通过安全问题重置密码', 'reset=' + code(reset) + ' 登录=' + code(afterReset))
  if (code(afterReset) === 0 && data(afterReset) && data(afterReset).token) adminToken = data(afterReset).token

  const noAuth = await req('GET', '/api/medcare/books')
  if (noAuth.status === 401) ok('TC-10', '未登录访问受保护接口')
  else bad('TC-10', '未登录访问受保护接口', '状态 ' + noAuth.status)

  // ================= 2. 用药本与成员 =================
  const book = await req('POST', '/api/medcare/books', {
    token: adminToken,
    body: { name: '测试用药本 ' + uniq, care_recipient_name: '测试用药人', timezone: 'Asia/Shanghai' },
  })
  const bookD = data(book) || {}
  if (code(book) === 0 && bookD.id) ok('TC-11', '创建用药本')
  else { bad('TC-11', '创建用药本', JSON.stringify(book.json)); return }
  const bookId = bookD.id

  const books = await req('GET', '/api/medcare/books', { token: adminToken })
  if (code(books) === 0 && (data(books) || []).some((b) => b.id === bookId)) ok('TC-12', '用药本列表')
  else bad('TC-12', '用药本列表', JSON.stringify(books.json))

  const invite = await req('POST', '/api/medcare/books/' + bookId + '/invites', {
    token: adminToken, body: { role: 'member', expires_hours: 24, max_uses: 5 },
  })
  const inviteD = data(invite) || {}
  if (code(invite) === 0 && inviteD.code && inviteD.token) ok('TC-13', '生成邀请码')
  else { bad('TC-13', '生成邀请码', JSON.stringify(invite.json)); return }
  const inviteCode = inviteD.code
  const inviteToken = inviteD.token

  const memberName = 'tc_member_' + uniq
  const reg2 = await req('POST', '/api/auth/register', { body: { username: memberName, password: 'smoke-Passw0rd' } })
  const memberToken = data(reg2).token
  const join = await req('POST', '/api/medcare/invites/join-by-code', { token: memberToken, body: { code: inviteCode } })
  if (code(join) === 0) ok('TC-14', '第二账号用邀请码加入')
  else bad('TC-14', '第二账号用邀请码加入', JSON.stringify(join.json))

  const members = await req('GET', '/api/medcare/books/' + bookId + '/members', { token: adminToken })
  const memberRows = data(members) || []
  if (code(members) === 0 && memberRows.length >= 2) ok('TC-15', '成员列表')
  else bad('TC-15', '成员列表', JSON.stringify(members.json))

  const memberWrite = await req('POST', '/api/medcare/books/' + bookId + '/medicines', {
    token: memberToken, body: { name: '成员越权药品', unit: '片' },
  })
  if (memberWrite.status === 403) ok('TC-16', '成员无写权限')
  else bad('TC-16', '成员无写权限', '状态 ' + memberWrite.status)

  const outsiderName = 'tc_out_' + uniq
  const reg3 = await req('POST', '/api/auth/register', { body: { username: outsiderName, password: 'smoke-Passw0rd' } })
  const outsiderToken = data(reg3).token
  const outsiderRead = await req('GET', '/api/medcare/books/' + bookId + '/medicines', { token: outsiderToken })
  if (outsiderRead.status === 404) ok('TC-17', '非成员访问返回 404')
  else bad('TC-17', '非成员访问返回 404', '状态 ' + outsiderRead.status)

  const joinAgain = await req('POST', '/api/medcare/invites/join-by-code', { token: memberToken, body: { code: inviteCode } })
  const members2 = await req('GET', '/api/medcare/books/' + bookId + '/members', { token: adminToken })
  if ((data(members2) || []).length === memberRows.length) ok('TC-18', '重复加入幂等')
  else bad('TC-18', '重复加入幂等', '成员数 ' + memberRows.length + ' → ' + (data(members2) || []).length + '，返回 ' + code(joinAgain))

  const preview = await req('GET', '/api/medcare/invites/' + inviteToken, { token: adminToken })
  if (code(preview) === 0) ok('TC-19', '邀请码预览')
  else bad('TC-19', '邀请码预览', JSON.stringify(preview.json))

  const revokeInvite = await req('POST', '/api/medcare/books/' + bookId + '/invites', {
    token: adminToken, body: { role: 'member', expires_hours: 24, max_uses: 1 },
  })
  const revokeId = (data(revokeInvite) || {}).id
  const revoke = await req('DELETE', '/api/medcare/books/' + bookId + '/invites/' + revokeId, { token: adminToken })
  const reg4 = await req('POST', '/api/auth/register', { body: { username: 'tc_revoke_' + uniq, password: 'smoke-Passw0rd' } })
  const joinRevoked = await req('POST', '/api/medcare/invites/join-by-code', { token: data(reg4).token, body: { code: (data(revokeInvite) || {}).code } })
  if (code(revoke) === 0 && code(joinRevoked) !== 0) ok('TC-20', '撤销邀请后无法加入')
  else bad('TC-20', '撤销邀请后无法加入', 'revoke=' + code(revoke) + ' join=' + code(joinRevoked))

  // ================= 3. 药品与库存 =================
  const today = bjDate()
  const med = await req('POST', '/api/medcare/books/' + bookId + '/medicines', {
    token: adminToken,
    body: { name: '阿莫西林胶囊', category: 'oral', form: '胶囊', spec: '0.25g×24粒', unit: '粒', low_stock_threshold: 10, expiry_warn_days: 30 },
  })
  const medD = data(med) || {}
  if (code(med) === 0 && medD.id) ok('TC-21', '新建药品')
  else { bad('TC-21', '新建药品', JSON.stringify(med.json)); return }
  const medId = medD.id

  const medList = await req('GET', '/api/medcare/books/' + bookId + '/medicines', { token: adminToken })
  const medGet = await req('GET', '/api/medcare/books/' + bookId + '/medicines/' + medId, { token: adminToken })
  if (code(medList) === 0 && code(medGet) === 0 && (data(medGet) || {}).id === medId) ok('TC-22', '药品列表与详情')
  else bad('TC-22', '药品列表与详情', JSON.stringify(medGet.json))

  const stockIn = await req('POST', '/api/medcare/books/' + bookId + '/medicines/' + medId + '/stock-in', {
    token: adminToken,
    body: { quantity: 40, batch_no: 'B20261101', expiry_date: '2027-06-30', channel: 'pharmacy', supplier: '测试药房' },
  })
  const stockQty = (data(stockIn) || {}).medicine ? data(stockIn).medicine.stock_quantity : null
  if (code(stockIn) === 0 && stockQty === 40) ok('TC-23', '入库（含来源与批次）')
  else bad('TC-23', '入库（含来源与批次）', 'code=' + code(stockIn) + ' 库存=' + stockQty)

  const batches = await req('GET', '/api/medcare/books/' + bookId + '/medicines/' + medId + '/batches', { token: adminToken })
  if (code(batches) === 0 && (data(batches) || []).some((b) => b.batch_no === 'B20261101')) ok('TC-24', '批次列表')
  else bad('TC-24', '批次列表', JSON.stringify(batches.json))

  const movements = await req('GET', '/api/medcare/books/' + bookId + '/medicines/' + medId + '/stock-movements', { token: adminToken })
  const mvRows = (data(movements) || {}).items || []
  const inRow = mvRows.find((m) => m.type === 'in')
  if (code(movements) === 0 && inRow && inRow.channel === 'pharmacy') ok('TC-25', '出入库明细含入库来源')
  else bad('TC-25', '出入库明细含入库来源', JSON.stringify(movements.json))

  const take = await req('POST', '/api/medcare/books/' + bookId + '/medicines/' + medId + '/stocktake', {
    token: adminToken, body: { target_quantity: 38, reason: '月度盘点' },
  })
  const medAfterTake = await req('GET', '/api/medcare/books/' + bookId + '/medicines/' + medId, { token: adminToken })
  if (code(take) === 0 && (data(medAfterTake) || {}).stock_quantity === 38) ok('TC-26', '库存盘点')
  else bad('TC-26', '库存盘点', 'code=' + code(take) + ' 库存=' + ((data(medAfterTake) || {}).stock_quantity))

  await req('POST', '/api/medcare/books/' + bookId + '/medicines/' + medId + '/stocktake', {
    token: adminToken, body: { target_quantity: 5, reason: '模拟用完' },
  })
  const alerts = await req('GET', '/api/medcare/books/' + bookId + '/stock-alerts', { token: adminToken })
  const lowList = (data(alerts) || {}).low_stock || []
  if (code(alerts) === 0 && lowList.some((m) => m.id === medId)) ok('TC-27', '低库存提醒')
  else bad('TC-27', '低库存提醒', JSON.stringify(alerts.json))

  const archive = await req('DELETE', '/api/medcare/books/' + bookId + '/medicines/' + medId, { token: adminToken })
  const restore = await req('POST', '/api/medcare/books/' + bookId + '/medicines/' + medId + '/restore', { token: adminToken })
  if (code(archive) === 0 && code(restore) === 0) ok('TC-28', '药品归档与恢复')
  else bad('TC-28', '药品归档与恢复', 'archive=' + code(archive) + ' restore=' + code(restore))

  const medUpdate = await req('PUT', '/api/medcare/books/' + bookId + '/medicines/' + medId, {
    token: adminToken,
    body: { name: '阿莫西林胶囊（改）', category: 'oral', unit: '粒', spec: '0.25g×24粒', low_stock_threshold: 10, expiry_warn_days: 30 },
  })
  const medAfterUpdate = await req('GET', '/api/medcare/books/' + bookId + '/medicines/' + medId, { token: adminToken })
  if (code(medUpdate) === 0 && (data(medAfterUpdate) || {}).stock_quantity === 5) ok('TC-29', '药品基础信息与库存分离')
  else bad('TC-29', '药品基础信息与库存分离', 'code=' + code(medUpdate) + ' 库存=' + ((data(medAfterUpdate) || {}).stock_quantity))

  // ================= 4. 地点库 =================
  const loc = await req('POST', '/api/medcare/books/' + bookId + '/locations', {
    token: adminToken, body: { name: '社区诊所', type: 'clinic', address: '幸福路 12 号', contact_name: '李医生', contact_phone: '13800000000' },
  })
  const locD = data(loc) || {}
  if (code(loc) === 0 && locD.id) ok('TC-57', '新建地点')
  else { bad('TC-57', '新建地点', JSON.stringify(loc.json)); return }
  const locId = locD.id

  const homeLoc = await req('POST', '/api/medcare/books/' + bookId + '/locations', {
    token: adminToken, body: { name: '家里', type: 'home', address: '客厅' },
  })
  const homeLocId = (data(homeLoc) || {}).id

  const locList = await req('GET', '/api/medcare/books/' + bookId + '/locations', { token: adminToken })
  const locUpdate = await req('PUT', '/api/medcare/books/' + bookId + '/locations/' + locId, {
    token: adminToken, body: { name: '社区诊所（改）', type: 'clinic', address: '幸福路 12 号' },
  })
  if (code(locList) === 0 && code(locUpdate) === 0) ok('TC-58', '地点列表与更新')
  else bad('TC-58', '地点列表与更新', JSON.stringify(locUpdate.json))

  const locArchive = await req('DELETE', '/api/medcare/books/' + bookId + '/locations/' + locId, { token: adminToken })
  const locRestore = await req('POST', '/api/medcare/books/' + bookId + '/locations/' + locId + '/restore', { token: adminToken })
  if (code(locArchive) === 0 && code(locRestore) === 0) ok('TC-59', '地点归档与恢复')
  else bad('TC-59', '地点归档与恢复', 'archive=' + code(locArchive) + ' restore=' + code(locRestore))

  // ================= 5. 用药计划 =================
  // 时间槽：一个刚过去的（用于漏服/补记）、一个未来的（待完成）
  const pastTime = sameDay(-15) ? hhmm(-15) : hhmm(-15)
  const futureTime = hhmm(30)
  const plan = await req('POST', '/api/medcare/books/' + bookId + '/plans', {
    token: adminToken,
    body: {
      medicine_id: medId, title: '每日两次', type: 'oral', dose_amount: 1, dose_unit: '粒',
      frequency: { type: 'daily', times: [pastTime, futureTime] }, start_date: today, timezone: 'Asia/Shanghai',
    },
  })
  const planD = (data(plan) || {}).plan
  if (code(plan) === 0 && planD && planD.id) ok('TC-30', '新建每日计划')
  else { bad('TC-30', '新建每日计划', JSON.stringify(plan.json)); return }
  const planId = planD.id

  const planList = await req('GET', '/api/medcare/books/' + bookId + '/plans', { token: adminToken })
  const planGet = await req('GET', '/api/medcare/books/' + bookId + '/plans/' + planId, { token: adminToken })
  const planDetail = (data(planGet) || {}).plan
  if (code(planList) === 0 && planDetail && planDetail.id === planId && planDetail.frequency) ok('TC-31', '计划列表与详情')
  else bad('TC-31', '计划列表与详情', JSON.stringify(planGet.json))

  const planPreview = await req('GET', '/api/medcare/books/' + bookId + '/plans/' + planId + '/preview?days=7', { token: adminToken })
  const pv = data(planPreview) || {}
  if (code(planPreview) === 0 && Array.isArray(pv.times) && pv.times.length > 0 && pv.from && pv.to) ok('TC-32', '频率预览')
  else bad('TC-32', '频率预览', JSON.stringify(planPreview.json))

  const pause = await req('POST', '/api/medcare/books/' + bookId + '/plans/' + planId + '/pause', { token: adminToken })
  const pausedPlan = ((data(await req('GET', '/api/medcare/books/' + bookId + '/plans/' + planId, { token: adminToken }))) || {}).plan
  const resume = await req('POST', '/api/medcare/books/' + bookId + '/plans/' + planId + '/resume', { token: adminToken })
  const resumedPlan = ((data(await req('GET', '/api/medcare/books/' + bookId + '/plans/' + planId, { token: adminToken }))) || {}).plan
  if (code(pause) === 0 && pausedPlan && pausedPlan.status === 'paused' && code(resume) === 0 && resumedPlan && resumedPlan.status === 'active') ok('TC-33', '暂停与恢复计划')
  else bad('TC-33', '暂停与恢复计划', 'pause=' + code(pause) + ' 状态=' + (pausedPlan && pausedPlan.status) + ' resume=' + code(resume) + ' 状态=' + (resumedPlan && resumedPlan.status))

  const endPlan = await req('POST', '/api/medcare/books/' + bookId + '/plans/' + planId + '/end', { token: adminToken })
  if (code(endPlan) === 0) ok('TC-33b', '结束计划')
  else bad('TC-33b', '结束计划', JSON.stringify(endPlan.json))

  // 改计划只影响未来：新建一个计划后改剂量，历史事件不变
  const plan2 = await req('POST', '/api/medcare/books/' + bookId + '/plans', {
    token: adminToken,
    body: {
      medicine_id: medId, title: '改剂量验证', type: 'oral', dose_amount: 1, dose_unit: '粒',
      frequency: { type: 'daily', times: [futureTime] }, start_date: today, timezone: 'Asia/Shanghai',
    },
  })
  const plan2Id = ((data(plan2) || {}).plan || {}).id
  const evBefore = await req('GET', '/api/medcare/books/' + bookId + '/dose-events?plan_id=' + plan2Id, { token: adminToken })
  const evBeforeRows = (data(evBefore) || {}).items || []
  await req('PUT', '/api/medcare/books/' + bookId + '/plans/' + plan2Id, {
    token: adminToken,
    body: {
      medicine_id: medId, title: '改剂量验证', type: 'oral', dose_amount: 2, dose_unit: '粒',
      frequency: { type: 'daily', times: [futureTime] }, start_date: today, timezone: 'Asia/Shanghai',
    },
  })
  const evAfter = await req('GET', '/api/medcare/books/' + bookId + '/dose-events?plan_id=' + plan2Id, { token: adminToken })
  const evAfterRows = (data(evAfter) || {}).items || []
  const histSame = evBeforeRows.filter((e) => e.status !== 'pending').every((e) => {
    const after = evAfterRows.find((x) => x.id === e.id)
    return after && after.dose_amount === e.dose_amount
  })
  if (evBeforeRows.length > 0 && histSame) ok('TC-34', '修改计划只影响未来未完成事件')
  else bad('TC-34', '修改计划只影响未来未完成事件', '改前 ' + evBeforeRows.length + ' 条，改后 ' + evAfterRows.length + ' 条')

  // 注射计划
  const injMed = await req('POST', '/api/medcare/books/' + bookId + '/medicines', {
    token: adminToken, body: { name: '头孢曲松钠注射液', category: 'injection', form: '注射液', spec: '1g', unit: '瓶' },
  })
  const injMedId = (data(injMed) || {}).id
  await req('POST', '/api/medcare/books/' + bookId + '/medicines/' + injMedId + '/stock-in', {
    token: adminToken, body: { quantity: 10, channel: 'hospital' },
  })
  const injPlan = await req('POST', '/api/medcare/books/' + bookId + '/plans', {
    token: adminToken,
    body: {
      medicine_id: injMedId, title: '诊所打针', type: 'injection', dose_amount: 1, dose_unit: '瓶',
      frequency: { type: 'daily', times: [futureTime] }, start_date: today, end_date: today,
      location_id: locId, executor_type: 'doctor', executor_name: '社区诊所李医生',
    },
  })
  const injPlanD = (data(injPlan) || {}).plan
  if (code(injPlan) === 0 && injPlanD && injPlanD.executor_type === 'doctor' && injPlanD.location_name) ok('TC-35', '注射计划带地点与执行人')
  else bad('TC-35', '注射计划带地点与执行人', JSON.stringify(injPlan.json))

  // 输液计划
  const infMed = await req('POST', '/api/medcare/books/' + bookId + '/medicines', {
    token: adminToken, body: { name: '生理盐水 100ml', category: 'infusion', form: '注射液', spec: '100ml', unit: '袋' },
  })
  const infMedId = (data(infMed) || {}).id
  await req('POST', '/api/medcare/books/' + bookId + '/medicines/' + infMedId + '/stock-in', {
    token: adminToken, body: { quantity: 10, channel: 'hospital' },
  })
  const infPlan = await req('POST', '/api/medcare/books/' + bookId + '/plans', {
    token: adminToken,
    body: {
      medicine_id: infMedId, title: '居家输液', type: 'infusion', dose_amount: 1, dose_unit: '袋',
      frequency: { type: 'daily', times: [futureTime] }, start_date: today, end_date: today,
      location_id: homeLocId, executor_type: 'family', executor_name: '家属小张', drip_rate: 60, duration_minutes: 90,
    },
  })
  const infPlanD = (data(infPlan) || {}).plan
  if (code(infPlan) === 0 && infPlanD && infPlanD.drip_rate === 60 && infPlanD.duration_minutes === 90) ok('TC-36', '输液计划带滴速与时长')
  else bad('TC-36', '输液计划带滴速与时长', JSON.stringify(infPlan.json))

  // （药品转移用例放在统计之后执行，避免把诊所事件搬走影响地点统计）

  // ================= 6. 今日事件 =================
  const dash = await req('GET', '/api/medcare/books/' + bookId + '/dashboard?date=' + today, { token: adminToken })
  const dashD = data(dash) || {}
  const allEvents = []
  if (dashD.groups) for (const g of dashD.groups) allEvents.push(...(g.events || []))
  if (code(dash) === 0 && allEvents.length > 0 && dashD.summary) ok('TC-38', '今日看板')
  else bad('TC-38', '今日看板', JSON.stringify(dash.json))

  const infEvent = allEvents.find((e) => e.type === 'infusion' && e.status === 'pending')
  const pendingOral = allEvents.find((e) => e.type === 'oral' && e.status === 'pending')
  const missedOral = allEvents.find((e) => e.type === 'oral' && e.status !== 'pending' && e.status !== 'done')
  log('  （事件：输液 ' + (infEvent ? infEvent.id : '无') + '，待完成口服 ' + (pendingOral ? pendingOral.id : '无') + '，过期口服 ' + (missedOral ? missedOral.id : '无') + '）')

  if (infEvent) {
    const complete = await req('POST', '/api/medcare/books/' + bookId + '/dose-events/' + infEvent.id + '/complete', {
      token: adminToken, body: { version: infEvent.version, deduct_stock: true },
    })
    const completeD = data(complete) || {}
    const deducted = completeD.stock ? completeD.stock.deducted : null
    const infMedAfter = await req('GET', '/api/medcare/books/' + bookId + '/medicines/' + infMedId, { token: adminToken })
    if (code(complete) === 0 && completeD.event && completeD.event.status === 'done' && deducted === 1 && (data(infMedAfter) || {}).stock_quantity === 9) ok('TC-39', '完成用药扣库存')
    else bad('TC-39', '完成用药扣库存', 'code=' + code(complete) + ' 扣减=' + deducted + ' 库存=' + ((data(infMedAfter) || {}).stock_quantity))

    const completeAgain = await req('POST', '/api/medcare/books/' + bookId + '/dose-events/' + infEvent.id + '/complete', {
      token: adminToken, body: { version: infEvent.version, deduct_stock: true },
    })
    const infMedAfter2 = await req('GET', '/api/medcare/books/' + bookId + '/medicines/' + infMedId, { token: adminToken })
    if ((data(infMedAfter2) || {}).stock_quantity === 9) ok('TC-40', '重复完成幂等（不重复扣库存）')
    else bad('TC-40', '重复完成幂等（不重复扣库存）', '库存 9 → ' + ((data(infMedAfter2) || {}).stock_quantity) + '，返回 ' + code(completeAgain))

    const undo = await req('POST', '/api/medcare/books/' + bookId + '/dose-events/' + infEvent.id + '/undo', { token: adminToken })
    const infMedAfter3 = await req('GET', '/api/medcare/books/' + bookId + '/medicines/' + infMedId, { token: adminToken })
    if (code(undo) === 0 && (data(infMedAfter3) || {}).stock_quantity === 10) ok('TC-41', '撤销完成恢复库存')
    else bad('TC-41', '撤销完成恢复库存', 'code=' + code(undo) + ' 库存=' + ((data(infMedAfter3) || {}).stock_quantity))
  } else bad('TC-39', '完成用药扣库存', '今天没有待完成的输液事件（时间槽未落在今天？）')

  if (infEvent) {
    const start = await req('POST', '/api/medcare/books/' + bookId + '/dose-events/' + infEvent.id + '/start', { token: adminToken })
    const startD = data(start) || {}
    const startAgain = await req('POST', '/api/medcare/books/' + bookId + '/dose-events/' + infEvent.id + '/start', { token: adminToken })
    if (code(start) === 0 && startD.actual_start_at && (data(startAgain) || {}).actual_start_at === startD.actual_start_at) ok('TC-47', '输液记录实际开始时间（幂等）')
    else bad('TC-47', '输液记录实际开始时间（幂等）', 'start=' + code(start) + ' again=' + code(startAgain))

    const startWrong = await req('POST', '/api/medcare/books/' + bookId + '/dose-events/' + (pendingOral ? pendingOral.id : infEvent.id) + '/start', { token: adminToken })
    if (pendingOral && code(startWrong) !== 0) ok('TC-49', '非输液事件不允许记录开始时间')
    else bad('TC-49', '非输液事件不允许记录开始时间', '返回 ' + code(startWrong))

    const completeInf = await req('POST', '/api/medcare/books/' + bookId + '/dose-events/' + infEvent.id + '/complete', {
      token: adminToken, body: { version: (data(startAgain) || startD || {}).version, deduct_stock: true },
    })
    const evDone = (data(completeInf) || {}).event
    if (code(completeInf) === 0 && evDone && evDone.actual_start_at && evDone.actual_end_at) ok('TC-48', '输液完成记录结束时间')
    else bad('TC-48', '输液完成记录结束时间', JSON.stringify(completeInf.json))
  }

  if (pendingOral) {
    const snooze = await req('POST', '/api/medcare/books/' + bookId + '/dose-events/' + pendingOral.id + '/snooze', { token: adminToken, body: { minutes: 30 } })
    if (code(snooze) === 0 && (data(snooze) || {}).snooze_until) ok('TC-43', '稍后提醒')
    else bad('TC-43', '稍后提醒', JSON.stringify(snooze.json))
  } else bad('TC-43', '稍后提醒', '没有待完成事件')

  const missTarget = missedOral || pendingOral
  if (missTarget) {
    const miss = await req('POST', '/api/medcare/books/' + bookId + '/dose-events/' + missTarget.id + '/miss', { token: adminToken, body: {} })
    const missData = data(miss) || {}
    const missEvent = missData.event || missData
    if (code(miss) === 0 && missEvent && missEvent.status === 'missed') ok('TC-42', '标记漏服')
    else bad('TC-42', '标记漏服', JSON.stringify(miss.json))
  } else bad('TC-42', '标记漏服', '没有可用事件')

  // 调整/备注/地点覆盖使用「改剂量验证」计划的事件，避免与上面的漏服事件冲突
  const plan2Events = (data(await req('GET', '/api/medcare/books/' + bookId + '/dose-events?plan_id=' + plan2Id, { token: adminToken })) || {}).items || []
  const adjustTarget = plan2Events.find((e) => e.status === 'pending') || null
  if (adjustTarget) {
    const adjust = await req('POST', '/api/medcare/books/' + bookId + '/dose-events/' + adjustTarget.id + '/adjust', {
      token: adminToken, body: { adjustment_type: 'doctor', scope: 'event', note: '复诊减量' },
    })
    if (code(adjust) === 0) ok('TC-44', '医生调整（仅本次）')
    else bad('TC-44', '医生调整（仅本次）', JSON.stringify(adjust.json))

    const note = await req('POST', '/api/medcare/books/' + bookId + '/dose-events/' + adjustTarget.id + '/note', { token: adminToken, body: { note: '饭后服用' } })
    if (code(note) === 0) ok('TC-45a', '事件备注')
    else bad('TC-45a', '事件备注', JSON.stringify(note.json))

    const evLocation = await req('POST', '/api/medcare/books/' + bookId + '/dose-events/' + adjustTarget.id + '/location', {
      token: adminToken, body: { location_id: locId, location_name: '社区诊所', address: '幸福路 12 号' },
    })
    if (code(evLocation) === 0) ok('TC-45b', '单次地点覆盖')
    else bad('TC-45b', '单次地点覆盖', JSON.stringify(evLocation.json))
  } else {
    skipped('TC-44', '医生调整（仅本次）', '缺少可用的待完成事件')
    skipped('TC-45a', '事件备注', '缺少可用的待完成事件')
    skipped('TC-45b', '单次地点覆盖', '缺少可用的待完成事件')
  }

  const stale = await req('POST', '/api/medcare/books/' + bookId + '/dose-events/' + (pendingOral ? pendingOral.id : infEvent.id) + '/complete', {
    token: adminToken, body: { version: 999999, deduct_stock: false },
  })
  if (code(stale) !== 0) ok('TC-46', '版本冲突保护')
  else bad('TC-46', '版本冲突保护', '返回成功')

  // ================= 7. 统计 =================
  const overview = await req('GET', '/api/medcare/books/' + bookId + '/stats/overview?from=' + today + '&to=' + today, { token: adminToken })
  const ovD = data(overview) || {}
  if (code(overview) === 0 && typeof ovD.completion_rate === 'number' && 'clean_streak_days' in ovD) ok('TC-50', '统计总览')
  else bad('TC-50', '统计总览', JSON.stringify(overview.json))

  const trend = await req('GET', '/api/medcare/books/' + bookId + '/stats/trend?from=' + today + '&to=' + today, { token: adminToken })
  const heat = await req('GET', '/api/medcare/books/' + bookId + '/stats/heatmap?from=' + today + '&to=' + today, { token: adminToken })
  if (code(trend) === 0 && code(heat) === 0) ok('TC-51', '趋势与热力图')
  else bad('TC-51', '趋势与热力图', JSON.stringify(trend.json))

  const breakdown = await req('GET', '/api/medcare/books/' + bookId + '/stats/breakdown?from=' + today + '&to=' + today, { token: adminToken })
  const bdD = data(breakdown) || {}
  if (code(breakdown) === 0 && bdD.by_medicine && bdD.by_type && bdD.by_time && bdD.by_reason) ok('TC-52', '分类统计')
  else bad('TC-52', '分类统计', JSON.stringify(breakdown.json))

  const locTypes = bdD.by_location_type || {}
  if (code(breakdown) === 0 && (locTypes.clinic || 0) > 0 && (locTypes.home || 0) > 0) ok('TC-53', '按地点类型统计')
  else bad('TC-53', '按地点类型统计', JSON.stringify(locTypes))

  const byLoc = bdD.by_location || {}
  if (code(breakdown) === 0 && Object.keys(byLoc).length > 0) ok('TC-54', '按地点统计')
  else bad('TC-54', '按地点统计', JSON.stringify(byLoc))

  const byExec = bdD.by_executor || {}
  const execNames = Object.keys(byExec)
  if (code(breakdown) === 0 && execNames.length > 0 && execNames.some((n) => n.includes('家属小张') || n.includes('社区诊所李医生'))) ok('TC-55', '按执行人统计')
  else bad('TC-55', '按执行人统计', JSON.stringify(byExec))

  if (typeof ovD.total === 'number' && ovD.total >= ovD.done) ok('TC-56', '统计口径（待完成不计入分母）')
  else bad('TC-56', '统计口径（待完成不计入分母）', JSON.stringify(ovD))

  // —— 首次用户体验回归（2026-11-09 走查修复）——
  // TC-83 新建计划跳过今天已过时刻时给出明确返回
  const pastDate = new Date(Date.now() - 3 * 3600000)
  if (bjDate(pastDate) === today) {
    const pastTime = bjParts(pastDate).time
    const skipPlan = await req('POST', '/api/medcare/books/' + bookId + '/plans', {
      token: adminToken,
      body: { medicine_id: medId, title: '跳过时刻验证', type: 'oral', dose_amount: 1, dose_unit: '粒', frequency: { type: 'daily', times: [pastTime] }, start_date: today, end_date: today },
    })
    const skippedTimes = (data(skipPlan) || {}).skipped_today_times || []
    if (code(skipPlan) === 0 && skippedTimes.includes(pastTime)) ok('TC-83', '新建计划返回今天被跳过的时刻')
    else bad('TC-83', '新建计划返回今天被跳过的时刻', JSON.stringify(skipPlan.json))
  } else skipped('TC-83', '新建计划返回今天被跳过的时刻', '当前北京时间凌晨，3 小时前属于昨天，无法构造')

  // TC-84 今日无事件但已有计划：看板给出计划数与下一项
  const yesterday = bjDate(new Date(Date.now() - 24 * 3600000))
  const dashYesterday = await req('GET', '/api/medcare/books/' + bookId + '/dashboard?date=' + yesterday, { token: adminToken })
  const dyD = data(dashYesterday) || {}
  if (code(dashYesterday) === 0 && (dyD.summary || {}).total === 0 && (dyD.active_plan_count || 0) > 0 && dyD.next_upcoming) ok('TC-84', '无事件日看板返回计划数与下一项')
  else bad('TC-84', '无事件日看板返回计划数与下一项', JSON.stringify(dashYesterday.json))

  // TC-85 连续无漏服只统计有用药记录的天（新用药本完成 1 次 = 1 天）
  const streakBook = await req('POST', '/api/medcare/books', { token: adminToken, body: { name: '连续天数验证 ' + uniq, care_recipient_name: '验证用药人' } })
  const streakBookId = (data(streakBook) || {}).id
  const sMed = await req('POST', '/api/medcare/books/' + streakBookId + '/medicines', { token: adminToken, body: { name: '验证药片', category: 'oral', unit: '片' } })
  const sMedId = (data(sMed) || {}).id
  await req('POST', '/api/medcare/books/' + streakBookId + '/medicines/' + sMedId + '/stock-in', { token: adminToken, body: { quantity: 10 } })
  // 「当前时间 +40 分钟」在深夜会跨到第二天，那样计划时间就成了今天已过的 00:0x，
  // 事件会被判成漏服，用例就会在每天 23:20 之后必失败。跨天时改用第二天作为计划日期。
  const sTime = hhmm(40)
  const sDate = sameDay(40) ? today : bjDate(new Date(Date.now() + 40 * 60000))
  await req('POST', '/api/medcare/books/' + streakBookId + '/plans', {
    token: adminToken,
    body: { medicine_id: sMedId, title: '验证计划', type: 'oral', dose_amount: 1, dose_unit: '片', frequency: { type: 'daily', times: [sTime] }, start_date: sDate, end_date: sDate },
  })
  const sDash = await req('GET', '/api/medcare/books/' + streakBookId + '/dashboard?date=' + sDate, { token: adminToken })
  const sEv = ((data(sDash) || {}).groups || []).flatMap((g) => g.events || []).find((e) => e.status === 'pending')
  if (sEv) await req('POST', '/api/medcare/books/' + streakBookId + '/dose-events/' + sEv.id + '/complete', { token: adminToken, body: { version: sEv.version, deduct_stock: true } })
  const sOverview = await req('GET', '/api/medcare/books/' + streakBookId + '/stats/overview?from=' + sDate + '&to=' + sDate, { token: adminToken })
  const sOv = data(sOverview) || {}
  if (code(sOverview) === 0 && sOv.clean_streak_days === 1) ok('TC-85', '连续无漏服天数只算有记录的天')
  else bad('TC-85', '连续无漏服天数只算有记录的天', 'clean_streak_days=' + sOv.clean_streak_days)

  // TC-86 地点/执行人统计只计注射与输液（口服即使覆盖了地点也不计入）
  const shotCount = allEvents.filter((e) => e.type === 'injection' || e.type === 'infusion').length
  const locSum = Object.values(bdD.by_location || {}).reduce((a, b) => a + b, 0)
  const noOralExecutor = !('未指定执行人' in (bdD.by_executor || {}))
  if (locSum === shotCount && noOralExecutor) ok('TC-86', '地点与执行人统计只计注射/输液')
  else bad('TC-86', '地点与执行人统计只计注射/输液', '地点合计 ' + locSum + '，注射输液事件 ' + shotCount + '，执行人 ' + JSON.stringify(bdD.by_executor))

  // 药品转移（连带计划与事件）
  const book2 = await req('POST', '/api/medcare/books', {
    token: adminToken, body: { name: '转移目标本 ' + uniq, care_recipient_name: '目标用药人' },
  })
  const book2Id = (data(book2) || {}).id
  const transferMed = await req('POST', '/api/medcare/books/' + bookId + '/medicines/' + injMedId + '/transfer', {
    token: adminToken, body: { target_book_id: book2Id },
  })
  const injPlanInBook2 = await req('GET', '/api/medcare/books/' + book2Id + '/plans', { token: adminToken })
  const book2Plans = (data(injPlanInBook2) || {}).items || []
  if (code(transferMed) === 0 && book2Plans.some((p) => p.id === injPlanD.id)) ok('TC-37', '药品转移连带计划到目标用药本')
  else bad('TC-37', '药品转移连带计划到目标用药本', 'transfer=' + code(transferMed) + ' 目标本计划 ' + book2Plans.length + ' 条')

  const transferPlanFail = await req('POST', '/api/medcare/books/' + bookId + '/plans/' + planId + '/transfer', {
    token: adminToken, body: { target_book_id: book2Id },
  })
  if (code(transferPlanFail) !== 0) ok('TC-37b', '计划转移在目标本缺少药品时给出明确提示')
  else bad('TC-37b', '计划转移在目标本缺少药品时给出明确提示', '返回成功')


  // ================= 8. 通知与偏好 =================
  const channels = await req('GET', '/api/medcare/channels', { token: adminToken })
  if (code(channels) === 0 && Array.isArray(data(channels))) ok('TC-60', '通知渠道列表')
  else bad('TC-60', '通知渠道列表', JSON.stringify(channels.json))

  const notifications = await req('GET', '/api/medcare/notifications', { token: adminToken })
  const unread = await req('GET', '/api/medcare/notifications/unread-count', { token: adminToken })
  if (code(notifications) === 0 && code(unread) === 0) ok('TC-61', '通知中心')
  else bad('TC-61', '通知中心', JSON.stringify(unread.json))

  const prefSet = await req('PUT', '/api/medcare/preferences', {
    token: adminToken,
    body: { lead_minutes: 15, overdue_first_minutes: 30, overdue_repeat_minutes: 5, overdue_max_repeats: 5, daily_summary_time: '21:00', daily_summary_enabled: true, low_stock_notify_enabled: true, theme_mode: 'system', theme_brand: 'a' },
  })
  const prefD = data(await req('GET', '/api/medcare/preferences', { token: adminToken })) || {}
  if (code(prefSet) === 0 && prefD.overdue_repeat_minutes === 5 && prefD.lead_minutes === 15) ok('TC-62', '偏好设置读写')
  else bad('TC-62', '偏好设置读写', JSON.stringify(prefD))

  const prefRepeat = await req('PUT', '/api/medcare/preferences', {
    token: adminToken, body: { overdue_repeat_minutes: 60, lead_minutes: 15, overdue_first_minutes: 30, daily_summary_time: '21:00' },
  })
  const prefD3 = data(await req('GET', '/api/medcare/preferences', { token: adminToken })) || {}
  if (code(prefRepeat) === 0 && prefD3.overdue_repeat_minutes === 60) ok('TC-63', '未完成重复提醒间隔可配置')
  else bad('TC-63', '未完成重复提醒间隔可配置', JSON.stringify(prefD3))

  // ================= 9. 操作日志 =================
  const logs = await req('GET', '/api/medcare/books/' + bookId + '/operation-logs?pageSize=100', { token: adminToken })
  const logRows = (data(logs) || {}).items || []
  if (code(logs) === 0 && logRows.some((l) => l.action === '完成用药')) ok('TC-64', '操作日志记录')
  else bad('TC-64', '操作日志记录', '共 ' + logRows.length + ' 条，动作样例 ' + JSON.stringify(logRows.slice(0, 3).map((l) => l.action)))

  const sensitiveLog = logRows.find((l) => (l.detail || '').includes('含敏感字段'))
  if (!sensitiveLog) {
    await req('PUT', '/api/medcare/books/' + bookId + '/medicines/' + medId, {
      token: adminToken, body: { name: '阿莫西林胶囊（改）', unit: '粒', password: 'smoke-masked' },
    })
    const logs2 = await req('GET', '/api/medcare/books/' + bookId + '/operation-logs?pageSize=100', { token: adminToken })
    const rows2 = (data(logs2) || {}).items || []
    if (rows2.some((l) => (l.detail || '').includes('含敏感字段'))) ok('TC-65', '敏感字段脱敏')
    else bad('TC-65', '敏感字段脱敏', '未找到脱敏记录')
  } else ok('TC-65', '敏感字段脱敏')

  // ================= 11. 2026-11-12 新增功能：AI 服务商 / 识别删除 / 图片管理 =================
  // TC-89~TC-94：AI 服务商配置（接口层持久化）
  const aiPut = await req('PUT', '/api/medcare/admin/ai', {
    token: adminToken,
    body: { enabled: false, provider_type: 'mimo', base_url: 'https://api.xiaomimimo.com/v1', model: 'mimo-v2.6-flash', timeout_seconds: 120 },
  })
  const aiGet = await req('GET', '/api/medcare/admin/ai', { token: adminToken })
  const aiCfg = data(aiGet) || {}
  if (code(aiPut) === 0 && aiCfg.provider_type === 'mimo' && aiCfg.base_url === 'https://api.xiaomimimo.com/v1' && aiCfg.model === 'mimo-v2.6-flash') ok('TC-93', 'AI 服务商选择持久化')
  else bad('TC-93', 'AI 服务商选择持久化', JSON.stringify(aiCfg))

  // TC-116：真实识别调用链路（本地 mock OpenAI 兼容服务，验证请求/解析/轮询/确认全链路）
  let mockServer = null
  try {
    mockServer = http.createServer((req, res) => {
      let body = ''
      req.on('data', (c) => (body += c))
      req.on('end', () => {
        const payload = {
          doc_type: 'prescription', hospital: '测试医院', doctor: '张医生', prescription_date: '2026-11-12',
          items: [{ raw_name: '阿莫西林胶囊', name: '阿莫西林胶囊', spec: '0.25g×24粒', form: '胶囊', dose_amount: 1, dose_unit: '粒', frequency_type: 'daily', times: ['08:00', '20:00'], duration_days: 7, instructions: '饭后服用', confidence: 0.98 }],
        }
        res.writeHead(200, { 'Content-Type': 'application/json' })
        res.end(JSON.stringify({ choices: [{ message: { content: '\u0060\u0060\u0060json\n' + JSON.stringify(payload) + '\n\u0060\u0060\u0060' } }] }))
      })
    })
    await new Promise((resolve) => mockServer.listen(0, '127.0.0.1', resolve))
    const mport = mockServer.address().port
    await req('PUT', '/api/medcare/admin/ai', {
      token: adminToken,
      body: { enabled: true, provider_type: 'mock', base_url: 'http://127.0.0.1:' + mport + '/v1', model: 'mock-vision', api_key: 'smoke-sk-mock', timeout_seconds: 60 },
    })
    const fd = new FormData()
    fd.append('file', new Blob([pngBuffer()], { type: 'image/png' }), 'prescription.png')
    fd.append('save_image', 'false')
    const recRes = await fetch(BASE + '/api/medcare/books/' + bookId + '/prescriptions/recognize', { method: 'POST', headers: { Authorization: 'Bearer ' + adminToken }, body: fd })
    const recJson = await recRes.json()
    const recId = recJson?.data?.import_id
    let recognized = null
    for (let i = 0; i < 25 && recId; i++) {
      await new Promise((r) => setTimeout(r, 400))
      const g = await req('GET', '/api/medcare/books/' + bookId + '/prescriptions/' + recId, { token: adminToken })
      const gd = data(g) || {}
      if (gd.status === 'recognized') { recognized = gd.draft; break }
      if (gd.status === 'failed') break
    }
    if (recJson?.code === 0 && recognized && (recognized.items || []).length === 1 && recognized.items[0].name === '阿莫西林胶囊') ok('TC-116', 'AI 识别调用链路（mock 服务）')
    else bad('TC-116', 'AI 识别调用链路（mock 服务）', JSON.stringify(recJson).slice(0, 200) + ' draft=' + JSON.stringify(recognized).slice(0, 200))
    if (recId) await req('DELETE', '/api/medcare/books/' + bookId + '/prescriptions/' + recId, { token: adminToken })
    await req('PUT', '/api/medcare/admin/ai', {
      token: adminToken,
      body: { enabled: false, provider_type: 'mimo', base_url: 'https://api.xiaomimimo.com/v1', model: 'mimo-v2.6-flash', timeout_seconds: 120 },
    })
  } catch (e) {
    bad('TC-116', 'AI 识别调用链路（mock 服务）', e.message)
  } finally {
    if (mockServer) { try { mockServer.close() } catch {} }
  }

  // TC-117：外部真实 AI（需要 E2E_AI_BASE_URL / E2E_AI_API_KEY / E2E_AI_MODEL）
  if (process.env.E2E_AI_BASE_URL && process.env.E2E_AI_API_KEY) {
    try {
      await req('PUT', '/api/medcare/admin/ai', {
        token: adminToken,
        body: { enabled: true, provider_type: 'custom', base_url: process.env.E2E_AI_BASE_URL, model: process.env.E2E_AI_MODEL || 'gpt-4o-mini', api_key: process.env.E2E_AI_API_KEY, timeout_seconds: 120 },
      })
      const sample = require('node:fs').readFileSync(require('node:path').join(__dirname, 'testdata', 'prescription-sample.png'))
      const fd = new FormData()
      fd.append('file', new Blob([sample], { type: 'image/png' }), 'prescription-sample.png')
      const r = await fetch(BASE + '/api/medcare/books/' + bookId + '/prescriptions/recognize', { method: 'POST', headers: { Authorization: 'Bearer ' + adminToken }, body: fd })
      const rj = await r.json()
      const rid = rj?.data?.import_id
      let draft = null
      for (let i = 0; i < 90 && rid; i++) {
        await new Promise((x) => setTimeout(x, 1000))
        const g = await req('GET', '/api/medcare/books/' + bookId + '/prescriptions/' + rid, { token: adminToken })
        const gd = data(g) || {}
        if (gd.status === 'recognized') { draft = gd.draft; break }
        if (gd.status === 'failed') break
      }
      if (draft && (draft.items || []).length > 0) ok('TC-117', '外部真实 AI 识别（' + (process.env.E2E_AI_MODEL || '') + '）')
      else bad('TC-117', '外部真实 AI 识别', '未返回可用草稿')
      if (rid) await req('DELETE', '/api/medcare/books/' + bookId + '/prescriptions/' + rid, { token: adminToken })
    } catch (e) {
      bad('TC-117', '外部真实 AI 识别', e.message)
    }
  } else {
    skipped('TC-117', '外部真实 AI 识别', '未提供 E2E_AI_BASE_URL / E2E_AI_API_KEY')
  }

  // TC-95~TC-99：识别记录删除
  const seeded = seedPrescriptions(bookId, adminUser.id)
  if (seeded) {
    const h1 = await req('GET', '/api/medcare/books/' + bookId + '/prescriptions', { token: adminToken })
    const hRows = data(h1) || []
    if (code(h1) === 0 && hRows.length === 2) ok('TC-95', '识别历史含待确认与失败记录')
    else bad('TC-95', '识别历史含待确认与失败记录', '共 ' + hRows.length + ' 条')
    const failed = hRows.find((x) => x.status === 'failed')
    const recognized = hRows.find((x) => x.status === 'recognized')
    if (failed) {
      await req('DELETE', '/api/medcare/books/' + bookId + '/prescriptions/' + failed.id, { token: adminToken })
    }
    const h2 = await req('GET', '/api/medcare/books/' + bookId + '/prescriptions', { token: adminToken })
    if ((data(h2) || []).length === 1) ok('TC-97', '删除失败识别记录')
    else bad('TC-97', '删除失败识别记录', '剩余 ' + (data(h2) || []).length)

    if (recognized) {
      const beforeMed = await req('GET', '/api/medcare/books/' + bookId + '/medicines?pageSize=200', { token: adminToken })
      const beforeTotal = (data(beforeMed) || {}).total || 0
      const conf = await req('POST', '/api/medcare/books/' + bookId + '/prescriptions/' + recognized.id + '/confirm', {
        token: adminToken,
        body: { start_date: '2026-11-12', hospital: '测试医院', doctor: '张医生', channel: 'hospital', items: [{ raw_name: '测试药片', name: '测试药片', dose_amount: 1, dose_unit: '片', frequency_type: 'daily', times: ['08:00'], duration_days: 7, instructions: '饭后', create_plan: true, medicine_mode: 'new' }] },
      })
      const afterMed = await req('GET', '/api/medcare/books/' + bookId + '/medicines?pageSize=200', { token: adminToken })
      const afterTotal = (data(afterMed) || {}).total || 0
      await req('DELETE', '/api/medcare/books/' + bookId + '/prescriptions/' + recognized.id, { token: adminToken })
      const afterMed2 = await req('GET', '/api/medcare/books/' + bookId + '/medicines?pageSize=200', { token: adminToken })
      const afterTotal2 = (data(afterMed2) || {}).total || 0
      if (code(conf) === 0 && afterTotal === beforeTotal + 1 && afterTotal2 === afterTotal) ok('TC-98', '删除已录入识别记录不影响药品数据')
      else bad('TC-98', '删除已录入识别记录不影响药品数据', JSON.stringify({ conf: code(conf), beforeTotal, afterTotal, afterTotal2 }))
    }
    const h3 = await req('GET', '/api/medcare/books/' + bookId + '/prescriptions', { token: adminToken })
    if ((data(h3) || []).length === 0) ok('TC-99', '已删除识别记录不再列出')
    else bad('TC-99', '已删除识别记录不再列出', '剩余 ' + (data(h3) || []).length)
  } else {
    skipped('TC-95~99', '识别记录删除', '未设置 E2E_DB（请通过 e2e-test.sh 运行）')
  }

  // TC-100~TC-109：图片管理接口
  const up = await uploadImages(bookId, adminToken, '自动化测试图片', 2)
  const upItems = (up.json && up.json.data && up.json.data.items) || []
  if (up.status === 200 && up.json.code === 0 && upItems.length === 2 && upItems[0].title === '自动化测试图片 1' && upItems[1].tags.includes('降压药')) ok('TC-100', '多图上传与标题序号')
  else bad('TC-100', '多图上传与标题序号', JSON.stringify(up.json).slice(0, 250))
  const imageId = upItems[0] && upItems[0].id
  const catMed = await req('GET', '/api/medcare/books/' + bookId + '/images?category=medicine', { token: adminToken })
  const catRx = await req('GET', '/api/medcare/books/' + bookId + '/images?category=prescription', { token: adminToken })
  const searchImg = await req('GET', '/api/medcare/books/' + bookId + '/images?q=降压药', { token: adminToken })
  if ((data(catMed) || {}).total === 2 && (data(catRx) || {}).total === 0 && (data(searchImg) || {}).total === 2) ok('TC-102/103', '图片分类筛选与搜索')
  else bad('TC-102/103', '图片分类筛选与搜索', JSON.stringify([(data(catMed) || {}).total, (data(catRx) || {}).total, (data(searchImg) || {}).total]))
  const updImg = await req('PUT', '/api/medcare/books/' + bookId + '/images/' + imageId, { token: adminToken, body: { title: '自动化测试图片（改）', category: 'treatment', tags: ['标签A', '标签B'], note: '已改' } })
  const updImgD = data(updImg) || {}
  if (code(updImg) === 0 && updImgD.title === '自动化测试图片（改）' && updImgD.category === 'treatment' && (updImgD.tags || []).length === 2) ok('TC-106', '编辑图片标题/分类/标签/备注')
  else bad('TC-106', '编辑图片标题/分类/标签/备注', JSON.stringify(updImgD))
  const fileRes = await fetch(BASE + '/api/medcare/books/' + bookId + '/images/' + imageId + '/file', { headers: { Authorization: 'Bearer ' + adminToken } })
  const fileBuf = Buffer.from(await fileRes.arrayBuffer())
  if (fileRes.status === 200 && fileRes.headers.get('content-type') === 'image/png' && fileBuf.equals(pngBuffer())) ok('TC-104', '图片预览接口')
  else bad('TC-104', '图片预览接口', fileRes.status + ' ' + fileRes.headers.get('content-type') + ' ' + fileBuf.length)
  const dlRes = await fetch(BASE + '/api/medcare/books/' + bookId + '/images/' + imageId + '/file?download=1', { headers: { Authorization: 'Bearer ' + adminToken } })
  const cdHeader = dlRes.headers.get('content-disposition') || ''
  if (dlRes.status === 200 && cdHeader.startsWith('attachment') && cdHeader.includes("filename*=UTF-8''")) ok('TC-105', '图片下载响应头')
  else bad('TC-105', '图片下载响应头', cdHeader)
  const encHex = imageEncryptedHex()
  if (encHex && encHex !== '89504e47' && encHex !== 'ffd8ffe0') ok('TC-109', '图片加密存储')
  else bad('TC-109', '图片加密存储', encHex || '无 E2E_DB')
  const memberList = await req('GET', '/api/medcare/books/' + bookId + '/images', { token: memberToken })
  const memberUpload = await uploadImages(bookId, memberToken, '越权上传', 1)
  const outsiderList = await req('GET', '/api/medcare/books/' + bookId + '/images', { token: outsiderToken })
  if (code(memberList) === 0 && memberUpload.status === 403 && outsiderList.status === 404) ok('TC-108', '图片权限隔离')
  else bad('TC-108', '图片权限隔离', JSON.stringify([memberList.status, memberUpload.status, outsiderList.status]))
  await req('DELETE', '/api/medcare/books/' + bookId + '/images/' + imageId, { token: adminToken })
  const afterDel = await req('GET', '/api/medcare/books/' + bookId + '/images', { token: adminToken })
  if ((data(afterDel) || {}).total === 1) ok('TC-107', '删除图片')
  else bad('TC-107', '删除图片', '剩余 ' + (data(afterDel) || {}).total)

  // TC-153~155：上传压缩与缩略图
  try {
    const fs = require('node:fs')
    const path = require('node:path')
    const rawLarge = fs.readFileSync(path.join(__dirname, 'testdata', 'large-label.jpg'))
    const fdLarge = new FormData()
    fdLarge.append('files', new Blob([rawLarge], { type: 'image/jpeg' }), 'large-label.jpg')
    fdLarge.append('title', '大图压缩测试')
    fdLarge.append('category', 'medicine')
    const upLarge = await fetch(BASE + '/api/medcare/books/' + bookId + '/images', { method: 'POST', headers: { Authorization: 'Bearer ' + adminToken }, body: fdLarge })
    const upLargeJson = await upLarge.json()
    const largeItem = upLargeJson?.data?.items?.[0]
    if (largeItem && largeItem.size < rawLarge.length * 0.95) ok('TC-153', '上传图片压缩（' + rawLarge.length + '→' + largeItem.size + ' 字节）')
    else bad('TC-153', '上传图片压缩', '原 ' + rawLarge.length + ' 存 ' + (largeItem && largeItem.size))
    if (largeItem) {
      const thumbRes = await fetch(BASE + '/api/medcare/books/' + bookId + '/images/' + largeItem.id + '/file?thumb=1', { headers: { Authorization: 'Bearer ' + adminToken } })
      const thumbBuf = Buffer.from(await thumbRes.arrayBuffer())
      if (thumbRes.status === 200 && thumbRes.headers.get('content-type') === 'image/jpeg' && thumbBuf.length > 0 && thumbBuf.length < largeItem.size) ok('TC-154', '缩略图接口（' + thumbBuf.length + ' 字节）')
      else bad('TC-154', '缩略图接口', thumbRes.status + ' ' + thumbRes.headers.get('content-type') + ' ' + thumbBuf.length)
      if (process.env.E2E_DB) {
        const py = 'import sqlite3,sys;d=sqlite3.connect(sys.argv[1]);d.execute("update book_images set thumb_encrypted=NULL, thumb_mime=\'\', thumb_size=0 where id=?",(int(sys.argv[2]),));d.commit()'
        execFileSync('python3', ['-c', py, process.env.E2E_DB, String(largeItem.id)])
        const lazyRes = await fetch(BASE + '/api/medcare/books/' + bookId + '/images/' + largeItem.id + '/file?thumb=1', { headers: { Authorization: 'Bearer ' + adminToken } })
        const lazyBuf = Buffer.from(await lazyRes.arrayBuffer())
        if (lazyRes.status === 200 && lazyRes.headers.get('content-type') === 'image/jpeg' && lazyBuf.length > 0) ok('TC-155', '旧图片按需生成缩略图')
        else bad('TC-155', '旧图片按需生成缩略图', lazyRes.status + ' ' + lazyBuf.length)
      } else skipped('TC-155', '旧图片按需生成缩略图', '未设置 E2E_DB')
      await req('DELETE', '/api/medcare/books/' + bookId + '/images/' + largeItem.id, { token: adminToken })
    }
  } catch (e) { bad('TC-153~155', '上传压缩与缩略图', e.message) }

  // ================= 10. 界面用例 =================
  let chromium = null
  try { chromium = require('playwright').chromium } catch (e) { chromium = null }
  if (!chromium) {
    const uiCases = [['TC-66', '登录页可用'], ['TC-67', '首次使用注册页'], ['TC-68', '忘记密码页'], ['TC-69', '我的页账户信息'], ['TC-70', '我的页安全问题'], ['TC-71', '赞赏弹窗（仅管理员）'], ['TC-72', '侧边栏高亮唯一'], ['TC-73', '手机端无横向滚动'], ['TC-74', '无原生弹窗'], ['TC-75', '输液卡片信息完整'], ['TC-76', '统计页地点与执行人区块'], ['TC-82', '引导页不弹赞赏'], ['TC-87', '非今日日期头部文案'], ['TC-88', '未设置阈值文案']]
    for (const [id, name] of uiCases) skipped(id, name, '未安装 playwright')
  } else {
    try {
      const browser = await chromium.launch()
      const ctx = await browser.newContext({ viewport: { width: 390, height: 844 }, deviceScaleFactor: 2, isMobile: true, hasTouch: true })
      const page = await ctx.newPage()
      let nativeDialog = false
      page.on('dialog', async (d) => { nativeDialog = true; await d.dismiss() })
      await page.addInitScript((t) => { try { localStorage.setItem('medcare.token', t) } catch (e) {} }, adminToken)

      // TC-82 全新账号（没有用药本）进入引导页：不应有赞赏弹窗与横幅
      const freshName = 'tc_ui_fresh_' + uniq
      const freshReg = await req('POST', '/api/auth/register', { body: { username: freshName, password: 'smoke-Passw0rd' } })
      const freshCtx = await browser.newContext({ viewport: { width: 390, height: 844 }, deviceScaleFactor: 2, isMobile: true, hasTouch: true })
      const freshPage = await freshCtx.newPage()
      await freshPage.addInitScript((t) => { try { localStorage.setItem('medcare.token', t) } catch (e) {} }, data(freshReg).token)
      await freshPage.goto(BASE + '/', { waitUntil: 'networkidle' })
      await freshPage.waitForTimeout(2600)
      const freshModal = await freshPage.locator('.support-card').count()
      const freshBanner = await freshPage.locator('.support-banner').count()
      if (freshPage.url().includes('/books') && freshModal === 0 && freshBanner === 0) ok('TC-82', '引导页（无用药本）不弹赞赏、不显示横幅')
      else bad('TC-82', '引导页（无用药本）不弹赞赏、不显示横幅', 'url=' + freshPage.url() + ' 弹窗=' + freshModal + ' 横幅=' + freshBanner)
      await freshCtx.close()

      // 登录页：另开一个不带令牌的上下文，否则会被路由守卫直接跳到今日页
      const anonCtx = await browser.newContext({ viewport: { width: 390, height: 844 }, deviceScaleFactor: 2, isMobile: true, hasTouch: true })
      const anonPage = await anonCtx.newPage()
      await anonPage.goto(BASE + '/login', { waitUntil: 'domcontentloaded' })
      await anonPage.waitForTimeout(700)
      const loginInputs = await anonPage.locator('input').count()
      if (loginInputs >= 2) ok('TC-66', '登录页可用')
      else bad('TC-66', '登录页可用', '输入框数量 ' + loginInputs + '，当前地址 ' + anonPage.url())

      const forgotLink = await anonPage.getByText('忘记密码？').count()
      if (forgotLink > 0) ok('TC-67', '登录页含忘记密码入口')
      else bad('TC-67', '登录页含忘记密码入口', '未找到入口')
      await anonCtx.close()

      await page.goto(BASE + '/forgot-password', { waitUntil: 'domcontentloaded' })
      await page.waitForTimeout(400)
      const forgotInput = await page.locator('input').count()
      if (forgotInput >= 1) ok('TC-68', '忘记密码页渲染')
      else bad('TC-68', '忘记密码页渲染', '无输入框')

      await page.goto(BASE + '/profile', { waitUntil: 'networkidle' })
      await page.waitForTimeout(800)
      const infoRows = await page.locator('.info-row').count()
      const sqText = await page.locator('text=安全问题').count()
      if (infoRows >= 4) ok('TC-69', '我的页账户信息')
      else bad('TC-69', '我的页账户信息', '信息行 ' + infoRows)
      if (sqText > 0) ok('TC-70', '我的页安全问题区块')
      else bad('TC-70', '我的页安全问题区块', '未找到')

      await page.goto(BASE + '/today', { waitUntil: 'networkidle' })
      await page.waitForTimeout(2500)
      const supportModal = await page.locator('.support-card h3').count()
      if (supportModal > 0) ok('TC-71', '赞赏弹窗（管理员）')
      else bad('TC-71', '赞赏弹窗（管理员）', '未出现弹窗')
      const close = page.locator('.support-close')
      if (await close.count()) await close.click()
      await page.waitForTimeout(300)

      // TC-88 未设置低库存阈值时文案不带多余单位
      const noThresholdMed = await req('POST', '/api/medcare/books/' + bookId + '/medicines', { token: adminToken, body: { name: '无阈值验证药', category: 'oral', unit: '片' } })
      const ntId = (data(noThresholdMed) || {}).id
      await page.goto(BASE + '/medicines/' + ntId, { waitUntil: 'networkidle' })
      await page.waitForTimeout(700)
      await page.getByRole('button', { name: '库存', exact: true }).click()
      await page.waitForTimeout(500)
      const ntText = await page.locator('body').innerText()
      if (ntText.includes('低库存阈值 未设置') && !ntText.includes('未设置 片')) ok('TC-88', '未设置阈值文案不带多余单位')
      else bad('TC-88', '未设置阈值文案不带多余单位', ntText.match(/低库存阈值[^\n]*/)?.[0] || '未找到')

      const activeNavs = await page.locator('.sidebar .nav-link.router-link-active').allTextContents()
      if (activeNavs.length <= 1) ok('TC-72', '侧边栏高亮唯一')
      else bad('TC-72', '侧边栏高亮唯一', JSON.stringify(activeNavs))

      const pages = ['/today', '/plans', '/medicines', '/stats', '/profile', '/locations']
      let overflow = ''
      for (const p of pages) {
        await page.goto(BASE + p, { waitUntil: 'networkidle' })
        await page.waitForTimeout(500)
        const m = await page.evaluate(() => ({ sw: document.documentElement.scrollWidth, cw: document.documentElement.clientWidth }))
        if (m.sw > m.cw + 1) overflow += p + '(' + m.sw + '>' + m.cw + ') '
      }
      if (!overflow) ok('TC-73', '手机端无横向滚动')
      else bad('TC-73', '手机端无横向滚动', overflow)

      if (!nativeDialog) ok('TC-74', '无原生弹窗')
      else bad('TC-74', '无原生弹窗', '触发了原生 dialog')

      await page.goto(BASE + '/today', { waitUntil: 'networkidle' })
      await page.waitForTimeout(900)
      const dripText = await page.locator('text=滴速').count()
      const executorText = await page.locator('text=家属小张').count()
      if (dripText > 0 && executorText > 0) ok('TC-75', '输液卡片信息完整（地点/执行人/滴速）')
      else bad('TC-75', '输液卡片信息完整（地点/执行人/滴速）', '滴速 ' + dripText + ' 执行人 ' + executorText)

      // TC-87 选择非今日日期时头部显示「当天」
      await page.goto(BASE + '/today', { waitUntil: 'networkidle' })
      await page.waitForTimeout(700)
      const tomorrowDay = Number(bjDate(new Date(Date.now() + 24 * 3600000)).slice(8))
      const dayChip = page.locator('.date-strip button').filter({ hasText: String(tomorrowDay) }).first()
      if (await dayChip.count()) {
        await dayChip.click()
        await page.waitForTimeout(800)
        const headText = await page.locator('.page-head').innerText()
        if (headText.includes('当天有')) ok('TC-87', '非今日日期头部显示「当天」')
        else bad('TC-87', '非今日日期头部显示「当天」', headText.replace(/\n+/g, ' | '))
      } else skipped('TC-87', '非今日日期头部显示「当天」', '日期条上未找到明天的按钮')

      await page.goto(BASE + '/stats', { waitUntil: 'networkidle' })
      await page.waitForTimeout(900)
      const byLocText = await page.locator('text=按地点').count()
      const byExecText = await page.locator('text=按执行人').count()
      if (byLocText > 0 && byExecText > 0) ok('TC-76', '统计页地点与执行人区块')
      else bad('TC-76', '统计页地点与执行人区块', '地点 ' + byLocText + ' 执行人 ' + byExecText)

      // ---- 2026-11-12 新增功能 UI ----
      await page.goto(BASE + '/settings', { waitUntil: 'networkidle' })
      await page.waitForTimeout(1200)
      const scAI = page.locator('.support-close'); if (await scAI.count()) await scAI.click()
      await page.waitForTimeout(300)
      const aiCard = page.locator('.card').filter({ hasText: 'AI 处方识别' }).first()
      const providerSelect = aiCard.locator('button.select-like').first()
      let optionTexts = []
      if (await providerSelect.count()) {
        await providerSelect.click()
        await page.waitForTimeout(500)
        optionTexts = await page.locator('.sheet button').allTextContents()
        const hasCore = ['OpenAI 官方', 'DeepSeek 深度求索', '通义千问（阿里云百炼）', '小米 MiMo', '自定义 / 未列出（手动填写）'].every((x) => optionTexts.includes(x))
        if (hasCore) ok('TC-118', 'AI 服务商下拉界面完整')
        else bad('TC-118', 'AI 服务商下拉界面完整', JSON.stringify(optionTexts.slice(0, 8)))
        const mimoOpt = page.locator('.sheet button').filter({ hasText: '小米 MiMo' })
        if (await mimoOpt.count()) { await mimoOpt.first().click(); await page.waitForTimeout(400) }
      } else bad('TC-118', 'AI 服务商下拉界面完整', '未找到下拉')
      const aiIns = aiCard.locator('input.input')
      const aiVals = []
      for (let i = 0; i < await aiIns.count(); i++) aiVals.push(await aiIns.nth(i).inputValue())
      if (aiVals[0] === 'https://api.xiaomimimo.com/v1' && aiVals[1] === 'mimo-v2.6-flash') ok('TC-119', '选择小米 MiMo 自动填地址与模型')
      else bad('TC-119', '选择小米 MiMo 自动填地址与模型', JSON.stringify(aiVals))
      await aiCard.getByRole('button', { name: /保存/ }).first().click()
      await page.waitForTimeout(900)
      await page.reload({ waitUntil: 'networkidle' })
      await page.waitForTimeout(1000)
      const scAI2 = page.locator('.support-close'); if (await scAI2.count()) await scAI2.click()
      await page.waitForTimeout(300)
      const aiCard2 = page.locator('.card').filter({ hasText: 'AI 处方识别' }).first()
      const aiLabel = await aiCard2.locator('button.select-like').first().innerText()
      const aiVals2 = []
      const aiIns2 = aiCard2.locator('input.input')
      for (let i = 0; i < await aiIns2.count(); i++) aiVals2.push(await aiIns2.nth(i).inputValue())
      if (aiLabel.includes('小米 MiMo') && aiVals2[0] === 'https://api.xiaomimimo.com/v1') ok('TC-120', 'AI 服务商保存后刷新回显')
      else bad('TC-120', 'AI 服务商保存后刷新回显', aiLabel + ' / ' + JSON.stringify(aiVals2))

      // 识别历史删除（重新种一条待确认记录）
      if (seedPrescriptions(bookId, adminUser.id)) {
        await page.goto(BASE + '/prescriptions', { waitUntil: 'networkidle' })
        await page.waitForTimeout(1200)
        const scP = page.locator('.support-close'); if (await scP.count()) await scP.click()
        await page.waitForTimeout(300)
        const hp = await page.locator('.history-card').count()
        const delBtns = await page.locator('.history-del').count()
        if (hp >= 1 && delBtns >= 1) ok('TC-121', '识别历史显示删除按钮')
        else bad('TC-121', '识别历史显示删除按钮', hp + '/' + delBtns)
        await page.locator('.history-del').first().click()
        await page.waitForTimeout(500)
        await page.locator('.modal .btn-danger').click()
        await page.waitForTimeout(1000)
        const hp2 = await page.locator('.history-card').count()
        if (hp2 === hp - 1) ok('TC-122', '界面删除识别记录')
        else bad('TC-122', '界面删除识别记录', hp + '->' + hp2)
      } else {
        skipped('TC-121', '界面删除识别记录', '未设置 E2E_DB')
        skipped('TC-122', '界面删除识别记录', '未设置 E2E_DB')
      }

      // 图片管理 UI
      await page.goto(BASE + '/images', { waitUntil: 'networkidle' })
      await page.waitForTimeout(1800)
      const scImg = page.locator('.support-close'); if (await scImg.count()) await scImg.click()
      await page.waitForTimeout(300)
      const cardCount0 = await page.locator('.img-card').count()
      const thumbCount = await page.locator('.img-card img').count()
      if (cardCount0 >= 1 && thumbCount >= 1) ok('TC-123', '图片管理列表与缩略图')
      else bad('TC-123', '图片管理列表与缩略图', cardCount0 + '/' + thumbCount)
      if (cardCount0 >= 1) {
        await page.locator('.img-card').first().click()
        await page.waitForTimeout(900)
        const previewOk = (await page.locator('.preview-img').count()) > 0
        if (previewOk) ok('TC-124', '图片大图预览')
        else bad('TC-124', '图片大图预览', '无预览图')
        await page.locator('.sheet input.input').first().fill('界面自动化改名')
        await page.getByRole('button', { name: '保存', exact: true }).click()
        await page.waitForTimeout(900)
        await page.locator('.sheet-head .icon-btn').click()
        await page.waitForTimeout(400)
        const t0 = await page.locator('.img-title').first().innerText()
        if (t0 === '界面自动化改名') ok('TC-125', '界面编辑图片标题')
        else bad('TC-125', '界面编辑图片标题', t0)
      } else {
        skipped('TC-124', '图片大图预览', '列表为空')
        skipped('TC-125', '界面编辑图片标题', '列表为空')
      }
      await page.getByRole('button', { name: /上传图片/ }).first().click()
      await page.waitForTimeout(600)
      await page.locator('input[type=file]').setInputFiles({ name: 'ui.png', mimeType: 'image/png', buffer: pngBuffer() })
      await page.waitForTimeout(300)
      await page.locator('.sheet input.input').first().fill('界面上传')
      await page.getByRole('button', { name: '上传', exact: true }).click()
      await page.waitForTimeout(1800)
      const cardCount1 = await page.locator('.img-card').count()
      if (cardCount1 === cardCount0 + 1) ok('TC-126', '界面上传图片')
      else bad('TC-126', '界面上传图片', cardCount0 + '->' + cardCount1)
      const of = await page.evaluate(() => ({ sw: document.documentElement.scrollWidth, cw: document.documentElement.clientWidth }))
      if (of.sw <= of.cw + 1) ok('TC-127', '图片页手机端无横向滚动')
      else bad('TC-127', '图片页手机端无横向滚动', JSON.stringify(of))

      // 计划两种查看方式（按时间 / 按药名）
      await page.goto(BASE + '/plans', { waitUntil: 'networkidle' })
      await page.waitForTimeout(1200)
      const scPlan = page.locator('.support-close'); if (await scPlan.count()) await scPlan.click()
      await page.waitForTimeout(300)
      const modeBtns = await page.locator('.mode-btn').count()
      if (modeBtns === 2) ok('TC-133', '计划页两种查看方式入口')
      else bad('TC-133', '计划页两种查看方式入口', '按钮数 ' + modeBtns)
      await page.locator('.view-tabs button').filter({ hasText: '按月' }).click()
      await page.waitForTimeout(1000)
      const monthCellCount = await page.locator('.month-cell:not(.blank)').count()
      const monthGroupCount = await page.locator('.month-group').count()
      if (monthCellCount >= 28) ok('TC-134', '计划按月视图')
      else bad('TC-134', '计划按月视图', '天数 ' + monthCellCount)
      await page.locator('.mode-btn').filter({ hasText: '按药名' }).click()
      await page.waitForTimeout(900)
      const medGroups = await page.locator('.medicine-group').count()
      if (medGroups >= 1 || (await page.locator('.empty').count()) > 0) ok('TC-135', '计划按药名视图')
      else bad('TC-135', '计划按药名视图', '药名分组 ' + medGroups)
      const planOverflow = await page.evaluate(() => ({ sw: document.documentElement.scrollWidth, cw: document.documentElement.clientWidth }))
      if (planOverflow.sw <= planOverflow.cw + 1) ok('TC-136', '计划页手机端无横向滚动')
      else bad('TC-136', '计划页手机端无横向滚动', JSON.stringify(planOverflow))

      // 图片管理多维度分组（全部用药本 / 按用药人 / 按日期）
      let groupBookId = 0
      try {
        const gb = await req('POST', '/api/medcare/books', { token: adminToken, body: { name: '分组测试本', care_recipient_name: '分组用药人' } })
        groupBookId = data(gb)?.id || 0
        if (groupBookId) {
          await uploadImages(groupBookId, adminToken, '第二本图片', 1)
          if (process.env.E2E_DB) {
            const py = 'import sqlite3,sys,datetime;d=sqlite3.connect(sys.argv[1]);y=(datetime.datetime.now(datetime.timezone.utc)-datetime.timedelta(days=1)).isoformat();d.execute("update book_images set created_at=? where book_id=? and id=(select min(id) from book_images where book_id=?)",(y,int(sys.argv[2]),int(sys.argv[2])));d.commit()'
            execFileSync('python3', ['-c', py, process.env.E2E_DB, String(groupBookId)])
          }
        }
      } catch (e) { groupBookId = 0 }
      await page.goto(BASE + '/images', { waitUntil: 'networkidle' })
      await page.waitForTimeout(1400)
      const scGroup = page.locator('.support-close'); if (await scGroup.count()) await scGroup.click()
      await page.waitForTimeout(300)
      const allScope = page.locator('.chip').filter({ hasText: '全部用药本' })
      if (groupBookId && (await allScope.count())) {
        await allScope.first().click()
        await page.waitForTimeout(1200)
        const personGroups = await page.locator('.group-head').count()
        const allCards = await page.locator('.img-card').count()
        if (personGroups >= 2 && allCards >= 2) ok('TC-148', '图片管理全部用药本与按用药人分组')
        else bad('TC-148', '图片管理全部用药本与按用药人分组', '组 ' + personGroups + ' 张 ' + allCards)
        await page.locator('.chip').filter({ hasText: '按日期' }).click()
        await page.waitForTimeout(600)
        const dateGroups = await page.locator('.group-head').count()
        if (dateGroups >= 2) ok('TC-149', '图片管理按日期分组')
        else bad('TC-149', '图片管理按日期分组', '日期组 ' + dateGroups)
        const uploadBtnInAll = await page.getByRole('button', { name: /上传图片/ }).count()
        if (uploadBtnInAll === 0) ok('TC-150', '全部用药本范围隐藏上传按钮')
        else bad('TC-150', '全部用药本范围隐藏上传按钮', '按钮 ' + uploadBtnInAll)
        await page.locator('.img-card').first().click()
        await page.waitForTimeout(900)
        if (await page.locator('.preview-img').count()) ok('TC-151', '全部范围图片预览')
        else bad('TC-151', '全部范围图片预览', '无预览图')
        await page.locator('.sheet-head .icon-btn').click()
        await page.waitForTimeout(300)
        const groupOverflow = await page.evaluate(() => ({ sw: document.documentElement.scrollWidth, cw: document.documentElement.clientWidth }))
        if (groupOverflow.sw <= groupOverflow.cw + 1) ok('TC-152', '图片分组页手机端无横向滚动')
        else bad('TC-152', '图片分组页手机端无横向滚动', JSON.stringify(groupOverflow))
      } else {
        skipped('TC-148~152', '图片管理多维度分组', '无法创建第二个用药本或未找到范围切换')
      }

      // 更多菜单
      await page.goto(BASE + '/today', { waitUntil: 'networkidle' })
      await page.waitForTimeout(1000)
      const scMore = page.locator('.support-close'); if (await scMore.count()) await scMore.click()
      await page.waitForTimeout(200)
      const moreTrigger = page.locator('.more-trigger')
      if (await moreTrigger.count()) {
        await moreTrigger.click()
        await page.waitForTimeout(300)
        const moreItems = await page.locator('.more-item').count()
        if (moreItems >= 8) ok('TC-170', '更多菜单展开与常用入口')
        else bad('TC-170', '更多菜单展开与常用入口', '入口数 ' + moreItems)
        const imgEntry = page.locator('.more-item').filter({ hasText: '图片管理' })
        if (await imgEntry.count()) {
          await imgEntry.first().click()
          await page.waitForTimeout(800)
          if (page.url().includes('/images') && (await page.locator('.more-menu').count()) === 0) ok('TC-171', '更多菜单进入图片管理并自动收起')
          else bad('TC-171', '更多菜单进入图片管理并自动收起', page.url())
        } else bad('TC-171', '更多菜单进入图片管理并自动收起', '未找到图片管理入口')
      } else bad('TC-170', '更多菜单展开与常用入口', '未找到更多按钮')

      await browser.close()
    } catch (e) {
      bad('TC-66~171', '界面用例执行', e.message)
    }
  }

  log('')
  log('通过 ' + pass + ' / 失败 ' + fail + ' / 跳过 ' + skip + ' / 总计 ' + (pass + fail + skip))
  if (failures.length) {
    log('')
    log('失败明细：')
    for (const f of failures) log('  - ' + f)
  }
  process.exit(fail > 0 ? 1 : 0)
}

main().catch((e) => { log('执行异常：' + e.stack); process.exit(2) })
