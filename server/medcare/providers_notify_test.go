package medcare

import (
    "strings"
    "testing"
    "time"
)

// 通知正文：用药本名用方括号、第一行说明药品类型、第二行给出原计划时间与超时时长。
func TestNotificationMessageFormat(t *testing.T) {
    ev := DoseEvent{
        ID: 1, MedicineName: "环孢素软胶囊", Type: CatInfusion,
        DoseAmount: 2, DoseUnit: "片", LocalTime: "08:00",
        ScheduledAt: nowUTC().Add(-8*time.Hour - 6*time.Minute),
    }
    msg := notificationMessage{
        BookName: "宝贝用药本",
        Title:    "还没吃药 · 再次提醒",
        Lines:    notificationJobLines(ev, UserPreference{}),
    }
    s := messageText(msg)
    if !strings.Contains(s, "【用药管理】[宝贝用药本]") {
        t.Fatalf("用药本名应带方括号：%s", s)
    }
    if !strings.Contains(s, "环孢素软胶囊（输液）") {
        t.Fatalf("应说明药品类型：%s", s)
    }
    if !strings.Contains(s, "原计划 08:00") {
        t.Fatalf("应给出原计划时间：%s", s)
    }
    if !strings.Contains(s, "已超时 8 小时 6 分钟") {
        t.Fatalf("应给出超时时长：%s", s)
    }
}

// 到点前不应出现「已超时」。
func TestNotificationNoOverdueBeforeTime(t *testing.T) {
    ev := DoseEvent{ID: 1, MedicineName: "药", Type: CatOral, DoseAmount: 1, DoseUnit: "片", LocalTime: "20:00", ScheduledAt: nowUTC().Add(2 * time.Hour)}
    lines := notificationJobLines(ev, UserPreference{})
    joined := strings.Join(lines, "\n")
    if strings.Contains(joined, "已超时") {
        t.Fatalf("未到点不应出现已超时：%s", joined)
    }
    if !strings.Contains(joined, "原计划 20:00") {
        t.Fatalf("应给出原计划时间：%s", joined)
    }
}

func TestHumanDurationCN(t *testing.T) {
    cases := []struct {
        d time.Duration
        want string
    }{
        {45 * time.Minute, "45 分钟"},
        {8*time.Hour + 6*time.Minute, "8 小时 6 分钟"},
        {26*time.Hour + 5*time.Minute, "1 天 2 小时 5 分钟"},
    }
    for _, c := range cases {
        if got := humanDurationCN(c.d); got != c.want {
            t.Fatalf("humanDurationCN(%v)=%q 期望 %q", c.d, got, c.want)
        }
    }
}
