package medcare

import (
    "net/http"
    "net/http/httptest"
    "path/filepath"
    "strconv"
    "strings"
    "testing"
    "time"

    "github.com/gin-gonic/gin"
    "gorm.io/driver/sqlite"
    "gorm.io/gorm"
    gormlogger "gorm.io/gorm/logger"

    "smallgo/server/database"
)

func setupReliabilityDB(t *testing.T) *gorm.DB {
    t.Helper()
    db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "test.db")), &gorm.Config{Logger: gormlogger.Default.LogMode(gormlogger.Silent)})
    if err != nil {
        t.Fatalf("open db: %v", err)
    }
    if err := db.AutoMigrate(&NotificationJob{}, &DoseEvent{}, &Medicine{}, &MedicineBatch{},
        &StockMovement{}, &StockAllocation{}, &MedicationBook{}, &BookMember{}, &Plan{}, &database.AuditLog{}); err != nil {
        t.Fatalf("migrate: %v", err)
    }
    return db
}

// 进程重启遗留的 sending 任务必须被回收，否则提醒永久丢失。
func TestReclaimStaleSendingJobs(t *testing.T) {
    db := setupReliabilityDB(t)
    now := nowUTC()
    stale := now.Add(-staleSendingAfter - time.Minute)
    fresh := now.Add(-time.Second)

    staleJob := NotificationJob{BookID: 1, RecipientUserID: 1, Kind: JobDue, Status: JobSending, RunAt: now, DedupeKey: "stale", LockedAt: &stale, WorkerID: "w"}
    freshJob := NotificationJob{BookID: 1, RecipientUserID: 1, Kind: JobDue, Status: JobSending, RunAt: now, DedupeKey: "fresh", LockedAt: &fresh, WorkerID: "w"}
    // 锁字段为空的 sending 同样视为卡死。
    noLockJob := NotificationJob{BookID: 1, RecipientUserID: 1, Kind: JobDue, Status: JobSending, RunAt: now, DedupeKey: "nolock"}
    sentJob := NotificationJob{BookID: 1, RecipientUserID: 1, Kind: JobDue, Status: JobSent, RunAt: now, DedupeKey: "sent"}
    for _, j := range []*NotificationJob{&staleJob, &freshJob, &noLockJob, &sentJob} {
        if err := db.Create(j).Error; err != nil {
            t.Fatalf("create job: %v", err)
        }
    }

    if n := reclaimStaleSendingJobs(db); n != 2 {
        t.Fatalf("回收数量 = %d, want 2", n)
    }
    // 每次查询用独立结构体：复用带主键的目标会让 GORM 附加旧主键条件。
    var gotStale, gotFresh, gotSent NotificationJob
    if err := db.First(&gotStale, staleJob.ID).Error; err != nil {
        t.Fatalf("read stale: %v", err)
    }
    if gotStale.Status != JobRetry || gotStale.LockedAt != nil || gotStale.WorkerID != "" {
        t.Fatalf("卡死任务未复位: status=%s locked=%v worker=%q", gotStale.Status, gotStale.LockedAt, gotStale.WorkerID)
    }
    if err := db.First(&gotFresh, freshJob.ID).Error; err != nil {
        t.Fatalf("read fresh: %v", err)
    }
    if gotFresh.Status != JobSending {
        t.Fatalf("正在发送的任务被误抢: status=%s", gotFresh.Status)
    }
    if err := db.First(&gotSent, sentJob.ID).Error; err != nil {
        t.Fatalf("read sent: %v", err)
    }
    if gotSent.Status != JobSent {
        t.Fatalf("已发送任务被改动: status=%s", gotSent.Status)
    }
}

// 卡死的 sending 允许手动重试；正在发送的不允许，避免重复发送。
func TestRetryDeliveryRevivesStaleSendingOnly(t *testing.T) {
    db := setupReliabilityDB(t)
    now := nowUTC()
    stale := now.Add(-staleSendingAfter - time.Minute)
    fresh := now.Add(-time.Second)
    staleJob := NotificationJob{BookID: 1, RecipientUserID: 1, Kind: JobDue, Status: JobSending, RunAt: now, DedupeKey: "s", LockedAt: &stale}
    freshJob := NotificationJob{BookID: 1, RecipientUserID: 1, Kind: JobDue, Status: JobSending, RunAt: now, DedupeKey: "f", LockedAt: &fresh}
    db.Create(&staleJob)
    db.Create(&freshJob)

    if w := retryDeliveryRequest(t, db, staleJob.ID); w.Code != http.StatusOK {
        t.Fatalf("卡死任务重试 status = %d, want 200, body=%s", w.Code, w.Body.String())
    }
    var got NotificationJob
    db.First(&got, staleJob.ID)
    if got.Status != JobPending || got.LockedAt != nil {
        t.Fatalf("重试后状态不符: status=%s locked=%v", got.Status, got.LockedAt)
    }
    if w := retryDeliveryRequest(t, db, freshJob.ID); w.Code != http.StatusBadRequest {
        t.Fatalf("正在发送的任务不应可重试: status = %d", w.Code)
    }
}

func retryDeliveryRequest(t *testing.T, db *gorm.DB, id uint) *httptest.ResponseRecorder {
    t.Helper()
    gin.SetMode(gin.TestMode)
    r := gin.New()
    r.POST("/retry/:id", func(c *gin.Context) {
        c.Set("userID", uint(1))
        handleRetryDelivery(db)(c)
    })
    w := httptest.NewRecorder()
    r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/retry/"+idParam(id), strings.NewReader("")))
    return w
}

func idParam(v uint) string { return strconv.FormatUint(uint64(v), 10) }

// 打卡不带备注时，不能抹掉此前经 /note 写入的备注。
func TestCompleteEventKeepsExistingNote(t *testing.T) {
    db := setupReliabilityDB(t)
    now := nowUTC()
    book := MedicationBook{Name: "测试本", CareRecipientName: "用药人", OwnerUserID: 1, Status: BookActive, Timezone: BeijingTimezone, CreatedAt: now, UpdatedAt: now}
    if err := db.Create(&book).Error; err != nil {
        t.Fatalf("create book: %v", err)
    }
    member := BookMember{BookID: book.ID, UserID: 1, Role: RoleOwner, JoinedAt: now, CreatedAt: now, UpdatedAt: now}
    db.Create(&member)
    plan := Plan{BookID: book.ID, MedicineID: 1, Type: CatOral, Status: PlanActive, StartDate: "2026-11-13", Timezone: BeijingTimezone, CreatedAt: now, UpdatedAt: now}
    db.Create(&plan)
    ev := DoseEvent{
        BookID: book.ID, PlanID: plan.ID, MedicineID: 1, MedicineName: "测试药", Type: CatOral,
        DoseAmount: 1, DoseUnit: "片", ScheduledAt: now, LocalDate: todayIn(BeijingTimezone), LocalTime: "08:00",
        Status: EventPending, Note: "饭后服用", Version: 1, CreatedAt: now, UpdatedAt: now,
    }
    if err := db.Create(&ev).Error; err != nil {
        t.Fatalf("create event: %v", err)
    }

    // 不扣库存、不带备注地打卡。
    if w := completeEventRequest(t, db, book.ID, ev.ID, `{"deduct_stock":false}`); w.Code != http.StatusOK {
        t.Fatalf("打卡 status = %d, body=%s", w.Code, w.Body.String())
    }
    var first DoseEvent
    if err := db.First(&first, ev.ID).Error; err != nil {
        t.Fatalf("read event: %v", err)
    }
    if first.Status != EventDone {
        t.Fatalf("打卡未生效: status=%s", first.Status)
    }
    if first.Note != "饭后服用" {
        t.Fatalf("已有备注被抹掉: note=%q", first.Note)
    }

    // 显式带备注时照常覆盖。
    if err := db.Model(&DoseEvent{}).Where("id = ?", ev.ID).Updates(map[string]interface{}{"status": EventPending, "completed_at": nil}).Error; err != nil {
        t.Fatalf("reset event: %v", err)
    }
    if w := completeEventRequest(t, db, book.ID, ev.ID, `{"deduct_stock":false,"note":"改为空腹"}`); w.Code != http.StatusOK {
        t.Fatalf("二次打卡 status = %d, body=%s", w.Code, w.Body.String())
    }
    var second DoseEvent
    if err := db.First(&second, ev.ID).Error; err != nil {
        t.Fatalf("read event again: %v", err)
    }
    if second.Note != "改为空腹" {
        t.Fatalf("显式备注未写入: note=%q", second.Note)
    }
}

func completeEventRequest(t *testing.T, db *gorm.DB, bookID, eventID uint, body string) *httptest.ResponseRecorder {
    t.Helper()
    gin.SetMode(gin.TestMode)
    r := gin.New()
    r.POST("/complete/:id", func(c *gin.Context) {
        c.Set("userID", uint(1))
        c.Set("bookID", bookID)
        c.Set("bookRole", RoleOwner)
        c.Set("bookMember", BookMember{BookID: bookID, UserID: 1, Role: RoleOwner})
        handleCompleteEvent(db)(c)
    })
    w := httptest.NewRecorder()
    req := httptest.NewRequest(http.MethodPost, "/complete/"+idParam(eventID), strings.NewReader(body))
    req.Header.Set("Content-Type", "application/json")
    r.ServeHTTP(w, req)
    return w
}
