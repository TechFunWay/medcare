package medcare

import (
    "strconv"
    "strings"

    "smallgo/server/database"

    "gorm.io/gorm"
)

// 药品基础信息字典的出厂默认值：程序内置，安装/首次启动时写入数据库（系统配置，user_id=0）。
// 管理员之后可以在「偏好设置 → 药品字典」里增删改；已有配置行时不再覆盖，避免把用户删掉的项又塞回去。
var (
    defaultFormOptions = []string{
        "片剂", "胶囊", "缓释片", "控释片", "分散片", "咀嚼片", "颗粒剂", "干混悬剂",
        "口服液", "口服溶液", "注射液", "粉针剂", "输液", "软膏", "乳膏", "凝胶",
        "滴眼液", "滴耳液", "滴鼻液", "喷雾剂", "气雾剂", "贴剂", "栓剂", "其他",
    }
    defaultUnitOptions = []string{
        "片", "粒", "颗", "袋", "支", "瓶", "盒", "板", "包", "管", "贴", "喷", "滴", "次",
        "ml", "g", "mg", "IU",
    }
    defaultSpecOptions = []string{
        "5mg", "10mg", "20mg", "25mg", "50mg", "75mg", "100mg", "150mg", "200mg", "250mg", "500mg",
        "0.1g", "0.2g", "0.25g", "0.5g", "1g", "2g",
        "5ml", "10ml", "20ml", "50ml", "100ml", "250ml", "500ml",
        "10片/盒", "12片/盒", "20片/盒", "24片/盒", "30片/盒", "100片/瓶",
        "1支/盒", "1瓶/盒", "10袋/盒",
    }
    defaultPackageSpecOptions = []string{
        "10片/盒", "12片/盒", "20片/盒", "24片/盒", "30片/盒", "36片/盒", "48片/盒",
        "60片/瓶", "100片/瓶", "1支/盒", "5支/盒", "10支/盒", "1瓶/盒", "1袋/盒", "10袋/盒", "1板/盒",
    }
)

// medicineDictSeedVersion 字典出厂数据版本：每次往默认列表里加新项就 +1。
// 升级时只把「新版本才出现」的默认项追加到已有字典末尾（不覆盖管理员的自定义与排序），
// 管理员以前手动删掉的旧项也不会因为版本升级又冒出来。
const medicineDictSeedVersion = 2

// dedupeOptions 去空白、去重并保持首次出现顺序；字典写入与合并都用它。
func dedupeOptions(list []string) []string {
    seen := make(map[string]bool, len(list))
    out := make([]string, 0, len(list))
    for _, v := range list {
        v = strings.TrimSpace(v)
        if v == "" || seen[v] {
            continue
        }
        seen[v] = true
        out = append(out, v)
    }
    return out
}

// seedMedicineDict 把内置默认字典写入系统配置（仅当该键还没有记录时）。
// 已有记录（哪怕是空串，表示管理员清空过）一律不动。
func seedMedicineDict(db *gorm.DB) {
    seeds := []struct {
        key   string
        items []string
    }{
        {"medcare_form_options", defaultFormOptions},
        {"medcare_unit_options", defaultUnitOptions},
        {"medcare_spec_options", defaultSpecOptions},
        {"medcare_package_spec_options", defaultPackageSpecOptions},
    }
    // 已应用过的出厂版本号：0 表示还没跑过（老库）或全新库。
    appliedVersion := 0
    var verRow database.SystemConfig
    if err := db.Where("user_id = 0 AND key = ?", "medcare_dict_seed_version").First(&verRow).Error; err == nil {
        if n, convErr := strconv.Atoi(strings.TrimSpace(verRow.Value)); convErr == nil {
            appliedVersion = n
        }
    }
    for _, s := range seeds {
        defaults := dedupeOptions(s.items)
        var row database.SystemConfig
        err := db.Where("user_id = 0 AND key = ?", s.key).First(&row).Error
        switch {
        case err == gorm.ErrRecordNotFound:
            db.Create(&database.SystemConfig{UserID: 0, Key: s.key, Value: strings.Join(defaults, ","), Public: true})
        case err != nil:
            continue
        case strings.TrimSpace(row.Value) == "":
            // 空值等于「用内置默认」，补全默认项。
            db.Model(&row).Updates(map[string]interface{}{"value": strings.Join(defaults, ","), "public": true})
        case appliedVersion < medicineDictSeedVersion:
            // 老库升级：把默认项里「还没有」的追加到末尾，保留管理员已有内容与顺序。
            merged := mergeDefaults(splitOptions(row.Value), defaults)
            db.Model(&row).Updates(map[string]interface{}{"value": strings.Join(merged, ","), "public": true})
        }
    }
    if appliedVersion < medicineDictSeedVersion {
        // 记下已应用的出厂版本，避免每次启动都重复合并。
        value := strconv.Itoa(medicineDictSeedVersion)
        if verRow.ID == 0 {
            db.Create(&database.SystemConfig{UserID: 0, Key: "medcare_dict_seed_version", Value: value})
        } else {
            db.Model(&verRow).Update("value", value)
        }
    }
}

// splitOptions 按逗号切分并去空白去重（与服务端存储格式一致）。
func splitOptions(raw string) []string {
    parts := strings.Split(raw, ",")
    return dedupeOptions(parts)
}

// mergeDefaults 把 defaults 里 current 还没有的项按「默认顺序」插到相邻默认项旁边，
// 这样新增「颗」会排在「粒」后面，而不是一律追加到末尾；管理员已有内容与顺序不变。
func mergeDefaults(current []string, defaults []string) []string {
    result := append([]string{}, current...)
    have := make(map[string]bool, len(current))
    for _, v := range current {
        have[v] = true
    }
    for i, def := range defaults {
        if have[def] {
            continue
        }
        // 找前面最近的、已经在 result 里的默认项，插到它后面；没有就插到最前面。
        anchor := -1
        for j := i - 1; j >= 0; j-- {
            for k, v := range result {
                if v == defaults[j] {
                    anchor = k
                    break
                }
            }
            if anchor >= 0 {
                break
            }
        }
        pos := 0
        if anchor >= 0 {
            pos = anchor + 1
        }
        result = append(result, "")
        copy(result[pos+1:], result[pos:])
        result[pos] = def
        have[def] = true
    }
    return result
}

