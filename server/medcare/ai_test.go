package medcare

import "testing"

func TestExtractJSONObject(t *testing.T) {
    cases := []string{
        "{\"a\":1}",
        "```json\n{\"a\":1}\n```",
        "这是结果：\n{\"a\":1}\n以上。",
        "{\"a\":\"含 } 括号\"}",
        "no json here",
    }
    for _, c := range cases {
        got := extractJSONObject(c)
        if c == "no json here" {
            if got != c {
                t.Fatalf("expected passthrough, got %q", got)
            }
            continue
        }
        if got[0] != '{' {
            t.Fatalf("expected object start, got %q", got)
        }
    }
}

func TestNormalizeAIBaseURL(t *testing.T) {
    cases := []struct{ in, want string }{
        {"", ""},
        {"https://token-plan-cn.xiaomimimo.com", "https://token-plan-cn.xiaomimimo.com/v1"},
        {"https://token-plan-cn.xiaomimimo.com/", "https://token-plan-cn.xiaomimimo.com/v1"},
        {"https://api.example.com/v1", "https://api.example.com/v1"},
        {"https://api.example.com/v1/", "https://api.example.com/v1"},
        {"https://api.example.com/v2", "https://api.example.com/v2"},
        {"https://api.example.com/v1/chat/completions", "https://api.example.com/v1/chat/completions"},
        {"https://api.example.com/v1/models", "https://api.example.com/v1/models"},
        {"  https://api.example.com/v1  ", "https://api.example.com/v1"},
        {"https://ark.cn-beijing.volces.com/api/v3", "https://ark.cn-beijing.volces.com/api/v3"},
    }
    for _, c := range cases {
        if got := normalizeAIBaseURL(c.in); got != c.want {
            t.Fatalf("normalizeAIBaseURL(%q) = %q, want %q", c.in, got, c.want)
        }
    }
}

func TestAIEndpoints(t *testing.T) {
    cases := []struct{ base, chat, models string }{
        {"https://api.example.com", "https://api.example.com/v1/chat/completions", "https://api.example.com/v1/models"},
        {"https://api.example.com/v1", "https://api.example.com/v1/chat/completions", "https://api.example.com/v1/models"},
        {"https://api.example.com/v1/chat/completions", "https://api.example.com/v1/chat/completions", "https://api.example.com/v1/models"},
        {"https://api.example.com/v1/models", "https://api.example.com/v1/models/chat/completions", "https://api.example.com/v1/models"},
    }
    for _, c := range cases {
        if got := aiEndpoint(c.base); got != c.chat {
            t.Fatalf("aiEndpoint(%q) = %q, want %q", c.base, got, c.chat)
        }
        if got := aiModelsEndpoint(c.base); got != c.models {
            t.Fatalf("aiModelsEndpoint(%q) = %q, want %q", c.base, got, c.models)
        }
    }
}
