package medcare

import (
    "crypto/sha256"
    "encoding/hex"
    "encoding/json"
    "errors"
    "fmt"
    "io"
    "net/http"
    "strconv"
    "strings"
    "time"

    "smallgo/server/audit"
    "smallgo/server/response"

    "github.com/gin-gonic/gin"
    "gorm.io/gorm"
)

type PrescriptionDTO struct {
    ID           uint       `json:"id"`
    Status       string     `json:"status"`
    Kind         string     `json:"kind"`
    Intent       string     `json:"intent"`
    Hospital     string     `json:"hospital"`
    Doctor       string     `json:"doctor"`
    Date         string     `json:"prescription_date"`
    ItemCount    int        `json:"item_count"`
    Confidence   float64    `json:"confidence"`
    HasImage     bool       `json:"has_image"`
    ErrorCode    string     `json:"error_code,omitempty"`
    ErrorMessage string     `json:"error_message,omitempty"`
    CreatedAt    time.Time  `json:"created_at"`
    ConfirmedAt  *time.Time `json:"confirmed_at,omitempty"`
    // 本次识别用了哪家 OCR（空=直读图片）与 token 用量，供记录页展示。
    OCRProvider        string `json:"ocr_provider,omitempty"`
    OCRTokens          int    `json:"ocr_tokens"`
    AIPromptTokens     int    `json:"ai_prompt_tokens"`
    AICompletionTokens int    `json:"ai_completion_tokens"`
}

func prescriptionMeta(raw string) prescriptionDraft {
    var d prescriptionDraft
    _ = json.Unmarshal([]byte(raw), &d)
    return d
}

func prescriptionToDTO(p PrescriptionImport) PrescriptionDTO {
    meta := prescriptionMeta(p.ParsedJSON)
    kind := p.Kind
    if kind == "" {
        kind = RecognizeKindPrescription
    }
    return PrescriptionDTO{ID: p.ID, Status: p.Status, Kind: kind, Intent: p.Intent, Hospital: meta.Hospital, Doctor: meta.Doctor, Date: meta.PrescriptionDate, ItemCount: len(meta.Items), Confidence: p.Confidence, HasImage: len(p.ImageEncrypted) > 0, ErrorCode: p.ErrorCode, ErrorMessage: p.ErrorMessage, CreatedAt: p.CreatedAt, ConfirmedAt: p.ConfirmedAt,
        OCRProvider: p.OCRProvider, OCRTokens: p.OCRTokens, AIPromptTokens: p.AIPromptTokens, AICompletionTokens: p.AICompletionTokens}
}

// normalizeRecognizeKind 收敛前端传来的单据类型；未知值按处方笺处理，保持老行为。
// normalizeRecognizeIntent 收敛识别用途：archive 只建档 / stock 入库 / plan 生成计划，其余留空。
func normalizeRecognizeIntent(intent string) string {
    switch strings.TrimSpace(intent) {
    case "archive", "stock", "plan":
        return strings.TrimSpace(intent)
    }
    return ""
}

func normalizeRecognizeKind(kind string) string {
    switch strings.TrimSpace(kind) {
    case RecognizeKindBox:
        return RecognizeKindBox
    case RecognizeKindReceipt:
        return RecognizeKindReceipt
    default:
        return RecognizeKindPrescription
    }
}

func handleRecognizePrescription(db *gorm.DB) gin.HandlerFunc {
    return func(c *gin.Context) {
        ensureSystemConfigs(db)
        var cfg SystemAIConfig
        db.First(&cfg, 1)
        if !cfg.Enabled || cfg.BaseURL == "" || cfg.Model == "" {
            response.Error(c, http.StatusServiceUnavailable, 2011, "AI 识别未配置，请手动录入或联系管理员")
            return
        }
        maxBytes := int64(cfg.MaxImageMB) * 1024 * 1024
        if maxBytes <= 0 {
            maxBytes = 10 * 1024 * 1024
        }
        c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes+2*1024*1024)
        if err := c.Request.ParseMultipartForm(maxBytes + 2*1024*1024); err != nil {
            response.ErrorBadRequest(c, "图片过大或上传失败")
            return
        }
        file, header, err := c.Request.FormFile("file")
        if err != nil {
            response.ErrorBadRequest(c, "请选择处方图片")
            return
        }
        defer file.Close()
        if header.Size > maxBytes {
            response.ErrorBadRequest(c, "图片不能超过 "+strconv.Itoa(cfg.MaxImageMB)+"MB")
            return
        }
        image, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
        if err != nil || int64(len(image)) > maxBytes {
            response.ErrorBadRequest(c, "图片读取失败或过大")
            return
        }
        mime := http.DetectContentType(image)
        if mime != "image/jpeg" && mime != "image/png" && mime != "image/webp" {
            response.ErrorBadRequest(c, "仅支持 JPG、PNG、WebP 图片")
            return
        }
        saveImage := c.PostForm("save_image") == "true" || c.PostForm("save_image") == "1"
        kind := normalizeRecognizeKind(c.PostForm("kind"))
        intent := normalizeRecognizeIntent(c.PostForm("intent"))
        hash := sha256.Sum256(image)
        now := nowUTC()
        imp := PrescriptionImport{BookID: contextBookID(c), CreatedBy: currentUserID(c), Status: PrescriptionRecognizing, Kind: kind, Intent: intent, ProviderType: cfg.ProviderType, Model: cfg.Model, ImageMIME: mime, ImageSize: len(image), ImageSHA256: hex.EncodeToString(hash[:]), CreatedAt: now, UpdatedAt: now}
        if saveImage {
            encrypted, err := encryptBytes(db, image)
            if err != nil {
                response.ErrorInternal(c, "加密处方图片失败")
                return
            }
            imp.ImageEncrypted = encrypted
        }
        if err := db.Create(&imp).Error; err != nil {
            response.ErrorInternal(c, "保存识别记录失败")
            return
        }
        // 识别放到后台执行：飞牛统一网关对单个请求有超时限制，识别一份收费单
        // 可能要 30–90 秒，同步等待会被网关截断成 502。前端改为轮询导入状态。
        runPrescriptionRecognition(db, cfg, imp, image, mime, kind)
        audit.Log(db, c, "medcare_prescription_recognize", "prescription_import", imp.ID, "")
        response.Success(c, gin.H{"import_id": imp.ID, "status": PrescriptionRecognizing, "has_image": len(imp.ImageEncrypted) > 0})
    }
}

// runPrescriptionRecognition 把识别交给固定数量的后台 worker，避免长请求被网关
// 超时，也避免每次上传都新开一个无上限的 goroutine。队列满时立刻收尾成失败，
// 前端能看到明确原因并重试，不会留下永远「识别中」的记录。
func runPrescriptionRecognition(db *gorm.DB, cfg SystemAIConfig, imp PrescriptionImport, image []byte, mime, kind string) {
    if enqueuePrescription(prescriptionTask{db: db, cfg: cfg, imp: imp, image: image, mime: mime, kind: kind}) {
        return
    }
    markPrescriptionFailed(db, imp.ID, "AI_QUEUE_FULL", "识别排队已满，请稍后重试")
}

func handleListPrescriptions(db *gorm.DB) gin.HandlerFunc {
    return func(c *gin.Context) {
        var rows []PrescriptionImport
        // 已删除的识别记录不再出现在历史列表里（软删除保留行，便于审计）。
        if err := db.Where("book_id = ? AND status <> ?", contextBookID(c), PrescriptionDiscarded).Order("created_at DESC").Limit(100).Find(&rows).Error; err != nil {
            response.ErrorInternal(c, "读取处方记录失败")
            return
        }
        items := make([]PrescriptionDTO, 0, len(rows))
        for _, p := range rows {
            items = append(items, prescriptionToDTO(p))
        }
        response.Success(c, items)
    }
}

func handleGetPrescription(db *gorm.DB) gin.HandlerFunc {
    return func(c *gin.Context) {
        id, ok := parseUintParam(c, "id")
        if !ok {
            return
        }
        var p PrescriptionImport
        if err := db.Where("id = ? AND book_id = ?", id, contextBookID(c)).First(&p).Error; err != nil {
            response.ErrorNotFound(c, "处方记录不存在")
            return
        }
        dto := prescriptionToDTO(p)
        response.Success(c, gin.H{"import": dto, "status": p.Status, "draft": prescriptionMeta(p.ParsedJSON), "confirmed": prescriptionMeta(p.ConfirmedJSON)})
    }
}

func handlePrescriptionImage(db *gorm.DB) gin.HandlerFunc {
    return func(c *gin.Context) {
        id, ok := parseUintParam(c, "id")
        if !ok {
            return
        }
        var p PrescriptionImport
        if err := db.Where("id = ? AND book_id = ?", id, contextBookID(c)).First(&p).Error; err != nil {
            response.ErrorNotFound(c, "处方记录不存在")
            return
        }
        if len(p.ImageEncrypted) == 0 {
            response.ErrorNotFound(c, "原图未保存")
            return
        }
        raw, err := decryptBytes(db, p.ImageEncrypted)
        if err != nil {
            response.ErrorInternal(c, "读取处方图片失败")
            return
        }
        c.Data(http.StatusOK, p.ImageMIME, raw)
    }
}

func handleDeletePrescription(db *gorm.DB) gin.HandlerFunc {
    return func(c *gin.Context) {
        id, ok := parseUintParam(c, "id")
        if !ok {
            return
        }
        result := db.Model(&PrescriptionImport{}).Where("id = ? AND book_id = ?", id, contextBookID(c)).Updates(map[string]interface{}{"status": PrescriptionDiscarded, "image_encrypted": nil, "updated_at": nowUTC()})
        if result.RowsAffected == 0 {
            response.ErrorNotFound(c, "处方记录不存在")
            return
        }
        response.Success(c, gin.H{"deleted": true})
    }
}

func handleRetryPrescription(db *gorm.DB) gin.HandlerFunc {
    return func(c *gin.Context) {
        id, ok := parseUintParam(c, "id")
        if !ok {
            return
        }
        var p PrescriptionImport
        if err := db.Where("id = ? AND book_id = ?", id, contextBookID(c)).First(&p).Error; err != nil {
            response.ErrorNotFound(c, "处方记录不存在")
            return
        }
        if len(p.ImageEncrypted) == 0 {
            response.ErrorBadRequest(c, "原图未保存，请重新上传")
            return
        }
        ensureSystemConfigs(db)
        var cfg SystemAIConfig
        db.First(&cfg, 1)
        if !cfg.Enabled {
            response.Error(c, http.StatusServiceUnavailable, 2011, "AI 识别未配置")
            return
        }
        image, err := decryptBytes(db, p.ImageEncrypted)
        if err != nil {
            response.ErrorInternal(c, "读取处方图片失败")
            return
        }
        p.Status = PrescriptionRecognizing
        p.ErrorCode = ""
        p.ErrorMessage = ""
        p.UpdatedAt = nowUTC()
        db.Save(&p)
        runPrescriptionRecognition(db, cfg, p, image, p.ImageMIME, p.Kind)
        response.Success(c, gin.H{"import_id": p.ID, "status": PrescriptionRecognizing})
    }
}

type confirmPrescriptionItem struct {
    RawName       string   `json:"raw_name"`
    Name          string   `json:"name"`
    GenericName   string   `json:"generic_name"`
    Spec          string   `json:"spec"`
    Form          string   `json:"form"`
    Route         string   `json:"route"`
    MedicineMode  string   `json:"medicine_mode"`
    MedicineID    uint     `json:"medicine_id"`
    DoseAmount    float64  `json:"dose_amount"`
    DoseUnit      string   `json:"dose_unit"`
    FrequencyType string   `json:"frequency_type"`
    Times         []string `json:"times"`
    CustomSchedule []ScheduleDay `json:"custom_schedule"`
    IntervalDays  int      `json:"interval_days"`
    Weekdays      []int    `json:"weekdays"`
    IntervalHours int      `json:"interval_hours"`
    StartAt       string   `json:"start_at"`
    DurationDays  int      `json:"duration_days"`
    Instructions  string   `json:"instructions"`
    CreatePlan    bool     `json:"create_plan"`
    // ReceivedAt 批次时间（RFC3339）；库里是入库批次的 ReceivedAt，缺省用当前时间。
    ReceivedAt string `json:"received_at"`
    // Quantity 收费单上的数量：没有计划时直接入库，方便"照着缴费单补库存"。
    Quantity  float64 `json:"quantity"`
    UnitPrice float64 `json:"unit_price"`
    Amount    float64 `json:"amount"`
    // 只建档时与「添加药品」表单一致的字段。
    LowStockThreshold float64 `json:"low_stock_threshold"`
    ExpiryWarnDays    int `json:"expiry_warn_days"`
}

// parseBatchTime 解析前端传来的批次时间（RFC3339，带时区）；解析失败回落到当前时间。
func parseBatchTime(raw string, fallback time.Time) time.Time {
    s := strings.TrimSpace(raw)
    if s == "" {
        return fallback
    }
    if t, err := time.Parse(time.RFC3339, s); err == nil {
        return t.UTC()
    }
    if t, err := time.ParseInLocation("2006-01-02 15:04:05", s, loadLocation(BeijingTimezone)); err == nil {
        return t.UTC()
    }
    if t, err := time.ParseInLocation("2006-01-02 15:04", s, loadLocation(BeijingTimezone)); err == nil {
        return t.UTC()
    }
    return fallback
}

func handleConfirmPrescription(db *gorm.DB) gin.HandlerFunc {
    return func(c *gin.Context) {
        id, ok := parseUintParam(c, "id")
        if !ok {
            return
        }
        var p PrescriptionImport
        if err := db.Where("id = ? AND book_id = ?", id, contextBookID(c)).First(&p).Error; err != nil {
            response.ErrorNotFound(c, "处方记录不存在")
            return
        }
        if p.Status == PrescriptionConfirmed {
            response.ErrorBadRequest(c, "该处方已经确认录入")
            return
        }
        var in struct {
            StartDate string                    `json:"start_date"`
            Hospital  string                    `json:"hospital"`
            Doctor    string                    `json:"doctor"`
            Channel   string                    `json:"channel"`
            Items     []confirmPrescriptionItem `json:"items"`
        }
        if err := c.ShouldBindJSON(&in); err != nil || len(in.Items) == 0 {
            response.ErrorBadRequest(c, "请至少确认一种药品")
            return
        }
        // 确认录入会建药品档案并入库，所以光有「处方识别 · 确认录入」不够，还要
        // 「药品与库存 · 新增」；条目里勾了排计划的，另需「用药计划 · 新建」，
        // 否则处方页就成了绕过药品与计划门禁的后门。
        member := contextBookMember(c)
        if !memberHasPermission(member, PermMedicineCreate) {
            response.ErrorForbidden(c, "确认录入会建药品档案并入库，需要「药品与库存 · 新增」权限，请联系用药本管理员")
            return
        }
        if !memberHasPermission(member, PermPlanCreate) {
            for _, item := range in.Items {
                if item.CreatePlan {
                    response.ErrorForbidden(c, "这批条目要同时排用药计划，需要「用药计划 · 新建」权限；也可以先只确认入库，计划交给有权限的家人补")
                    return
                }
            }
        }
        book, _ := getBook(db, p.BookID)
        // 只有确认后要生成用药计划时才需要开始日期；只建档 / 直接入库不需要。
        needsPlan := false
        for _, it := range in.Items {
            if it.CreatePlan {
                needsPlan = true
                break
            }
        }
        if needsPlan {
            if in.StartDate == "" {
                in.StartDate = todayIn(book.Timezone)
            }
            if _, err := time.Parse("2006-01-02", in.StartDate); err != nil {
                response.ErrorBadRequest(c, "开始日期格式无效")
                return
            }
        }
        // 用途决定是否入库：只有「库存入库」才建批次补库存；
        // 处方单（生成计划）与药品建档一律不入库——这正是一直被误入库的根因。
        stockIn := true
        switch p.Intent {
        case "stock":
            stockIn = true
        case "plan", "archive":
            stockIn = false
        default:
            // 老记录没有 intent：缴费单/药盒按入库处理，处方笺不入库。
            stockIn = p.Kind == RecognizeKindReceipt || p.Kind == RecognizeKindBox
        }

        userID := currentUserID(c)
        channel := normalizeStockChannel(in.Channel)
        if channel == "" {
            if p.Kind == RecognizeKindReceipt {
                channel = StockChannelOther
            } else {
                channel = StockChannelHospital
            }
        }
        entrySource := StockEntryPrescription
        switch p.Kind {
        case RecognizeKindBox:
            entrySource = StockEntryBox
        case RecognizeKindReceipt:
            entrySource = StockEntryReceipt
        }
        created := make([]map[string]interface{}, 0, len(in.Items))
        mergedCount := 0
        err := db.Transaction(func(tx *gorm.DB) error {
            var existing []Medicine
            if err := tx.Where("book_id = ? AND archived_at IS NULL", book.ID).Find(&existing).Error; err != nil {
                return err
            }
            for idx, item := range in.Items {
                if strings.TrimSpace(item.Name) == "" {
                    return fmt.Errorf("第 %d 条药品名称为空，请补全或删除这一条", idx+1)
                }
                if strings.TrimSpace(item.DoseUnit) == "" {
                    return fmt.Errorf("第 %d 条「%s」缺少单位（片/盒/支/瓶等），请补全", idx+1, strings.TrimSpace(item.Name))
                }
                // 只有要排计划时才需要剂量；药盒与缴费单只入库，数量在 quantity 里。
                if item.CreatePlan && item.DoseAmount <= 0 {
                    return errors.New("排计划需要填写每次剂量")
                }
                var medicine Medicine
                merged := false
                if item.MedicineMode == "existing" {
                    if err := tx.Where("id = ? AND book_id = ? AND archived_at IS NULL", item.MedicineID, book.ID).First(&medicine).Error; err != nil {
                        return errors.New("选择的已有药品不存在")
                    }
                } else if m, ok := matchMedicineInList(existing, item.Name, item.Spec); ok {
                    // 同名药品已在库里：合并补库存，不重复建档（2026-11-17 防重复加入）。
                    medicine = m
                    merged = true
                    mergedCount++
                } else {
                    category := item.Route
                    if category == "" {
                        category = CatOther
                    }
                    warnDays := item.ExpiryWarnDays
                    if warnDays <= 0 {
                        warnDays = 30
                    }
                    lowStock := item.LowStockThreshold
                    if lowStock < 0 {
                        lowStock = 0
                    }
                    medicine = Medicine{BookID: book.ID, Name: item.Name, GenericName: item.GenericName, Category: category, Form: item.Form, Spec: item.Spec, Unit: item.DoseUnit, Notes: item.Instructions, LowStockThreshold: lowStock, ExpiryWarnDays: warnDays, UnitPrice: clampPrice(item.UnitPrice), Version: 1, CreatedBy: userID, UpdatedBy: userID, CreatedAt: nowUTC(), UpdatedAt: nowUTC()}
                    if err := tx.Create(&medicine).Error; err != nil {
                        return err
                    }
                    existing = append(existing, medicine)
                }
                var planID uint
                // 只有「库存入库」用途才建批次并记入库流水；处方单一律只生成计划，不入库。
                if stockIn && item.Quantity > 0 {
                    now := nowUTC()
                    note := "处方识别导入"
                    if p.Kind == RecognizeKindReceipt {
                        note = "缴费单识别导入"
                    } else if p.Kind == RecognizeKindBox {
                        note = "药盒识别导入"
                    }
                    batchAt := parseBatchTime(item.ReceivedAt, now)
                    batch := MedicineBatch{BookID: book.ID, MedicineID: medicine.ID, QuantityInitial: item.Quantity, QuantityRemaining: item.Quantity, ReceivedAt: batchAt, Note: note, CreatedBy: userID, CreatedAt: now, UpdatedAt: now}
                    if err := tx.Create(&batch).Error; err != nil {
                        return err
                    }
                    mv := StockMovement{BookID: book.ID, MedicineID: medicine.ID, BatchID: &batch.ID, Type: "in", QuantityDelta: item.Quantity, QuantityAfter: item.Quantity, Reason: note, Channel: channel, EntrySource: entrySource, ActorUserID: userID, CreatedAt: now}
                    if err := tx.Create(&mv).Error; err != nil {
                        return err
                    }
                    updateMedicineStock(tx, book.ID, medicine.ID)
                }
                if item.CreatePlan {
                    endDate := ""
                    if item.DurationDays > 0 {
                        start, _ := time.Parse("2006-01-02", in.StartDate)
                        endDate = start.AddDate(0, 0, item.DurationDays-1).Format("2006-01-02")
                    }
                    pin := planInput{
                        MedicineID: medicine.ID,
                        Title:      medicine.Name + "处方用药",
                        Type:       firstNonEmpty(item.Route, medicine.Category),
                        DoseAmount: item.DoseAmount,
                        DoseUnit:   item.DoseUnit,
                        Frequency: FrequencyDTO{
                            Type: item.FrequencyType, Times: item.Times, IntervalDays: item.IntervalDays,
                            Weekdays: item.Weekdays, IntervalHours: item.IntervalHours, StartAt: item.StartAt, CustomSchedule: item.CustomSchedule,
                        },
                        StartDate: in.StartDate, EndDate: endDate, Timezone: book.Timezone,
                        Doctor: in.Doctor, Hospital: in.Hospital, Notes: item.Instructions,
                    }
                    if err := pin.normalize(tx, book); err != nil {
                        return err
                    }
                    plan := Plan{BookID: book.ID, Status: PlanActive, NotificationsEnabled: true, Version: 1, CreatedBy: userID, CreatedAt: nowUTC()}
                    applyPlanInput(tx, &plan, pin, book, userID)
                    if err := tx.Create(&plan).Error; err != nil {
                        return err
                    }
                    if _, err := generateForPlanRange(tx, plan); err != nil {
                        return err
                    }
                    planID = plan.ID
                }
                created = append(created, map[string]interface{}{"medicine_id": medicine.ID, "plan_id": planID, "name": medicine.Name, "merged": merged})
            }
            p.Status = PrescriptionConfirmed
            now := nowUTC()
            p.ConfirmedAt = &now
            p.ConfirmedJSON = encodeJSON(in)
            p.UpdatedAt = now
            return tx.Save(&p).Error
        })
        if err != nil {
            response.ErrorBadRequest(c, err.Error())
            return
        }
        audit.Log(db, c, "medcare_prescription_confirm", "prescription_import", p.ID, "")
        response.Success(c, gin.H{"created": created, "merged": mergedCount})
    }
}
