package medcare

import (
    "context"
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "path/filepath"
    "testing"
    "time"

    "github.com/gin-gonic/gin"
    "gorm.io/driver/sqlite"
    "gorm.io/gorm"
    gormlogger "gorm.io/gorm/logger"

    "smallgo/server/database"
)

// countingLogger 统计真正发出的 SQL 条数，用来锁住「查询数不随药品条数增长」。
type countingLogger struct {
    gormlogger.Interface
    count *int
}

func (l countingLogger) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
    *l.count++
    l.Interface.Trace(ctx, begin, fc, err)
}

func setupMedicineListDB(t *testing.T) (*gorm.DB, *int) {
    t.Helper()
    queries := 0
    db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "list.db")), &gorm.Config{
        Logger: countingLogger{Interface: gormlogger.Default.LogMode(gormlogger.Silent), count: &queries},
    })
    if err != nil {
        t.Fatalf("open db: %v", err)
    }
    if err := db.AutoMigrate(&Medicine{}, &MedicineBatch{}, &Plan{}, &MedicationBook{}, &BookMember{}, &database.User{}); err != nil {
        t.Fatalf("migrate: %v", err)
    }
    now := nowUTC()
    book := MedicationBook{Name: "测试用药本", CareRecipientName: "用药人", Timezone: BeijingTimezone, OwnerUserID: 1, CreatedAt: now, UpdatedAt: now}
    if err := db.Create(&book).Error; err != nil {
        t.Fatalf("create book: %v", err)
    }
    alice := database.User{Username: "alice", Password: "x", AuthVersion: 1}
    if err := db.Create(&alice).Error; err != nil {
        t.Fatalf("create user: %v", err)
    }
    return db, &queries
}

// seedMedicines 造 n 条药品，每条都带批次、在用计划与操作人，确保派生数据都有活可干。
func seedMedicines(t *testing.T, db *gorm.DB, n int) {
    t.Helper()
    now := nowUTC()
    today := todayIn(BeijingTimezone)
    for i := 0; i < n; i++ {
        m := Medicine{
            BookID: 1, Name: "药品" + idParam(uint(i+1)), Category: CatOral, Unit: "片",
            StockQuantity: 50, LowStockThreshold: 10, ExpiryWarnDays: 30, Version: 1,
            CreatedBy: 1, UpdatedBy: 1, CreatedAt: now, UpdatedAt: now.Add(time.Duration(i) * time.Second),
        }
        if err := db.Create(&m).Error; err != nil {
            t.Fatalf("create medicine: %v", err)
        }
        batch := MedicineBatch{BookID: 1, MedicineID: m.ID, QuantityInitial: 50, QuantityRemaining: 50, ExpiryDate: addDays(today, 200), ReceivedAt: now}
        if err := db.Create(&batch).Error; err != nil {
            t.Fatalf("create batch: %v", err)
        }
        plan := Plan{BookID: 1, MedicineID: m.ID, Type: CatOral, Status: PlanActive, StartDate: today, Timezone: BeijingTimezone, CreatedAt: now, UpdatedAt: now}
        if err := db.Create(&plan).Error; err != nil {
            t.Fatalf("create plan: %v", err)
        }
    }
}

func listMedicinesRequest(t *testing.T, db *gorm.DB, query string) *httptest.ResponseRecorder {
    t.Helper()
    gin.SetMode(gin.TestMode)
    r := gin.New()
    r.GET("/medicines", func(c *gin.Context) {
        c.Set("userID", uint(1))
        c.Set("bookID", uint(1))
        c.Set("bookRole", RoleOwner)
        handleListMedicines(db)(c)
    })
    w := httptest.NewRecorder()
    r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/medicines?"+query, nil))
    return w
}

func decodeMedicinePage(t *testing.T, w *httptest.ResponseRecorder) ([]MedicineDTO, int64) {
    t.Helper()
    if w.Code != http.StatusOK {
        t.Fatalf("列表 status = %d, body=%s", w.Code, w.Body.String())
    }
    var payload struct {
        Code int `json:"code"`
        Data struct {
            Items []MedicineDTO `json:"items"`
            Total int64         `json:"total"`
        } `json:"data"`
    }
    if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
        t.Fatalf("decode: %v", err)
    }
    return payload.Data.Items, payload.Data.Total
}

// 一页药品的查询数必须是常数：3 条与 12 条发出的 SQL 条数完全一致。
func TestMedicineListQueryCountStaysFlat(t *testing.T) {
    counts := map[int]int{}
    for _, size := range []int{3, 12} {
        db, queries := setupMedicineListDB(t)
        seedMedicines(t, db, size)
        *queries = 0
        items, total := decodeMedicinePage(t, listMedicinesRequest(t, db, "pageSize=100"))
        if len(items) != size || total != int64(size) {
            t.Fatalf("返回条数 = %d/%d, want %d", len(items), total, size)
        }
        counts[size] = *queries
    }
    t.Logf("药品列表 SQL 次数：3 条 %d 次，12 条 %d 次", counts[3], counts[12])
    if counts[3] != counts[12] {
        t.Fatalf("查询数随条数增长（N+1 未消除）: 3 条 %d 次, 12 条 %d 次", counts[3], counts[12])
    }
    // 书籍信息 + 计数 + 取页 + 三次批量预取，留一点余量。
    if counts[12] > 8 {
        t.Fatalf("查询数偏多: %d 次", counts[12])
    }
}

// 批量预取不能改变派生字段口径：库存状态、最近效期、在用计划数、操作人姓名。
func TestMedicineListDerivedFieldsUnchanged(t *testing.T) {
    db, _ := setupMedicineListDB(t)
    now := nowUTC()
    today := todayIn(BeijingTimezone)
    cases := []struct {
        name       string
        stock      float64
        threshold  float64
        expiry     string
        wantStatus string
    }{
        {name: "缺货药", stock: 0, threshold: 10, expiry: addDays(today, 200), wantStatus: "out"},
        {name: "过期药", stock: 50, threshold: 10, expiry: addDays(today, -1), wantStatus: "expired"},
        {name: "低库存药", stock: 5, threshold: 10, expiry: addDays(today, 200), wantStatus: "low"},
        {name: "近效期药", stock: 50, threshold: 10, expiry: addDays(today, 10), wantStatus: "expiring"},
        {name: "正常药", stock: 50, threshold: 10, expiry: addDays(today, 200), wantStatus: "ok"},
    }
    for _, tc := range cases {
        m := Medicine{BookID: 1, Name: tc.name, Category: CatOral, Unit: "片", StockQuantity: tc.stock,
            LowStockThreshold: tc.threshold, ExpiryWarnDays: 30, Version: 1, CreatedBy: 1, UpdatedBy: 1, CreatedAt: now, UpdatedAt: now}
        if err := db.Create(&m).Error; err != nil {
            t.Fatalf("create medicine: %v", err)
        }
        batch := MedicineBatch{BookID: 1, MedicineID: m.ID, QuantityInitial: 50, QuantityRemaining: tc.stock, ExpiryDate: tc.expiry, ReceivedAt: now}
        if err := db.Create(&batch).Error; err != nil {
            t.Fatalf("create batch: %v", err)
        }
    }
    // 给「正常药」挂两条在用计划与一条已结束计划，验证只数在用。
    var normal Medicine
    if err := db.Where("name = ?", "正常药").First(&normal).Error; err != nil {
        t.Fatalf("read medicine: %v", err)
    }
    for _, status := range []string{PlanActive, PlanPaused, PlanCompleted} {
        plan := Plan{BookID: 1, MedicineID: normal.ID, Type: CatOral, Status: status, StartDate: today, Timezone: BeijingTimezone, CreatedAt: now, UpdatedAt: now}
        if err := db.Create(&plan).Error; err != nil {
            t.Fatalf("create plan: %v", err)
        }
    }

    items, _ := decodeMedicinePage(t, listMedicinesRequest(t, db, "pageSize=100"))
    byName := make(map[string]MedicineDTO, len(items))
    for _, it := range items {
        byName[it.Name] = it
    }
    for _, tc := range cases {
        got, ok := byName[tc.name]
        if !ok {
            t.Fatalf("列表缺少 %s", tc.name)
        }
        if got.StockStatus != tc.wantStatus {
            t.Fatalf("%s 库存状态 = %s, want %s", tc.name, got.StockStatus, tc.wantStatus)
        }
        wantExpiry := tc.expiry
        if tc.stock <= 0 {
            wantExpiry = "" // 批次没有剩余数量，不参与最近效期
        }
        if got.NearestExpiry != wantExpiry {
            t.Fatalf("%s 最近效期 = %q, want %q", tc.name, got.NearestExpiry, wantExpiry)
        }
        if got.CreatedByName != "alice" || got.UpdatedByName != "alice" {
            t.Fatalf("%s 操作人姓名丢失: %q / %q", tc.name, got.CreatedByName, got.UpdatedByName)
        }
    }
    if got := byName["正常药"]; got.ActivePlanCount != 2 {
        t.Fatalf("在用计划数 = %d, want 2", got.ActivePlanCount)
    }
}

// 库存统计的最近效期同样一次查回，不再按药品逐条查。
func TestStatsStockNearestExpiryBatched(t *testing.T) {
    db, queries := setupMedicineListDB(t)
    seedMedicines(t, db, 8)
    gin.SetMode(gin.TestMode)
    r := gin.New()
    r.GET("/stats/stock", func(c *gin.Context) {
        c.Set("userID", uint(1))
        c.Set("bookID", uint(1))
        c.Set("bookRole", RoleOwner)
        handleStatsStock(db)(c)
    })
    *queries = 0
    w := httptest.NewRecorder()
    r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/stats/stock", nil))
    if w.Code != http.StatusOK {
        t.Fatalf("库存统计 status = %d, body=%s", w.Code, w.Body.String())
    }
    var payload struct {
        Data []struct {
            NearestExpiry string `json:"nearest_expiry"`
        } `json:"data"`
    }
    if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
        t.Fatalf("decode: %v", err)
    }
    if len(payload.Data) != 8 {
        t.Fatalf("统计条数 = %d, want 8", len(payload.Data))
    }
    for _, row := range payload.Data {
        if row.NearestExpiry == "" {
            t.Fatalf("最近效期丢失: %s", w.Body.String())
        }
    }
    // 书籍信息 + 药品列表 + 一次批次聚合。
    if *queries > 4 {
        t.Fatalf("库存统计查询数偏多（N+1 未消除）: %d 次", *queries)
    }
}
