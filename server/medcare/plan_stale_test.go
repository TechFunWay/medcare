package medcare

import (
    "path/filepath"
    "testing"
    "time"

    "gorm.io/driver/sqlite"
    "gorm.io/gorm"
    gormlogger "gorm.io/gorm/logger"
)

func newPlanTestDB(t *testing.T) *gorm.DB {
    t.Helper()
    db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "plan.db")), &gorm.Config{Logger: gormlogger.Default.LogMode(gormlogger.Silent)})
    if err != nil {
        t.Fatal(err)
    }
    if err := db.AutoMigrate(&Medicine{}, &Plan{}, &DoseEvent{}, &MedicationBook{}, &NotificationJob{}); err != nil {
        t.Fatal(err)
    }
    return db
}

// 改计划后：未完成的（过去的、今天的、未来的）全部清掉等重生成；已完成的保留。
func TestDeleteStalePendingEventsKeepsDoneClearsPending(t *testing.T) {
    db := newPlanTestDB(t)
    db.Create(&MedicationBook{ID: 1, Name: "本", Timezone: BeijingTimezone, Status: "active"})
    db.Create(&Medicine{ID: 1, BookID: 1, Name: "磺达肝癸钠注射液", Category: CatInjection, Unit: "支"})
    today := todayIn(BeijingTimezone)
    yday := addDays(today, -1)

    plan := Plan{ID: 1, BookID: 1, MedicineID: 1, Type: CatInjection, DoseAmount: 1, DoseUnit: "支",
        FrequencyType: FreqDaily, Times: "[\"08:00\"]", StartDate: yday, Timezone: BeijingTimezone, Status: PlanActive}
    db.Create(&plan)

    // 过去未完成、今天未完成、未来未完成 —— 都应被清掉
    db.Create(&DoseEvent{ID: 100, BookID: 1, PlanID: 1, MedicineID: 1, MedicineName: "药", Type: CatInjection,
        DoseAmount: 1, DoseUnit: "支", LocalDate: yday, LocalTime: "08:00", ScheduledAt: parseAt(t, yday+" 08:00"), Status: EventPending})
    db.Create(&DoseEvent{ID: 101, BookID: 1, PlanID: 1, MedicineID: 1, MedicineName: "药", Type: CatInjection,
        DoseAmount: 1, DoseUnit: "支", LocalDate: today, LocalTime: "08:00", ScheduledAt: parseAt(t, today+" 08:00"), Status: EventPending})
    db.Create(&DoseEvent{ID: 102, BookID: 1, PlanID: 1, MedicineID: 1, MedicineName: "药", Type: CatInjection,
        DoseAmount: 1, DoseUnit: "支", LocalDate: addDays(today, 1), LocalTime: "08:00", ScheduledAt: parseAt(t, addDays(today, 1)+" 08:00"), Status: EventPending})

    // 已完成 / 漏服 / 跳过 —— 都应保留
    db.Create(&DoseEvent{ID: 200, BookID: 1, PlanID: 1, MedicineID: 1, MedicineName: "药", Type: CatInjection,
        DoseAmount: 1, DoseUnit: "支", LocalDate: addDays(today, -3), LocalTime: "08:00", ScheduledAt: parseAt(t, addDays(today, -3)+" 08:00"), Status: EventDone})
    db.Create(&DoseEvent{ID: 201, BookID: 1, PlanID: 1, MedicineID: 1, MedicineName: "药", Type: CatInjection,
        DoseAmount: 1, DoseUnit: "支", LocalDate: addDays(today, -4), LocalTime: "08:00", ScheduledAt: parseAt(t, addDays(today, -4)+" 08:00"), Status: EventMissed})
    db.Create(&DoseEvent{ID: 202, BookID: 1, PlanID: 1, MedicineID: 1, MedicineName: "药", Type: CatInjection,
        DoseAmount: 1, DoseUnit: "支", LocalDate: addDays(today, -5), LocalTime: "08:00", ScheduledAt: parseAt(t, addDays(today, -5)+" 08:00"), Status: EventSkipped})

    if err := deleteStalePendingEvents(db, plan); err != nil {
        t.Fatal(err)
    }

    var pending int64
    db.Model(&DoseEvent{}).Where("plan_id = ? AND status = ?", 1, EventPending).Count(&pending)
    if pending != 0 {
        t.Fatalf("未完成事件应全部清掉，实际剩 %d", pending)
    }
    var kept int64
    db.Model(&DoseEvent{}).Where("id IN ?", []uint{200, 201, 202}).Count(&kept)
    if kept != 3 {
        t.Fatalf("已完成/漏服/跳过应全部保留，实际剩 %d", kept)
    }
}

// 计划已结束/归档：它名下未完成事件全部清掉（不限日期），历史保留。
func TestRepairStalePlanEventsClearsAllPendingOfInactivePlan(t *testing.T) {
    db := newPlanTestDB(t)
    db.Create(&MedicationBook{ID: 1, Name: "本", Timezone: BeijingTimezone, Status: "active"})
    db.Create(&Medicine{ID: 1, BookID: 1, Name: "药", Category: CatOral, Unit: "片"})
    today := todayIn(BeijingTimezone)
    yday := addDays(today, -1)
    db.Create(&Plan{ID: 2, BookID: 1, MedicineID: 1, Type: CatOral, DoseAmount: 1, DoseUnit: "片",
        FrequencyType: FreqDaily, Times: "[\"08:00\"]", StartDate: yday, Timezone: BeijingTimezone, Status: PlanEnded})
    db.Create(&DoseEvent{ID: 300, BookID: 1, PlanID: 2, MedicineID: 1, MedicineName: "药", Type: CatOral,
        DoseAmount: 1, DoseUnit: "片", LocalDate: yday, LocalTime: "08:00", ScheduledAt: parseAt(t, yday+" 08:00"), Status: EventPending})
    db.Create(&DoseEvent{ID: 301, BookID: 1, PlanID: 2, MedicineID: 1, MedicineName: "药", Type: CatOral,
        DoseAmount: 1, DoseUnit: "片", LocalDate: today, LocalTime: "08:00", ScheduledAt: parseAt(t, today+" 08:00"), Status: EventPending})
    db.Create(&DoseEvent{ID: 302, BookID: 1, PlanID: 2, MedicineID: 1, MedicineName: "药", Type: CatOral,
        DoseAmount: 1, DoseUnit: "片", LocalDate: addDays(today, 3), LocalTime: "08:00", ScheduledAt: parseAt(t, addDays(today, 3)+" 08:00"), Status: EventPending})
    db.Create(&DoseEvent{ID: 303, BookID: 1, PlanID: 2, MedicineID: 1, MedicineName: "药", Type: CatOral,
        DoseAmount: 1, DoseUnit: "片", LocalDate: addDays(today, -2), LocalTime: "08:00", ScheduledAt: parseAt(t, addDays(today, -2)+" 08:00"), Status: EventDone})

    repairStalePlanEvents(db, 1)

    var pending int64
    db.Model(&DoseEvent{}).Where("plan_id = ? AND status = ?", 2, EventPending).Count(&pending)
    if pending != 0 {
        t.Fatalf("已结束计划的未完成事件应全部清掉，实际剩 %d", pending)
    }
    var done int64
    db.Model(&DoseEvent{}).Where("id = ?", 303).Count(&done)
    if done != 1 {
        t.Fatalf("已完成事件应保留")
    }
}

func parseAt(t *testing.T, s string) time.Time {
    t.Helper()
    at, err := time.ParseInLocation("2006-01-02 15:04", s, loadLocation(BeijingTimezone))
    if err != nil {
        t.Fatal(err)
    }
    return at.UTC()
}
