package medcare

import (
    "path/filepath"
    "testing"

    "gorm.io/driver/sqlite"
    "gorm.io/gorm"
    gormlogger "gorm.io/gorm/logger"
)

// 药品删掉后，若计划已改到现有药品，遗留在旧药品上的待用药事件应被自动对齐到计划药品；
// 历史（完成/漏服/跳过）事件不能动。
func TestRepairOrphanEventMedicines(t *testing.T) {
    db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "repair.db")), &gorm.Config{Logger: gormlogger.Default.LogMode(gormlogger.Silent)})
    if err != nil { t.Fatal(err) }
    if err := db.AutoMigrate(&Medicine{}, &Plan{}, &DoseEvent{}); err != nil { t.Fatal(err) }

    bookID := uint(1)
    oldMed := Medicine{ID: 1, BookID: bookID, Name: "旧药", Category: CatOral, Unit: "片"}
    newMed := Medicine{ID: 2, BookID: bookID, Name: "新药(赛能)", Category: CatOral, Unit: "片"}
    db.Create(&oldMed); db.Create(&newMed)
    // 旧药归档
    db.Model(&Medicine{}).Where("id = ?", 1).Update("archived_at", nowUTC())
    // 计划已改到新药
    plan := Plan{ID: 10, BookID: bookID, MedicineID: 2, Type: CatOral, DoseAmount: 1, DoseUnit: "片", FrequencyType: FreqDaily, Times: "[\"08:00\"]", StartDate: "2026-11-22", Timezone: BeijingTimezone, Status: PlanActive}
    db.Create(&plan)
    // 待用药事件仍挂旧药
    db.Create(&DoseEvent{ID: 100, BookID: bookID, PlanID: 10, MedicineID: 1, MedicineName: "旧药", Type: CatOral, DoseAmount: 1, DoseUnit: "片", LocalDate: "2026-11-22", LocalTime: "08:00", ScheduledAt: nowUTC(), Status: EventPending})
    // 历史已完成事件也挂旧药，不应被改
    db.Create(&DoseEvent{ID: 101, BookID: bookID, PlanID: 10, MedicineID: 1, MedicineName: "旧药", Type: CatOral, DoseAmount: 1, DoseUnit: "片", LocalDate: "2026-11-20", LocalTime: "08:00", ScheduledAt: nowUTC().Add(-48 * 3600e9), Status: EventDone})

    repairOrphanEventMedicines(db, bookID)

    var pending, done DoseEvent
    db.First(&pending, 100)
    db.First(&done, 101)
    if pending.MedicineID != 2 || pending.MedicineName != "新药(赛能)" {
        t.Fatalf("待用药事件应改挂新药，实际 id=%d name=%s", pending.MedicineID, pending.MedicineName)
    }
    if done.MedicineID != 1 {
        t.Fatalf("历史已完成事件不应被修改，实际 medicine_id=%d", done.MedicineID)
    }
}
