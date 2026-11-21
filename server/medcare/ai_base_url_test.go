package medcare

import (
    "strings"
    "testing"
)

// AI 服务地址校验：合法用法要放行（含本机与局域网自建服务），
// 非法协议、URL 内嵌凭据与云元数据地址要挡住。
func TestValidateAIBaseURL(t *testing.T) {
    valid := []string{
        "",
        "https://api.openai.com/v1",
        "https://api.deepseek.com",
        "http://127.0.0.1:11434/v1",   // 本机 Ollama
        "http://192.168.1.10:1234/v1", // 局域网 LM Studio
        "http://nas.local:8080/v1",    // 内网主机名
    }
    for _, v := range valid {
        if err := validateAIBaseURL(v); err != nil {
            t.Fatalf("%q 应被接受，却报错: %v", v, err)
        }
    }

    invalid := map[string]string{
        "ftp://example.com/v1":                "只支持 http 或 https",
        "file:///etc/passwd":                  "只支持 http 或 https",
        "https://":                            "缺少主机名",
        "https://user:pass@example.com/v1":    "不要内嵌账号密码",
        "http://169.254.169.254/latest/meta":  "链路本地地址",
        "http://metadata.google.internal/v1":  "云元数据服务",
        "https://" + strings.Repeat("a", 300): "过长",
    }
    for v, want := range invalid {
        err := validateAIBaseURL(v)
        if err == nil {
            t.Fatalf("%q 应被拒绝", v)
        }
        if !strings.Contains(err.Error(), want) {
            t.Fatalf("%q 的报错应提示「%s」，实际: %v", v, want, err)
        }
    }
}
