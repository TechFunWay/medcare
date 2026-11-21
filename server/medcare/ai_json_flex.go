package medcare

import (
    "encoding/json"
    "strconv"
    "strings"
)

// 大模型返回的 JSON 经常把数字写成字符串，还夹带单位或货币符号，
// 例如 quantity:"2 盒"、unit_price:"￥32.50"、dose_amount:"5mg"、times:"08:00, 20:00"。
// 这些宽松类型把「数字 / 数字字符串 / 带单位字符串」都吃下来，
// 避免因为一个字段类型不符就让整张单据解析失败。

func parseFlexNumber(s string) float64 {
    s = strings.TrimSpace(s)
    if s == "" {
        return 0
    }
    s = strings.NewReplacer(",", "", "，", "", "￥", "", "¥", "", " ", "").Replace(s)
    // 取第一个连续的数字片段，忽略后面的单位（盒/片/mg/元…）。
    var b strings.Builder
    started := false
    for _, r := range s {
        if (r >= '0' && r <= '9') || r == '.' || r == '-' || r == '+' {
            b.WriteRune(r)
            started = true
        } else if started {
            break
        }
    }
    if !started {
        return 0
    }
    v, err := strconv.ParseFloat(b.String(), 64)
    if err != nil {
        return 0
    }
    return v
}

// flexFloat 兼容数字、数字字符串与带单位字符串。
type flexFloat float64

func (f *flexFloat) UnmarshalJSON(b []byte) error {
    s := strings.TrimSpace(string(b))
    if s == "" || s == "null" {
        *f = 0
        return nil
    }
    if s[0] == '"' {
        var str string
        if err := json.Unmarshal(b, &str); err != nil {
            return err
        }
        *f = flexFloat(parseFlexNumber(str))
        return nil
    }
    var n float64
    if err := json.Unmarshal(b, &n); err != nil {
        // 布尔或对象等异常形状一律按 0 处理，不让整条记录失败。
        *f = 0
        return nil
    }
    *f = flexFloat(n)
    return nil
}

// flexInt 在 flexFloat 基础上取整，兼容 "7" / "7天" / 7。
type flexInt int

func (i *flexInt) UnmarshalJSON(b []byte) error {
    var f flexFloat
    if err := f.UnmarshalJSON(b); err != nil {
        return err
    }
    *i = flexInt(int(f + 0.0001))
    return nil
}

// flexBool 兼容 true / "true" / 1 / "1" / "是"。
type flexBool bool

func (v *flexBool) UnmarshalJSON(b []byte) error {
    s := strings.TrimSpace(string(b))
    switch s {
    case "true", "\"true\"", "1", "\"1\"", "\"是\"", "\"yes\"":
        *v = true
    default:
        *v = false
    }
    return nil
}

// flexString 兼容字符串、数字与布尔，避免 spec 之类的字段写成数字时解析失败。
type flexString string

func (s *flexString) UnmarshalJSON(b []byte) error {
    raw := strings.TrimSpace(string(b))
    if raw == "" || raw == "null" {
        *s = ""
        return nil
    }
    var str string
    if err := json.Unmarshal(b, &str); err == nil {
        *s = flexString(str)
        return nil
    }
    var num json.Number
    if err := json.Unmarshal(b, &num); err == nil {
        *s = flexString(num.String())
        return nil
    }
    var bo bool
    if err := json.Unmarshal(b, &bo); err == nil {
        *s = flexString(strconv.FormatBool(bo))
        return nil
    }
    *s = ""
    return nil
}

// flexStringList 兼容 ["08:00","20:00"]、"08:00, 20:00"、[8,20] 等写法。
type flexStringList []string

func (l *flexStringList) UnmarshalJSON(b []byte) error {
    raw := strings.TrimSpace(string(b))
    if raw == "" || raw == "null" {
        *l = nil
        return nil
    }
    if raw[0] == '[' {
        var arr []json.RawMessage
        if err := json.Unmarshal(b, &arr); err != nil {
            *l = nil
            return nil
        }
        out := make([]string, 0, len(arr))
        for _, item := range arr {
            var fs flexString
            if err := fs.UnmarshalJSON(item); err == nil {
                v := strings.TrimSpace(string(fs))
                if v != "" {
                    out = append(out, v)
                }
            }
        }
        *l = out
        return nil
    }
    var str string
    if err := json.Unmarshal(b, &str); err != nil {
        // 数字等标量：整体当一个值。
        var num json.Number
        if e := json.Unmarshal(b, &num); e == nil {
            *l = []string{num.String()}
            return nil
        }
        *l = nil
        return nil
    }
    parts := strings.FieldsFunc(str, func(r rune) bool {
        return r == ',' || r == '，' || r == '、' || r == ';' || r == '；' || r == ' ' || r == '/'
    })
    out := make([]string, 0, len(parts))
    for _, p := range parts {
        if v := strings.TrimSpace(p); v != "" {
            out = append(out, v)
        }
    }
    *l = out
    return nil
}
