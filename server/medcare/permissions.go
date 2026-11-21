package medcare

import (
    "encoding/json"
    "errors"
    "strings"
)

// 能力键 = 模块:动作（例如 medicine:delete）。管理员在成员管理里逐格勾选，
// 勾了才有、不勾就没有；用药人可能是老人、孕妇、中学生或大学生，
// 实际动手的人各不同：有人只负责看今天吃了没，有人负责买药入库，
// 有人连处方单与检查报告都不该看到，所以「查看」本身也是一格权限。
const (
    ModToday        = "today"
    ModMedicine     = "medicine"
    ModPlan         = "plan"
    ModPrescription = "prescription"
    ModImage        = "image"
    ModStats        = "stats"
    ModMember       = "member"

    ActView   = "view"
    ActCreate = "create"
    ActEdit   = "edit"
    ActDelete = "delete"
    // ActInvite 属于成员模块：改角色/设权限/移除不进能力表，永远只认管理员。
    ActInvite = "invite"
    // 今日这一格特殊：不是 CRUD，而是「点提醒」和「点完成」。
    ActRemind = "remind"
    ActRecord = "record"
)

// 路由与 handler 里直接引用这些常量，避免手拼字符串写错。
const (
    PermTodayView      = ModToday + ":" + ActView
    PermTodayRemind    = ModToday + ":" + ActRemind
    PermTodayRecord    = ModToday + ":" + ActRecord
    PermMedicineView   = ModMedicine + ":" + ActView
    PermMedicineCreate = ModMedicine + ":" + ActCreate
    PermMedicineEdit   = ModMedicine + ":" + ActEdit
    PermMedicineDelete = ModMedicine + ":" + ActDelete
    PermPlanView       = ModPlan + ":" + ActView
    PermPlanCreate     = ModPlan + ":" + ActCreate
    PermPlanEdit       = ModPlan + ":" + ActEdit
    PermPlanDelete     = ModPlan + ":" + ActDelete
    PermPrescView      = ModPrescription + ":" + ActView
    PermPrescCreate    = ModPrescription + ":" + ActCreate
    PermPrescEdit      = ModPrescription + ":" + ActEdit
    PermPrescDelete    = ModPrescription + ":" + ActDelete
    PermImageView      = ModImage + ":" + ActView
    PermImageCreate    = ModImage + ":" + ActCreate
    PermImageEdit      = ModImage + ":" + ActEdit
    PermImageDelete    = ModImage + ":" + ActDelete
    PermStatsView      = ModStats + ":" + ActView
    PermMemberView     = ModMember + ":" + ActView
    PermMemberInvite   = ModMember + ":" + ActInvite
)

// PermissionAction 是矩阵里的一格。
type PermissionAction struct {
    Key   string `json:"key"`
    Act   string `json:"action"`
    Label string `json:"label"`
}

// PermissionModule 是矩阵里的一行；前端按 modules 渲染表格，
// 新增模块或动作只改这张表，判定与文案都不用动别处。
type PermissionModule struct {
    Module  string             `json:"module"`
    Label   string             `json:"label"`
    Hint    string             `json:"hint"`
    Actions []PermissionAction `json:"actions"`
}

func permKey(module, action string) string { return module + ":" + action }

// crud 是标准的「查看/新增/修改/删除」四格。
func crud(module, createLabel, editLabel, deleteLabel string) []PermissionAction {
    return []PermissionAction{
        {Key: permKey(module, ActView), Act: ActView, Label: "查看"},
        {Key: permKey(module, ActCreate), Act: ActCreate, Label: createLabel},
        {Key: permKey(module, ActEdit), Act: ActEdit, Label: editLabel},
        {Key: permKey(module, ActDelete), Act: ActDelete, Label: deleteLabel},
    }
}

// permissionModules 是能力的权威清单，顺序即界面展示顺序与序列化顺序。
func permissionModules() []PermissionModule {
    return []PermissionModule{
        {Module: ModToday, Label: "用药记录", Hint: "今日与日历的用药安排、提醒、打卡与备注", Actions: []PermissionAction{
            {Key: PermTodayView, Act: ActView, Label: "查看"},
            {Key: PermTodayRemind, Act: ActRemind, Label: "点提醒"},
            {Key: PermTodayRecord, Act: ActRecord, Label: "点完成"},
        }},
        {Module: ModMedicine, Label: "药品与库存", Hint: "药品档案、批次、出入库与盘点", Actions: crud(ModMedicine, "新增", "修改", "删除")},
        {Module: ModPlan, Label: "用药计划", Hint: "计划的新建、改期、暂停终止，含地点库维护", Actions: crud(ModPlan, "新建", "修改", "删除")},
        {Module: ModPrescription, Label: "处方识别", Hint: "拍照识别处方/药盒/缴费单与确认入库", Actions: crud(ModPrescription, "识别", "确认录入", "删除")},
        {Module: ModImage, Label: "图片资料", Hint: "药品照片、处方单、治疗单、检查报告", Actions: crud(ModImage, "上传", "编辑", "删除")},
        {Module: ModStats, Label: "统计与日志", Hint: "依从性统计、操作日志、库存流水", Actions: []PermissionAction{
            {Key: PermStatsView, Act: ActView, Label: "查看"},
        }},
        {Module: ModMember, Label: "成员与邀请", Hint: "查看成员列表、邀请家人；改角色与设权限只限管理员", Actions: []PermissionAction{
            {Key: PermMemberView, Act: ActView, Label: "查看"},
            {Key: PermMemberInvite, Act: ActInvite, Label: "邀请"},
        }},
    }
}

// allPermissionKeys 返回全部能力键（按权威清单顺序）。
func allPermissionKeys() []string {
    keys := make([]string, 0, 24)
    for _, module := range permissionModules() {
        for _, action := range module.Actions {
            keys = append(keys, action.Key)
        }
    }
    return keys
}

// memberViewKeys 是「成员」的默认可见范围：与本次改动之前一致——
// 成员能看全部业务数据、能点提醒，但不能改任何东西。
func memberViewKeys() []string {
    return []string{
        PermTodayView, PermTodayRemind,
        PermMedicineView, PermPlanView, PermPrescView, PermImageView,
        PermStatsView, PermMemberView, PermMemberInvite,
    }
}

// permissionOrder 返回能力键在权威清单里的下标，未知键返回 -1。
func permissionOrder(key string) int {
    for i, item := range allPermissionKeys() {
        if item == key {
            return i
        }
    }
    return -1
}

// legacyPermissionKeys 是上一版五种粗粒度能力的键。那一版没发过包，
// 但真机与本地库里可能已经留下过，遇到就展开成「模块:动作」，
// 别让一次升级把别人干活的权限清零。
var legacyPermissionKeys = map[string][]string{
    "remind":   {PermTodayRemind},
    "record":   {PermTodayRecord},
    "medicine": {PermMedicineView, PermMedicineCreate, PermMedicineEdit, PermMedicineDelete, PermPrescView, PermPrescCreate, PermPrescEdit, PermPrescDelete},
    "plan":     {PermPlanView, PermPlanCreate, PermPlanEdit, PermPlanDelete},
    "image":    {PermImageView, PermImageCreate, PermImageEdit, PermImageDelete},
}

// expandLegacyKeys 把旧键换成新键；没有旧键时原样返回。
func expandLegacyKeys(in []string) []string {
    out := make([]string, 0, len(in)+8)
    legacy := false
    for _, key := range in {
        if mapped, hit := legacyPermissionKeys[strings.TrimSpace(key)]; hit {
            legacy = true
            out = append(out, mapped...)
            continue
        }
        out = append(out, key)
    }
    if !legacy {
        return in
    }
    // 旧模型里「能看」是不言自明的，展开后补上默认可见范围。
    return append(out, memberViewKeys()...)
}

// unionPermissionKeys 合并去重并按权威顺序排列；未知键直接丢掉。
func unionPermissionKeys(groups ...[]string) []string {
    seen := map[string]bool{}
    out := make([]string, 0, 24)
    for _, group := range groups {
        for _, key := range group {
            key = strings.TrimSpace(key)
            if key == "" || seen[key] || permissionOrder(key) < 0 {
                continue
            }
            seen[key] = true
            out = append(out, key)
        }
    }
    for i := 1; i < len(out); i++ {
        for j := i; j > 0 && permissionOrder(out[j]) < permissionOrder(out[j-1]); j-- {
            out[j], out[j-1] = out[j-1], out[j]
        }
    }
    return out
}

// normalizePermissionKeys 校验并整理管理员提交的勾选：丢掉空项与重复项，
// 出现不认识的能力键直接报错（这是人提交的输入，宁可退回重填）。
func normalizePermissionKeys(in []string) ([]string, error) {
    expanded := expandLegacyKeys(in)
    for _, raw := range expanded {
        key := strings.TrimSpace(raw)
        if key == "" {
            continue
        }
        if permissionOrder(key) < 0 {
            return nil, errors.New("包含不认识的能力项：" + key)
        }
    }
    return unionPermissionKeys(expanded), nil
}

// defaultPermissions 返回某个角色（加上「是否用药人」标记）的默认勾选。
// 口径与本次改动之前完全一致，没有单独勾选时表现不变：
//   - 管理员与监护人：全部格子
//   - 成员：能看、能点提醒，不能改任何数据
//   - 用药人本人：再多一格「点完成」
func defaultPermissions(role string, isCareRecipient bool) []string {
    switch role {
    case RoleOwner, RoleManager:
        return allPermissionKeys()
    default:
        keys := memberViewKeys()
        if isCareRecipient {
            keys = append(keys, PermTodayRecord)
        }
        return unionPermissionKeys(keys)
    }
}

// permissionsOverride 解析成员上存的勾选值。空串＝没有单独勾选（跟随角色默认）；
// 合法 JSON 数组（包括空数组）＝管理员显式勾过，返回 override=true。
// 解析失败按「没有勾选」处理，绝不让一条脏数据把人锁在门外。
func permissionsOverride(raw string) (keys []string, override bool) {
    if strings.TrimSpace(raw) == "" {
        return nil, false
    }
    var parsed []string
    if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
        return nil, false
    }
    return unionPermissionKeys(expandLegacyKeys(parsed)), true
}

// encodePermissions 把勾选集合序列化成入库文本；nil 与空集合都写成 "[]"，
// 与「没有勾选」的空串区分开。
func encodePermissions(keys []string) string {
    if keys == nil {
        keys = []string{}
    }
    b, err := json.Marshal(keys)
    if err != nil {
        return "[]"
    }
    return string(b)
}

// effectivePermissions 是某位成员最终生效的勾选。
// 管理员恒为全部：成员管理只有管理员能做，把他关在门外整本就没法管了。
func effectivePermissions(m BookMember) []string {
    if m.Role == RoleOwner {
        return allPermissionKeys()
    }
    if keys, override := permissionsOverride(m.Permissions); override {
        return keys
    }
    return defaultPermissions(m.Role, m.IsCareRecipient)
}

// memberHasPermission 判断成员是否勾有某项能力。
func memberHasPermission(m BookMember, key string) bool {
    return containsString(effectivePermissions(m), key)
}

// memberCanView 判断成员能否看某个模块（读接口的门禁）。
func memberCanView(m BookMember, module string) bool {
    return memberHasPermission(m, permKey(module, ActView))
}

// containsString 是集合判断的小工具，避免为一次包含判断引入 sort/slices。
func containsString(items []string, target string) bool {
    for _, item := range items {
        if item == target {
            return true
        }
    }
    return false
}

// permissionLabel 返回能力的中文全称，例如「药品与库存 · 删除」，用于报错文案。
func permissionLabel(key string) string {
    for _, module := range permissionModules() {
        for _, action := range module.Actions {
            if action.Key == key {
                return module.Label + " · " + action.Label
            }
        }
    }
    return key
}

// permissionInput 是管理员提交的勾选载体。
// 用 *[]string 分不出「没传这个字段」和「传了 null」，而这两者含义不同：
// 前者是调用错误，后者是「恢复跟随角色默认」，所以这里留住原始字节自己判。
type permissionInput struct {
    raw     []byte
    present bool
}

func (in *permissionInput) UnmarshalJSON(data []byte) error {
    in.raw = append(in.raw[:0], data...)
    in.present = true
    return nil
}

// submitted 表示请求体里确实带了 permissions 字段。
func (in permissionInput) submitted() bool { return in.present }

// isNull 表示显式传了 null，即恢复跟随角色默认。
func (in permissionInput) isNull() bool {
    return strings.TrimSpace(string(in.raw)) == "null"
}

// keys 解析并校验提交的数组。
func (in permissionInput) keys() ([]string, error) {
    var list []string
    if err := json.Unmarshal(in.raw, &list); err != nil {
        return nil, errors.New("permissions 需要是由能力键组成的数组，或 null")
    }
    return normalizePermissionKeys(list)
}
