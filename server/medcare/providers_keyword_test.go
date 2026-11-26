package medcare

import (
    "strings"
    "testing"
)

// 关键词必须出现在请求体里（钉钉按内容文本校验），且不出现在正文可见部分。
func TestDingTalkPayloadCarriesKeyword(t *testing.T) {
    msg := notificationMessage{BookName: "宝贝用药本", Title: "药品缺货提醒 · 叶酸", Lines: []string{"当前库存 0 片"}}
    cfg := dingTalkConfig{Keyword: "yygl"}
    p := dingTalkPayload(cfg, msg)
    md := p["markdown"].(map[string]string)
    title := md["title"]
    text := md["text"]
    if !strings.Contains(title, "yygl") {
        t.Fatalf("title 应带关键词，实际 %q", title)
    }
    // 正文可见部分（去掉隐藏链接那一段）不应包含关键词明文
    visible := text
    if i := strings.Index(text, "\n\n[\u200b]"); i >= 0 {
        visible = text[:i]
    }
    if strings.Contains(visible, "yygl") {
        t.Fatalf("正文可见部分不应出现关键词，实际 %q", visible)
    }
    // 隐藏链接里要有关键词，作为兜底命中
    if !strings.Contains(text, "yygl") {
        t.Fatalf("隐藏链接应带关键词，实际 %q", text)
    }
}

func TestDingTalkPayloadNoKeyword(t *testing.T) {
    msg := notificationMessage{Title: "测试", Lines: []string{"内容"}}
    p := dingTalkPayload(dingTalkConfig{}, msg)
    md := p["markdown"].(map[string]string)
    if strings.Contains(md["text"], "techfunway.invalid") {
        t.Fatalf("没配关键词时不应加隐藏链接：%q", md["text"])
    }
}
