package medcare

import (
    "bytes"
    "encoding/json"
    "mime/multipart"
    "net/http"
    "net/http/httptest"
    "os"
    "path/filepath"
    "strings"
    "testing"

    "github.com/gin-gonic/gin"
    "gorm.io/driver/sqlite"
    "gorm.io/gorm"
    gormlogger "gorm.io/gorm/logger"

    "smallgo/server/backup"
    "smallgo/server/config"
    "smallgo/server/database"
)

// setupUploadTest 准备临时数据目录与最小数据库，返回备份目录。
func setupUploadTest(t *testing.T) (*gorm.DB, string) {
    t.Helper()
    dir := t.TempDir()
    old := config.C.DataDir
    config.C.DataDir = dir
    t.Cleanup(func() { config.C.DataDir = old })

    db, err := gorm.Open(sqlite.Open(filepath.Join(dir, "main.db")), &gorm.Config{Logger: gormlogger.Default.LogMode(gormlogger.Silent)})
    if err != nil {
        t.Fatalf("open db: %v", err)
    }
    if err := db.AutoMigrate(&database.AuditLog{}); err != nil {
        t.Fatalf("migrate: %v", err)
    }
    return db, backup.Dir(dir)
}

// makeBackupFile 造一个合法的备份文件：带 users 表的 SQLite 库。
func makeBackupFile(t *testing.T, withUsers bool) string {
    t.Helper()
    path := filepath.Join(t.TempDir(), "src.db")
    db, err := gorm.Open(sqlite.Open(path), &gorm.Config{Logger: gormlogger.Default.LogMode(gormlogger.Silent)})
    if err != nil {
        t.Fatalf("open src: %v", err)
    }
    if withUsers {
        if err := db.Exec("CREATE TABLE users (id integer primary key, username text)").Error; err != nil {
            t.Fatalf("create users: %v", err)
        }
    } else {
        if err := db.Exec("CREATE TABLE other (id integer primary key)").Error; err != nil {
            t.Fatalf("create other: %v", err)
        }
    }
    sqlDB, _ := db.DB()
    sqlDB.Close()
    return path
}

func uploadRequest(t *testing.T, db *gorm.DB, filename string, content []byte) (*httptest.ResponseRecorder, map[string]interface{}) {
    t.Helper()
    gin.SetMode(gin.TestMode)
    var body bytes.Buffer
    mw := multipart.NewWriter(&body)
    part, err := mw.CreateFormFile("file", filename)
    if err != nil {
        t.Fatalf("form file: %v", err)
    }
    part.Write(content)
    mw.Close()

    r := gin.New()
    r.POST("/backups/upload", func(c *gin.Context) {
        c.Set("userID", uint(1))
        handleUploadBackup(db)(c)
    })
    w := httptest.NewRecorder()
    req := httptest.NewRequest(http.MethodPost, "/backups/upload", &body)
    req.Header.Set("Content-Type", mw.FormDataContentType())
    r.ServeHTTP(w, req)
    var out map[string]interface{}
    _ = json.Unmarshal(w.Body.Bytes(), &out)
    return w, out
}

func readBytes(t *testing.T, path string) []byte {
    t.Helper()
    b, err := os.ReadFile(path)
    if err != nil {
        t.Fatalf("read %s: %v", path, err)
    }
    return b
}

// 合法备份上传后进入备份列表，且不留临时残余。
func TestUploadBackupAcceptsValidBackup(t *testing.T) {
    db, dir := setupUploadTest(t)
    src := makeBackupFile(t, true)
    // 原文件名带中文与空格：只进审计，不落盘。
    w, out := uploadRequest(t, db, "用药管理 备份 2026.db", readBytes(t, src))
    if w.Code != http.StatusOK {
        t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
    }
    data, _ := out["data"].(map[string]interface{})
    name, _ := data["name"].(string)
    if !strings.HasPrefix(name, "backup_") || !strings.HasSuffix(name, ".db") {
        t.Fatalf("服务端未按规则生成备份名: %q", name)
    }
    if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
        t.Fatalf("备份未落盘: %v", err)
    }
    items, err := backup.List(config.C.DataDir)
    if err != nil || len(items) != 1 || items[0].Name != name {
        t.Fatalf("备份列表不符: items=%v err=%v", items, err)
    }
    assertNoTempLeft(t, dir)
}

// 非 .db 后缀直接拒绝。
func TestUploadBackupRejectsWrongExtension(t *testing.T) {
    db, dir := setupUploadTest(t)
    src := makeBackupFile(t, true)
    w, _ := uploadRequest(t, db, "backup.txt", readBytes(t, src))
    if w.Code != http.StatusBadRequest {
        t.Fatalf("status = %d, want 400", w.Code)
    }
    assertNoTempLeft(t, dir)
}

// 后缀对但内容不是 SQLite：拒绝，且不留临时文件。
func TestUploadBackupRejectsGarbage(t *testing.T) {
    db, dir := setupUploadTest(t)
    w, _ := uploadRequest(t, db, "junk.db", []byte("这不是数据库，只是一段文本"))
    if w.Code != http.StatusBadRequest {
        t.Fatalf("status = %d, want 400", w.Code)
    }
    assertNoTempLeft(t, dir)
}

// 是 SQLite 但没有 users 表：不是本应用的库，拒绝。
func TestUploadBackupRejectsForeignDatabase(t *testing.T) {
    db, dir := setupUploadTest(t)
    src := makeBackupFile(t, false)
    w, out := uploadRequest(t, db, "foreign.db", readBytes(t, src))
    if w.Code != http.StatusBadRequest {
        t.Fatalf("status = %d, want 400, body=%s", w.Code, w.Body.String())
    }
    if msg, _ := out["message"].(string); !strings.Contains(msg, "users") {
        t.Fatalf("错误信息应点明缺 users 表: %q", msg)
    }
    assertNoTempLeft(t, dir)
}

// 缺少 file 字段。
func TestUploadBackupRejectsMissingFile(t *testing.T) {
    db, dir := setupUploadTest(t)
    gin.SetMode(gin.TestMode)
    r := gin.New()
    r.POST("/backups/upload", handleUploadBackup(db))
    w := httptest.NewRecorder()
    req := httptest.NewRequest(http.MethodPost, "/backups/upload", strings.NewReader(""))
    req.Header.Set("Content-Type", "multipart/form-data; boundary=xxx")
    r.ServeHTTP(w, req)
    if w.Code != http.StatusBadRequest {
        t.Fatalf("status = %d, want 400", w.Code)
    }
    assertNoTempLeft(t, dir)
}

func assertNoTempLeft(t *testing.T, dir string) {
    t.Helper()
    entries, err := os.ReadDir(dir)
    if err != nil {
        t.Fatalf("read dir: %v", err)
    }
    for _, e := range entries {
        if strings.HasPrefix(e.Name(), ".upload-") {
            t.Fatalf("残留临时文件: %s", e.Name())
        }
    }
}
