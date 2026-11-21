// 本地上传备份：把浏览器上传的 .db 备份文件落进备份目录，之后走与列表备份
// 完全相同的「恢复」流程（框架 backup.Restore：恢复前快照 → 原子替换 → 重启）。
//
// 放在实例层：框架 backup 包要与 smallgo 基线保持一致、不做改动，上传接口经
// 业务 App 的 SetupAdmin 挂到同一个 adminGroup（POST /api/backups/upload）。
//
// 安全要点：
//  1. 仅管理员（adminGroup）；请求体用 http.MaxBytesReader 兜底
//  2. 只收 .db 后缀；先落到不匹配备份名规则的临时文件，校验通过后才改成正式名
//     ——半截文件 / 校验失败的文件不会混进备份列表，也不可能被恢复
//  3. verifyBackupFile：SQLite 文件头 + integrity_check + 必须含 users 表，
//     拒绝把任意 SQLite 文件当备份
//  4. 正式名由服务端生成（backup_YYYYMMDD_HHMMSS.db，同秒冲突追加 -N），与框架
//     backup.Create 同规则，天然通过列表/下载/恢复的文件名校验，无路径穿越面；
//     原文件名只记进审计日志，不落盘
//
// 上传本身不改变当前数据：真正恢复仍走列表上的「恢复」按钮，完整复用既有的
// 确认弹窗与恢复前快照，一条代码路径两种入口。
package medcare

import (
    "fmt"
    "io"
    "net/http"
    "os"
    "path/filepath"
    "strings"
    "time"

    "github.com/gin-gonic/gin"
    "gorm.io/driver/sqlite"
    "gorm.io/gorm"
    gormlogger "gorm.io/gorm/logger"

    "smallgo/server/audit"
    "smallgo/server/backup"
    "smallgo/server/config"
    "smallgo/server/logger"
    "smallgo/server/response"
)

// maxBackupUploadBytes 单个上传备份的大小上限。正常备份远小于此，上限只用于
// 兜底防御（防把接口当网盘、防内存被拖爆）。
const maxBackupUploadBytes = 256 << 20

// sqliteFileHeader 是 SQLite 数据库文件的固定文件头。
const sqliteFileHeader = "SQLite format 3\x00"

// handleUploadBackup 接收浏览器上传的备份文件，校验通过后存为一份备份。
func handleUploadBackup(db *gorm.DB) gin.HandlerFunc {
    return func(c *gin.Context) {
        c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBackupUploadBytes)
        fileHeader, err := c.FormFile("file")
        if err != nil {
            response.ErrorBadRequest(c, "请选择要上传的备份文件（.db）")
            return
        }
        if fileHeader.Size > maxBackupUploadBytes {
            response.ErrorBadRequest(c, "备份文件过大（上限 256MB）")
            return
        }
        if ext := strings.ToLower(filepath.Ext(fileHeader.Filename)); ext != ".db" {
            response.ErrorBadRequest(c, "只支持 .db 备份文件")
            return
        }

        dir := backup.Dir(config.C.DataDir)
        tmp, err := os.CreateTemp(dir, ".upload-*.tmp")
        if err != nil {
            logger.Error("medcare: 创建上传临时文件失败: %v", err)
            response.ErrorInternal(c, "保存上传文件失败")
            return
        }
        tmpName := tmp.Name()
        defer os.Remove(tmpName) // 成功改名后路径已不存在，此调用是空操作

        src, err := fileHeader.Open()
        if err != nil {
            tmp.Close()
            response.ErrorBadRequest(c, "读取上传文件失败")
            return
        }
        _, copyErr := io.Copy(tmp, src)
        src.Close()
        closeErr := tmp.Close()
        if copyErr != nil || closeErr != nil {
            logger.Error("medcare: 写入上传备份失败: copy=%v close=%v", copyErr, closeErr)
            response.ErrorInternal(c, "保存上传文件失败")
            return
        }

        if err := verifyBackupFile(tmpName); err != nil {
            response.ErrorBadRequest(c, "备份校验失败："+err.Error())
            return
        }

        name := uniqueBackupName(dir)
        if err := os.Rename(tmpName, filepath.Join(dir, name)); err != nil {
            logger.Error("medcare: 上传备份转正失败: %v", err)
            response.ErrorInternal(c, "保存备份失败")
            return
        }

        logger.Info("medcare: 上传备份 %s（原文件名 %s，%d 字节）", name, fileHeader.Filename, fileHeader.Size)
        audit.Log(db, c, "backup_upload", "backup", 0,
            fmt.Sprintf("%s（原文件 %s）", name, fileHeader.Filename))
        response.Success(c, gin.H{"name": name})
    }
}

// verifyBackupFile 校验备份可用：文件头是 SQLite、integrity_check 通过、
// 且包含 users 表（防止把任意 SQLite 文件丢进目录就当备份恢复）。
func verifyBackupFile(path string) error {
    f, err := os.Open(path)
    if err != nil {
        return fmt.Errorf("打开备份失败: %w", err)
    }
    defer f.Close()
    head := make([]byte, len(sqliteFileHeader))
    if _, err := io.ReadFull(f, head); err != nil {
        return fmt.Errorf("读取备份文件头失败: %w", err)
    }
    if string(head) != sqliteFileHeader {
        return fmt.Errorf("不是有效的 SQLite 数据库文件")
    }

    probe, err := gorm.Open(sqlite.Open(path), &gorm.Config{
        Logger: gormlogger.Default.LogMode(gormlogger.Silent),
    })
    if err != nil {
        return fmt.Errorf("打开备份失败: %w", err)
    }
    sqlDB, err := probe.DB()
    if err != nil {
        return fmt.Errorf("获取备份连接失败: %w", err)
    }
    defer sqlDB.Close()

    rows, err := sqlDB.Query("PRAGMA integrity_check")
    if err != nil {
        return fmt.Errorf("完整性检查失败: %w", err)
    }
    var problems []string
    for rows.Next() {
        var s string
        if err := rows.Scan(&s); err != nil {
            rows.Close()
            return fmt.Errorf("读取完整性检查结果失败: %w", err)
        }
        problems = append(problems, s)
    }
    rows.Close()
    if len(problems) == 0 || problems[0] != "ok" {
        return fmt.Errorf("完整性检查未通过: %s", strings.Join(problems, "; "))
    }

    var n int
    if err := sqlDB.QueryRow("SELECT count(*) FROM sqlite_master WHERE type='table' AND name='users'").Scan(&n); err != nil {
        return fmt.Errorf("读取备份表结构失败: %w", err)
    }
    if n == 0 {
        return fmt.Errorf("备份里没有 users 表，不是本应用的数据库")
    }
    return nil
}

// uniqueBackupName 生成 backup_YYYYMMDD_HHMMSS.db，同秒冲突追加 -N 序号，
// 与框架 backup.Create 的命名规则一致，保证能通过列表/恢复的文件名校验。
func uniqueBackupName(dir string) string {
    base := "backup_" + time.Now().Format("20060102_150405")
    target := base + ".db"
    for i := 1; i <= 99; i++ {
        if _, err := os.Stat(filepath.Join(dir, target)); os.IsNotExist(err) {
            return target
        }
        target = fmt.Sprintf("%s-%d.db", base, i)
    }
    return target
}
