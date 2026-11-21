package medcare

import (
    "testing"
    "time"

    "smallgo/server/database"

    "gorm.io/gorm"
)

func setupTestDB(t *testing.T) *gorm.DB {
    t.Helper()
    dir := t.TempDir()
    db, err := database.InitDB(dir + "/test.db")
    if err != nil {
        t.Fatalf("InitDB: %v", err)
    }
    if err := database.AutoMigrate(db); err != nil {
        t.Fatalf("AutoMigrate: %v", err)
    }
    return db
}

func TestDailySummaryLines(t *testing.T) {
    db := setupTestDB(t)
    book := MedicationBook{Name: "总结本", CareRecipientName: "李奶奶", Timezone: "Asia/Shanghai", OwnerUserID: 1, CreatedBy: 1, Status: BookActive}
    if err := db.Create(&book).Error; err != nil {
        t.Fatal(err)
    }
    now := nowUTC()
    local := now.In(loadLocation("Asia/Shanghai"))
    date := local.Format("2006-01-02")
    events := []DoseEvent{
        {BookID: book.ID, PlanID: 1, MedicineID: 1, MedicineName: "药A", Type: CatOral, DoseAmount: 1, DoseUnit: "片", ScheduledAt: now.Add(-3 * time.Hour), LocalDate: date, LocalTime: "08:00", Status: EventDone},
        {BookID: book.ID, PlanID: 1, MedicineID: 1, MedicineName: "药A", Type: CatOral, DoseAmount: 1, DoseUnit: "片", ScheduledAt: now.Add(-2 * time.Hour), LocalDate: date, LocalTime: "12:00", Status: EventMissed},
        {BookID: book.ID, PlanID: 1, MedicineID: 1, MedicineName: "药A", Type: CatOral, DoseAmount: 1, DoseUnit: "片", ScheduledAt: now.Add(-1 * time.Hour), LocalDate: date, LocalTime: "18:00", Status: EventSkipped, SkipReason: SkipDoctor},
    }
    for i := range events {
        if err := db.Create(&events[i]).Error; err != nil {
            t.Fatal(err)
        }
    }
    lines := dailySummaryLines(db, book.ID, date)
    if len(lines) != 3 {
        t.Fatalf("expected 3 lines, got %d: %v", len(lines), lines)
    }
    if lines[0] != "完成 1 项 · 漏服 1 项 · 待完成 0 项" {
        t.Fatalf("unexpected first line: %s", lines[0])
    }
}

func TestCreateSummaryJobIdempotent(t *testing.T) {
    db := setupTestDB(t)
    runAt := nowUTC()
    createSummaryJob(db, 1, 1, 0, ChannelInApp, "2026-11-03", runAt)
    createSummaryJob(db, 1, 1, 0, ChannelInApp, "2026-11-03", runAt)
    var count int64
    db.Model(&NotificationJob{}).Where("dedupe_key = ?", "summary:2026-11-03:member:1:channel:0").Count(&count)
    if count != 1 {
        t.Fatalf("expected 1 summary job, got %d", count)
    }
}

func TestRunDailySummaryCreatesJobs(t *testing.T) {
    db := setupTestDB(t)
    realtimeDB = db
    now := nowUTC()
    local := now.In(loadLocation("Asia/Shanghai"))
    book := MedicationBook{Name: "本", CareRecipientName: "李奶奶", Timezone: "Asia/Shanghai", OwnerUserID: 1, CreatedBy: 1, Status: BookActive}
    db.Create(&book)
    db.Create(&BookMember{BookID: book.ID, UserID: 1, Role: RoleOwner, NotificationsEnabled: true, JoinedAt: now})
    db.Create(&UserPreference{UserID: 1, Timezone: "Asia/Shanghai", DailySummaryEnabled: true, DailySummaryTime: local.Format("15:04")})
    runDailySummary()
    var count int64
    db.Model(&NotificationJob{}).Where("kind = ?", JobSummary).Count(&count)
    if count == 0 {
        t.Fatal("expected daily summary job to be created at the configured minute")
    }
}

func TestBackfillMemberJobs(t *testing.T) {
    db := setupTestDB(t)
    now := nowUTC()
    book := MedicationBook{Name: "本", CareRecipientName: "李奶奶", Timezone: "Asia/Shanghai", OwnerUserID: 1, CreatedBy: 1, Status: BookActive}
    db.Create(&book)
    db.Create(&BookMember{BookID: book.ID, UserID: 1, Role: RoleOwner, NotificationsEnabled: true, JoinedAt: now})
    plan := Plan{BookID: book.ID, MedicineID: 1, Title: "计划", Type: CatOral, DoseAmount: 1, DoseUnit: "片", FrequencyType: FreqDaily, Times: encodeJSON([]string{"08:00"}), StartDate: now.In(loadLocation("Asia/Shanghai")).Format("2006-01-02"), Timezone: "Asia/Shanghai", Status: PlanActive, NotificationsEnabled: true, Version: 1}
    db.Create(&plan)
    ev := DoseEvent{BookID: book.ID, PlanID: plan.ID, MedicineID: 1, MedicineName: "药", Type: CatOral, DoseAmount: 1, DoseUnit: "片", ScheduledAt: now.Add(2 * time.Hour), LocalDate: now.In(loadLocation("Asia/Shanghai")).Format("2006-01-02"), LocalTime: "20:00", Status: EventPending, Version: 1}
    db.Create(&ev)
    db.Create(&NotificationChannel{UserID: 1, Type: ChannelEmail, Name: "邮箱", Enabled: true, ConfigEncrypted: "x", TargetMasked: "a***@b.c", Status: "active"})
    backfillMemberJobs(db, book.ID, 1)
    var count int64
    db.Model(&NotificationJob{}).Where("event_id = ?", ev.ID).Count(&count)
    if count == 0 {
        t.Fatal("expected jobs for existing event after backfill")
    }
}
