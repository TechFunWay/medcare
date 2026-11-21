package medcare

import (
    "context"
    "encoding/json"
    "net/http"
    "sync"
    "time"

    "smallgo/server/logger"

    "gorm.io/gorm"
)

// 处方识别要调外部 AI，单份可能 30–90 秒。原先每次上传都新开一个 goroutine，
// 并发没有上限；进程重启后 recognizing 记录也没人收尾，会永久卡在「识别中」。
// 这里改成固定 worker + 有界队列，并在启动时把中断的记录收尾成可重试的失败。
const (
    prescriptionWorkers   = 2
    prescriptionQueueSize = 16
)

type prescriptionTask struct {
    db    *gorm.DB
    cfg   SystemAIConfig
    imp   PrescriptionImport
    image []byte
    mime  string
    kind  string
}

var (
    prescriptionQueue chan prescriptionTask
    prescriptionOnce  sync.Once
)

// startPrescriptionWorkers 启动固定数量的识别 worker，并收尾上次进程遗留的记录。
// 在路由装配阶段调用一次；worker 只启动一次，收尾动作可重复执行且幂等。
func startPrescriptionWorkers(db *gorm.DB) {
    prescriptionOnce.Do(func() {
        prescriptionQueue = make(chan prescriptionTask, prescriptionQueueSize)
        for i := 0; i < prescriptionWorkers; i++ {
            go func() {
                for task := range prescriptionQueue {
                    processPrescription(task)
                }
            }()
        }
    })
    reclaimInterruptedPrescriptions(db)
}

// enqueuePrescription 入队；队列满时不阻塞请求，由调用方快速失败。
func enqueuePrescription(task prescriptionTask) bool {
    if prescriptionQueue == nil {
        return false
    }
    select {
    case prescriptionQueue <- task:
        return true
    default:
        return false
    }
}

// processPrescription 是单个识别任务的执行体：超时受控，panic 也要把记录收尾。
func processPrescription(task prescriptionTask) {
    importID := task.imp.ID
    defer func() {
        if r := recover(); r != nil {
            logger.Error("medcare: 处方识别异常退出: %v", r)
            markPrescriptionFailed(task.db, importID, "AI_RECOGNIZE_PANIC", "识别过程异常，请重试")
        }
    }()
    ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
    defer cancel()
    draft, err := recognizePrescription(ctx, task.db, task.cfg, task.image, task.mime, task.kind)
    if err != nil {
        markPrescriptionFailed(task.db, importID, "AI_RECOGNIZE_FAILED", trimRunes(err.Error(), 300))
        return
    }
    parsed, _ := json.Marshal(draft)
    confidence := 0.0
    for _, item := range draft.Items {
        confidence += item.Confidence
    }
    if len(draft.Items) > 0 {
        confidence /= float64(len(draft.Items))
    }
    if err := task.db.Model(&PrescriptionImport{}).Where("id = ?", importID).Updates(map[string]interface{}{
        "status": PrescriptionRecognized, "parsed_json": string(parsed), "confidence": confidence,
        "ocr_text": draft.OCRText,
        "ocr_provider": draft.OCRProvider,
        "ai_prompt_tokens": draft.AIPromptTokens,
        "ai_completion_tokens": draft.AICompletionTokens,
        "ocr_tokens": draft.OCRTokens,
        "error_code": "", "error_message": "", "updated_at": nowUTC(),
    }).Error; err != nil {
        logger.Error("medcare: 回写识别结果失败 import=%d: %v", importID, err)
    }
}

// markPrescriptionFailed 统一收尾失败状态，写库失败也要留日志。
func markPrescriptionFailed(db *gorm.DB, importID uint, code, message string) {
    if err := db.Model(&PrescriptionImport{}).Where("id = ?", importID).Updates(map[string]interface{}{
        "status": PrescriptionFailed, "error_code": code,
        "error_message": trimRunes(message, 300), "updated_at": nowUTC(),
    }).Error; err != nil {
        logger.Error("medcare: 回写识别失败状态失败 import=%d: %v", importID, err)
    }
}

// reclaimInterruptedPrescriptions 处理上次进程遗留的「识别中」记录。
// 识别是纯内存任务，进程一停就没人继续；不收尾的话前端会一直轮询不到结果。
// 存过原图的记录直接解密续跑（升级 / 重启不再丢识别，2026-11-16 用户反馈补）；
// 没存原图或 AI 已关的才收尾成可重试的失败，提示重新上传。
func reclaimInterruptedPrescriptions(db *gorm.DB) int64 {
    var rows []PrescriptionImport
    if err := db.Where("status = ?", PrescriptionRecognizing).Find(&rows).Error; err != nil {
        logger.Error("medcare: 读取中断的处方识别失败: %v", err)
        return 0
    }
    if len(rows) == 0 {
        return 0
    }
    ensureSystemConfigs(db)
    var cfg SystemAIConfig
    db.First(&cfg, 1)
    var resumed, failed int64
    for _, p := range rows {
        if cfg.Enabled && len(p.ImageEncrypted) > 0 {
            if image, err := decryptBytes(db, p.ImageEncrypted); err == nil {
                mime := p.ImageMIME
                if mime == "" {
                    mime = http.DetectContentType(image)
                }
                if enqueuePrescription(prescriptionTask{db: db, cfg: cfg, imp: p, image: image, mime: mime, kind: p.Kind}) {
                    resumed++
                    continue
                }
            }
        }
        markPrescriptionFailed(db, p.ID, "AI_RECOGNIZE_INTERRUPTED", "应用重启导致识别中断，请重试")
        failed++
    }
    logger.Info("medcare: 重启收尾：自动续跑 %d 条、标记失败 %d 条中断的处方识别", resumed, failed)
    return int64(len(rows))
}
