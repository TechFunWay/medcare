package medcare

import (
    "net/http"
    "net/http/httptest"
    "path/filepath"
    "testing"
    "time"

    "gorm.io/driver/sqlite"
    "gorm.io/gorm"
    gormlogger "gorm.io/gorm/logger"
)

func setupPrescriptionDB(t *testing.T) *gorm.DB {
    t.Helper()
    db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "presc.db")), &gorm.Config{Logger: gormlogger.Default.LogMode(gormlogger.Silent)})
    if err != nil {
        t.Fatalf("open db: %v", err)
    }
    if err := db.AutoMigrate(&PrescriptionImport{}, &SystemAIConfig{}); err != nil {
        t.Fatalf("migrate: %v", err)
    }
    return db
}

func newRecognizingImport(t *testing.T, db *gorm.DB) PrescriptionImport {
    t.Helper()
    now := nowUTC()
    imp := PrescriptionImport{BookID: 1, CreatedBy: 1, Status: PrescriptionRecognizing, Kind: RecognizeKindPrescription,
        ImageMIME: "image/png", ImageSize: 4, ImageSHA256: "hash", CreatedAt: now, UpdatedAt: now}
    if err := db.Create(&imp).Error; err != nil {
        t.Fatalf("create import: %v", err)
    }
    return imp
}

func waitPrescriptionStatus(t *testing.T, db *gorm.DB, id uint, want string) PrescriptionImport {
    t.Helper()
    deadline := time.Now().Add(8 * time.Second)
    for time.Now().Before(deadline) {
        var got PrescriptionImport
        if err := db.First(&got, id).Error; err != nil {
            t.Fatalf("read import: %v", err)
        }
        if got.Status == want {
            return got
        }
        time.Sleep(50 * time.Millisecond)
    }
    var last PrescriptionImport
    db.First(&last, id)
    t.Fatalf("状态未变成 %s，当前 %s（错误码 %q）", want, last.Status, last.ErrorCode)
    return last
}

// 进程重启遗留的「识别中」必须收尾成可重试的失败，其余状态不动。
func TestReclaimInterruptedPrescriptions(t *testing.T) {
    db := setupPrescriptionDB(t)
    stuck := newRecognizingImport(t, db)
    now := nowUTC()
    done := PrescriptionImport{BookID: 1, CreatedBy: 1, Status: PrescriptionRecognized, Kind: RecognizeKindPrescription, CreatedAt: now, UpdatedAt: now}
    failed := PrescriptionImport{BookID: 1, CreatedBy: 1, Status: PrescriptionFailed, Kind: RecognizeKindPrescription, ErrorCode: "AI_RECOGNIZE_FAILED", CreatedAt: now, UpdatedAt: now}
    for _, p := range []*PrescriptionImport{&done, &failed} {
        if err := db.Create(p).Error; err != nil {
            t.Fatalf("create import: %v", err)
        }
    }

    if n := reclaimInterruptedPrescriptions(db); n != 1 {
        t.Fatalf("收尾数量 = %d, want 1", n)
    }
    var gotStuck PrescriptionImport
    if err := db.First(&gotStuck, stuck.ID).Error; err != nil {
        t.Fatalf("read stuck: %v", err)
    }
    if gotStuck.Status != PrescriptionFailed || gotStuck.ErrorCode != "AI_RECOGNIZE_INTERRUPTED" {
        t.Fatalf("中断记录未收尾: status=%s code=%s", gotStuck.Status, gotStuck.ErrorCode)
    }
    var gotDone, gotFailed PrescriptionImport
    db.First(&gotDone, done.ID)
    db.First(&gotFailed, failed.ID)
    if gotDone.Status != PrescriptionRecognized {
        t.Fatalf("已完成记录被改动: %s", gotDone.Status)
    }
    if gotFailed.ErrorCode != "AI_RECOGNIZE_FAILED" {
        t.Fatalf("原失败原因被覆盖: %s", gotFailed.ErrorCode)
    }
    // 幂等：再跑一次不应再改动任何记录。
    if n := reclaimInterruptedPrescriptions(db); n != 0 {
        t.Fatalf("重复收尾数量 = %d, want 0", n)
    }
}

// 队列满时快速失败并写明原因，不再无上限堆 goroutine 与图片内存。
func TestPrescriptionQueueRejectsWhenFull(t *testing.T) {
    db := setupPrescriptionDB(t)
    saved := prescriptionQueue
    defer func() { prescriptionQueue = saved }()
    // 换一条没有 worker 消费的临时队列，容量 1。
    prescriptionQueue = make(chan prescriptionTask, 1)

    first := newRecognizingImport(t, db)
    runPrescriptionRecognition(db, SystemAIConfig{}, first, []byte("img"), "image/png", RecognizeKindPrescription)
    if len(prescriptionQueue) != 1 {
        t.Fatalf("首个任务未入队: len=%d", len(prescriptionQueue))
    }

    second := newRecognizingImport(t, db)
    runPrescriptionRecognition(db, SystemAIConfig{}, second, []byte("img"), "image/png", RecognizeKindPrescription)
    var got PrescriptionImport
    if err := db.First(&got, second.ID).Error; err != nil {
        t.Fatalf("read second: %v", err)
    }
    if got.Status != PrescriptionFailed || got.ErrorCode != "AI_QUEUE_FULL" {
        t.Fatalf("队列满时未快速失败: status=%s code=%s", got.Status, got.ErrorCode)
    }
    if got.ErrorMessage == "" {
        t.Fatalf("队列满时没有给用户可读原因")
    }
}

// worker 真的会消费队列，并把外部 AI 的失败回写到记录上。
func TestPrescriptionWorkerMarksFailure(t *testing.T) {
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusInternalServerError)
    }))
    defer srv.Close()

    db := setupPrescriptionDB(t)
    startPrescriptionWorkers(db)
    imp := newRecognizingImport(t, db)
    cfg := SystemAIConfig{Enabled: true, BaseURL: srv.URL, Model: "test-model", TimeoutSeconds: 5}
    runPrescriptionRecognition(db, cfg, imp, []byte("fake-image"), "image/png", RecognizeKindPrescription)

    got := waitPrescriptionStatus(t, db, imp.ID, PrescriptionFailed)
    if got.ErrorCode == "" {
        t.Fatalf("失败记录缺少错误码")
    }
}
