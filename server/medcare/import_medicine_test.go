package medcare

import (
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "path/filepath"
    "strings"
    "testing"

    "github.com/gin-gonic/gin"
    "gorm.io/driver/sqlite"
    "gorm.io/gorm"
    "gorm.io/gorm/logger"
)

// setupImportTestDB 用内存 SQLite 建一套最小表结构，覆盖「批量导入药品」用到的模型。
func setupImportTestDB(t *testing.T) *gorm.DB {
    t.Helper()
    db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "test.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
    if err != nil {
        t.Fatalf("open db: %v", err)
    }
    if err := db.AutoMigrate(&Medicine{}, &MedicineBatch{}, &StockMovement{}, &PrescriptionImport{}, &BookRevision{}, &MedicationBook{}, &BookMember{}); err != nil {
        t.Fatalf("migrate: %v", err)
    }
    return db
}

func importRequest(t *testing.T, db *gorm.DB, bookID uint, body string) (*httptest.ResponseRecorder, map[string]interface{}) {
    t.Helper()
    gin.SetMode(gin.TestMode)
    r := gin.New()
    r.POST("/import", func(c *gin.Context) {
        c.Set("bookID", bookID)
        c.Set("userID", uint(1))
        handleImportMedicines(db)(c)
    })
    w := httptest.NewRecorder()
    req := httptest.NewRequest(http.MethodPost, "/import", strings.NewReader(body))
    req.Header.Set("Content-Type", "application/json")
    r.ServeHTTP(w, req)
    var out map[string]interface{}
    _ = json.Unmarshal(w.Body.Bytes(), &out)
    return w, out
}

func TestImportMedicinesCreatesBatchAndStock(t *testing.T) {
    db := setupImportTestDB(t)
    body := `{"source":"receipt","items":[
        {"name":"氨氯地平片","spec":"5mg×28片","unit":"片","quantity":28,"expiry_date":"2028-05-01","manufacturer":"某某药业"},
        {"name":"阿托伐他汀钙片","spec":"20mg×7片","unit":"片","quantity":14}
    ]}`
    w, out := importRequest(t, db, 7, body)
    if w.Code != http.StatusOK {
        t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
    }
    data, _ := out["data"].(map[string]interface{})
    if got := data["count"]; got != float64(2) {
        t.Fatalf("count = %v, want 2", got)
    }
    var meds []Medicine
    db.Where("book_id = ?", 7).Find(&meds)
    if len(meds) != 2 {
        t.Fatalf("created %d medicines, want 2", len(meds))
    }
    var batches []MedicineBatch
    db.Where("book_id = ?", 7).Find(&batches)
    if len(batches) != 2 {
        t.Fatalf("created %d batches, want 2", len(batches))
    }
    var m Medicine
    db.Where("book_id = ? AND name = ?", 7, "氨氯地平片").First(&m)
    if m.StockQuantity != 28 {
        t.Fatalf("stock = %v, want 28", m.StockQuantity)
    }
    if m.ExpiryWarnDays != 30 {
        t.Fatalf("default expiry warn days = %d, want 30", m.ExpiryWarnDays)
    }
}

func TestImportMedicinesRejectsBadInput(t *testing.T) {
    db := setupImportTestDB(t)
    cases := []struct{ name, body string }{
        {"empty", `{"items":[]}`},
        {"blank name", `{"items":[{"name":"  ","quantity":1}]}`},
        {"bad expiry", `{"items":[{"name":"药品","expiry_date":"2028/05/01"}]}`},
    }
    for _, c := range cases {
        w, _ := importRequest(t, db, 7, c.body)
        if w.Code != http.StatusBadRequest {
            t.Fatalf("%s: status = %d, want 400 (%s)", c.name, w.Code, w.Body.String())
        }
    }
    var count int64
    db.Model(&Medicine{}).Where("book_id = ?", 7).Count(&count)
    if count != 0 {
        t.Fatalf("bad input left %d medicines behind, transaction should roll back", count)
    }
}

func TestImportMedicinesRollsBackOnPartialFailure(t *testing.T) {
    db := setupImportTestDB(t)
    body := `{"items":[{"name":"好药","quantity":5},{"name":"坏药","expiry_date":"2028/05/01"}]}`
    w, _ := importRequest(t, db, 7, body)
    if w.Code != http.StatusBadRequest {
        t.Fatalf("status = %d, want 400", w.Code)
    }
    var count int64
    db.Model(&Medicine{}).Where("book_id = ?", 7).Count(&count)
    if count != 0 {
        t.Fatalf("partial failure left %d medicines, want 0", count)
    }
}
