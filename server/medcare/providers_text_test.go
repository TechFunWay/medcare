package medcare

import (
    "strings"
    "testing"
)

// 通知正文不应包含访问路径（如 /today、/medicines），避免用户看到看不懂的地址。
func TestMessageTextOmitsAccessPath(t *testing.T) {
    msg := notificationMessage{
        BookName: "李奶奶的用药本",
        Title:    "药品缺货提醒 · 某药",
        Lines:    []string{"当前库存 0 瓶", "记得及时补货"},
        Link:     "https://example.com/today",
    }
    s := messageText(msg)
    if strings.Contains(s, "打开") {
        t.Fatalf("通知正文不应出现「打开」：%s", s)
    }
    if strings.Contains(s, "/today") || strings.Contains(s, "/medicines") {
        t.Fatalf("通知正文不应出现访问路径：%s", s)
    }
    if !strings.Contains(s, "药品缺货提醒") || !strings.Contains(s, "当前库存 0 瓶") {
        t.Fatalf("标题与正文丢了：%s", s)
    }
}
