package medcare

import (
    "testing"
    "time"
)

func TestExpandDaily(t *testing.T) {
    plan := Plan{
        FrequencyType: FreqDaily, Times: encodeJSON([]string{"08:00", "20:00"}),
        StartDate: "2026-11-01", Timezone: "Asia/Shanghai",
    }
    times, err := expandPlan(plan, "2026-11-01", "2026-11-02")
    if err != nil {
        t.Fatalf("expandPlan error: %v", err)
    }
    if len(times) != 4 {
        t.Fatalf("expected 4 occurrences, got %d", len(times))
    }
    loc := loadLocation("Asia/Shanghai")
    if got := times[0].In(loc).Format("2006-01-02 15:04"); got != "2026-11-01 08:00" {
        t.Fatalf("first occurrence = %s", got)
    }
}

func TestExpandEveryNDays(t *testing.T) {
    plan := Plan{
        FrequencyType: FreqEveryNDays, IntervalDays: 2, Times: encodeJSON([]string{"09:00"}),
        AnchorDate: "2026-11-01", StartDate: "2026-11-01", Timezone: "Asia/Shanghai",
    }
    times, err := expandPlan(plan, "2026-11-01", "2026-11-06")
    if err != nil {
        t.Fatalf("expandPlan error: %v", err)
    }
    if len(times) != 3 {
        t.Fatalf("expected 3 occurrences (1/3/5), got %d", len(times))
    }
    loc := loadLocation("Asia/Shanghai")
    for i, want := range []string{"2026-11-01", "2026-11-03", "2026-11-05"} {
        if got := times[i].In(loc).Format("2006-01-02"); got != want {
            t.Fatalf("occurrence %d = %s, want %s", i, got, want)
        }
    }
}

func TestExpandWeekly(t *testing.T) {
    plan := Plan{
        FrequencyType: FreqWeekly, Weekdays: encodeJSON([]int{1, 3, 5}), Times: encodeJSON([]string{"08:30"}),
        StartDate: "2026-11-02", Timezone: "Asia/Shanghai",
    }
    times, err := expandPlan(plan, "2026-11-02", "2026-11-08")
    if err != nil {
        t.Fatalf("expandPlan error: %v", err)
    }
    if len(times) != 3 {
        t.Fatalf("expected 3 occurrences, got %d", len(times))
    }
    loc := loadLocation("Asia/Shanghai")
    for _, at := range times {
        wd := int(at.In(loc).Weekday())
        if wd == 0 {
            wd = 7
        }
        if wd != 1 && wd != 3 && wd != 5 {
            t.Fatalf("unexpected weekday %d", wd)
        }
    }
}

func TestExpandIntervalHours(t *testing.T) {
    start := time.Date(2026, 11, 1, 8, 0, 0, 0, loadLocation("Asia/Shanghai"))
    plan := Plan{
        FrequencyType: FreqIntervalHours, IntervalHours: 8, StartAt: &start,
        StartDate: "2026-11-01", Timezone: "Asia/Shanghai",
    }
    times, err := expandPlan(plan, "2026-11-01", "2026-11-02")
    if err != nil {
        t.Fatalf("expandPlan error: %v", err)
    }
    // 11-01: 08:00、16:00；11-02: 00:00、08:00、16:00（共 5 次）
    if len(times) != 5 {
        t.Fatalf("expected 5 occurrences across two days, got %d", len(times))
    }
    loc := loadLocation("Asia/Shanghai")
    if got := times[0].In(loc).Format("01-02 15:04"); got != "11-01 08:00" {
        t.Fatalf("first occurrence = %s", got)
    }
    if got := times[2].In(loc).Format("01-02 15:04"); got != "11-02 00:00" {
        t.Fatalf("third occurrence = %s", got)
    }
}

func TestExpandRespectsEndDate(t *testing.T) {
    plan := Plan{
        FrequencyType: FreqDaily, Times: encodeJSON([]string{"08:00"}),
        StartDate: "2026-11-01", EndDate: "2026-11-03", Timezone: "Asia/Shanghai",
    }
    times, err := expandPlan(plan, "2026-11-01", "2026-11-10")
    if err != nil {
        t.Fatalf("expandPlan error: %v", err)
    }
    if len(times) != 3 {
        t.Fatalf("expected 3 occurrences, got %d", len(times))
    }
}

func TestParseClockRejectsInvalid(t *testing.T) {
    if _, err := parseClock([]string{"25:00"}); err == nil {
        t.Fatal("expected error for invalid clock")
    }
    got, err := parseClock([]string{"20:00", "08:00", "08:00"})
    if err != nil {
        t.Fatalf("parseClock error: %v", err)
    }
    if len(got) != 2 || got[0] != "08:00" || got[1] != "20:00" {
        t.Fatalf("unexpected clock list: %v", got)
    }
}

func TestHashInviteTokens(t *testing.T) {
    a := sha256Hex("abc")
    b := sha256Hex("abc")
    if a != b {
        t.Fatal("hash must be deterministic")
    }
    if len(a) != 64 {
        t.Fatalf("unexpected hash length %d", len(a))
    }
}

func TestRoleRank(t *testing.T) {
    if !roleAtLeast(RoleOwner, RoleManager) || !roleAtLeast(RoleManager, RoleMember) {
        t.Fatal("owner should outrank manager, manager should outrank member")
    }
    if roleAtLeast(RoleMember, RoleManager) {
        t.Fatal("member must not pass manager check")
    }
}
