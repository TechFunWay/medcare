package medcare

import (
    "errors"
    "fmt"
    "math"
    "net/http"
    "sort"
    "strings"
    "time"

    "smallgo/server/audit"
    "smallgo/server/logger"
    "smallgo/server/response"
    "smallgo/server/utils"

    "github.com/gin-gonic/gin"
    "gorm.io/gorm"
)

type MedicineDTO struct {
    ID                uint       `json:"id"`
    BookID            uint       `json:"book_id"`
    Name              string     `json:"name"`
    GenericName       string     `json:"generic_name"`
    Aliases           []string   `json:"aliases"`
    Category          string     `json:"category"`
    Form              string     `json:"form"`
    Spec              string     `json:"spec"`
    Unit              string     `json:"unit"`
    PackageSpec       string     `json:"package_spec"`
    Manufacturer      string     `json:"manufacturer"`
    Color             string     `json:"color"`
    CoverImageID      uint       `json:"cover_image_id"`
    Notes             string     `json:"notes"`
    StockQuantity     float64    `json:"stock_quantity"`
    LowStockThreshold float64    `json:"low_stock_threshold"`
    LowStockNotifyEnabled bool   `json:"low_stock_notify_enabled"`
    ExpiryWarnDays    int        `json:"expiry_warn_days"`
    UnitPrice         float64    `json:"unit_price"`
    StockStatus       string     `json:"stock_status"`
    NearestExpiry     string     `json:"nearest_expiry,omitempty"`
    ActivePlanCount   int64      `json:"active_plan_count"`
    ArchivedAt        *time.Time `json:"archived_at,omitempty"`
    Version           uint       `json:"version"`
    CreatedByName     string     `json:"created_by_name"`
    UpdatedByName     string     `json:"updated_by_name"`
    CreatedAt         time.Time  `json:"created_at"`
    UpdatedAt         time.Time  `json:"updated_at"`
}

type LocationDTO struct {
    ID           uint      `json:"id"`
    BookID       uint      `json:"book_id"`
    Name         string    `json:"name"`
    Type         string    `json:"type"`
    Address      string    `json:"address"`
    ContactName  string    `json:"contact_name"`
    ContactPhone string    `json:"contact_phone"`
    Latitude     *float64  `json:"latitude,omitempty"`
    Longitude    *float64  `json:"longitude,omitempty"`
    Notes        string    `json:"notes"`
    CreatedAt    time.Time `json:"created_at"`
    UpdatedAt    time.Time `json:"updated_at"`
}

func parseAliases(raw string) []string {
    raw = strings.TrimSpace(raw)
    if raw == "" || raw == "null" || raw == "[]" {
        return nil
    }
    var out []string
    if err := decodeJSON(raw, &out); err == nil {
        return out
    }
    for _, p := range strings.Split(raw, ",") {
        if v := strings.TrimSpace(p); v != "" {
            out = append(out, v)
        }
    }
    return out
}

// medicineToDTO 组装单个药品的返回结构：派生数据走与列表相同的批量预取，
// 单条也只发三次查询，且不再重复查最近效期。取不到派生数据时按空值展示并记日志，
// 不影响药品本身的读写结果。
// clampPrice 单价兜底：负数与 NaN 一律按 0，其余保留两位小数。
func clampPrice(v float64) float64 {
    if v <= 0 || math.IsNaN(v) || math.IsInf(v, 0) {
        return 0
    }
    return math.Round(v*100) / 100
}

func medicineToDTO(db *gorm.DB, m Medicine, today string) MedicineDTO {
    agg, err := loadMedicineAggregates(db, m.BookID, []Medicine{m}, today)
    if err != nil {
        logger.Error("medcare: 读取药品派生数据失败（按空值展示）: %v", err)
    }
    return buildMedicineDTO(m, today, agg)
}

func addDays(date string, days int) string {
    t, err := time.Parse("2006-01-02", date)
    if err != nil {
        return date
    }
    return t.AddDate(0, 0, days).Format("2006-01-02")
}

// 库存状态筛选下推到 SQL：原先 expiring / expired 是在分页之后用 Go 过滤的，
// 导致 total 与实际返回条数不符、翻页还会漏数据。这里把五种状态全部变成 SQL 条件，
// 口径与 stockStatusOf 完全一致——缺货 → 已过期 → 库存不足 → 临期 → 正常，
// 靠前的状态优先，所以「临期」要排除已过期与库存不足的药品。
const (
    expiredBatchExistsSQL  = `EXISTS (SELECT 1 FROM medicine_batches b WHERE b.medicine_id = medicines.id AND b.book_id = medicines.book_id AND b.quantity_remaining > 0 AND b.expiry_date <> '' AND b.expiry_date < ?)`
    expiringBatchExistsSQL = `EXISTS (SELECT 1 FROM medicine_batches b WHERE b.medicine_id = medicines.id AND b.book_id = medicines.book_id AND b.quantity_remaining > 0 AND b.expiry_date <> '' AND b.expiry_date <= date(?, '+' || medicines.expiry_warn_days || ' days'))`
)

// applyStockStatusFilter 按库存状态追加 SQL 条件；未知或空状态不加条件。
func applyStockStatusFilter(q *gorm.DB, status, today string) *gorm.DB {
    switch status {
    case "out":
        return q.Where("stock_quantity <= 0")
    case "low":
        return q.Where("low_stock_threshold > 0 AND stock_quantity > 0 AND stock_quantity <= low_stock_threshold")
    case "expired":
        return q.Where("stock_quantity > 0").Where(expiredBatchExistsSQL, today)
    case "expiring":
        return q.Where("stock_quantity > 0").
            Where("NOT "+expiredBatchExistsSQL, today).
            Where("NOT (low_stock_threshold > 0 AND stock_quantity <= low_stock_threshold)").
            Where(expiringBatchExistsSQL, today)
    case "ok":
        // 正常：有库存、未过期、不低库存、也不临期。原先传 ok 会被当成不过滤，
        // 直接返回全部药品，是个容易踩的哑坑。
        return q.Where("stock_quantity > 0").
            Where("NOT "+expiredBatchExistsSQL, today).
            Where("NOT (low_stock_threshold > 0 AND stock_quantity <= low_stock_threshold)").
            Where("NOT "+expiringBatchExistsSQL, today)
    }
    return q
}

// handleMedicineStats 统计当前用药本在管药品总数与各分类数量（不含已归档），
// 给药品页顶部做分类统计条。分类口径与 CatXxx 常量一致，未知分类归入 other。
func handleMedicineStats(db *gorm.DB) gin.HandlerFunc {
    return func(c *gin.Context) {
        rows := []struct {
            Category string
            Count    int64
        }{}
        if err := db.Model(&Medicine{}).
            Select("category, COUNT(*) AS count").
            Where("book_id = ? AND archived_at IS NULL", contextBookID(c)).
            Group("category").Scan(&rows).Error; err != nil {
            response.ErrorInternal(c, "读取药品统计失败")
            return
        }
        byCat := map[string]int64{}
        var total int64
        for _, r := range rows {
            cat := r.Category
            switch cat {
            case CatOral, CatInjection, CatInfusion, CatTopical, CatOther:
            default:
                cat = CatOther
            }
            byCat[cat] += r.Count
            total += r.Count
        }
        response.Success(c, gin.H{
            "total": total,
            "oral": byCat[CatOral], "injection": byCat[CatInjection], "infusion": byCat[CatInfusion],
            "topical": byCat[CatTopical], "other": byCat[CatOther],
        })
    }
}

func handleListMedicines(db *gorm.DB) gin.HandlerFunc {
    return func(c *gin.Context) {
        bookID := contextBookID(c)
        book, _ := getBook(db, bookID)
        today := todayIn(book.Timezone)
        page := utils.Atoi(c.Query("page"), 1)
        pageSize := utils.Atoi(c.Query("pageSize"), 20)
        if pageSize > 100 {
            pageSize = 100
        }
        q := db.Model(&Medicine{}).Where("book_id = ?", bookID)
        if strings.TrimSpace(c.Query("include_archived")) != "1" {
            q = q.Where("archived_at IS NULL")
        }
        if v := strings.TrimSpace(c.Query("query")); v != "" {
            like := "%" + v + "%"
            q = q.Where("name LIKE ? OR generic_name LIKE ? OR aliases LIKE ? OR spec LIKE ?", like, like, like, like)
        }
        if v := strings.TrimSpace(c.Query("category")); v != "" {
            q = q.Where("category = ?", v)
        }
        // 库存状态统一在 SQL 里筛，保证 total 与分页结果一致
        q = applyStockStatusFilter(q, strings.TrimSpace(c.Query("stock_status")), today)
        switch c.Query("sort") {
        case "stock":
            q = q.Order("stock_quantity ASC")
        case "stock_desc":
            q = q.Order("stock_quantity DESC")
        case "name":
            q = q.Order("name ASC")
        case "name_desc":
            q = q.Order("name DESC")
        case "created":
            q = q.Order("created_at DESC")
        default:
            q = q.Order("updated_at DESC")
        }
        var total int64
        if err := q.Count(&total).Error; err != nil {
            response.ErrorInternal(c, "读取药品失败")
            return
        }
        var rows []Medicine
        if err := q.Offset(utils.Offset(page, pageSize)).Limit(pageSize).Find(&rows).Error; err != nil {
            response.ErrorInternal(c, "读取药品失败")
            return
        }
        // 一页药品的派生数据一次取回，避免逐行查询
        dtos, err := medicinesToDTOs(db, bookID, rows, today)
        if err != nil {
            response.ErrorInternal(c, "读取药品失败")
            return
        }
        // 状态筛选已在 SQL 完成，这里不再做二次过滤，避免 total 与条目对不上
        response.SuccessPage(c, dtos, total, page, pageSize)
    }
}

func handleCreateMedicine(db *gorm.DB) gin.HandlerFunc {
    return func(c *gin.Context) {
        var in struct {
            Name              string   `json:"name"`
            GenericName       string   `json:"generic_name"`
            Aliases           []string `json:"aliases"`
            Category          string   `json:"category"`
            Form              string   `json:"form"`
            Spec              string   `json:"spec"`
            Unit              string   `json:"unit"`
            PackageSpec       string   `json:"package_spec"`
            Manufacturer      string   `json:"manufacturer"`
            Color             string   `json:"color"`
            Notes             string   `json:"notes"`
            LowStockThreshold float64  `json:"low_stock_threshold"`
            LowStockNotifyEnabled *bool `json:"low_stock_notify_enabled"`
            ExpiryWarnDays    int      `json:"expiry_warn_days"`
            UnitPrice         float64  `json:"unit_price"`
        }
        if err := c.ShouldBindJSON(&in); err != nil {
            response.ErrorBadRequest(c, "参数无效")
            return
        }
        in.Name = strings.TrimSpace(in.Name)
        in.Unit = strings.TrimSpace(in.Unit)
        if in.Name == "" || len([]rune(in.Name)) > 100 {
            response.ErrorBadRequest(c, "药品名称应为 1–100 个字符")
            return
        }
        if in.Unit == "" {
            response.ErrorBadRequest(c, "请填写库存单位")
            return
        }
        if in.Category == "" {
            in.Category = CatOther
        }
        if in.ExpiryWarnDays <= 0 {
            in.ExpiryWarnDays = 30
        }
        if in.LowStockThreshold < 0 {
            in.LowStockThreshold = 0
        }
        userID := currentUserID(c)
        now := nowUTC()
        m := Medicine{
            BookID: contextBookID(c), Name: in.Name, GenericName: in.GenericName, Aliases: encodeJSON(in.Aliases),
            Category: in.Category, Form: in.Form, Spec: in.Spec, Unit: in.Unit, PackageSpec: in.PackageSpec,
            Manufacturer: in.Manufacturer, Color: in.Color, Notes: in.Notes, LowStockThreshold: in.LowStockThreshold,
            LowStockNotifyDisabled: in.LowStockNotifyEnabled != nil && !*in.LowStockNotifyEnabled,
            ExpiryWarnDays: in.ExpiryWarnDays, UnitPrice: clampPrice(in.UnitPrice), Version: 1, CreatedBy: userID, UpdatedBy: userID, CreatedAt: now, UpdatedAt: now,
        }
        if err := db.Create(&m).Error; err != nil {
            response.ErrorInternal(c, "创建药品失败")
            return
        }
        audit.Log(db, c, "medcare_medicine_create", "medicine", m.ID, m.Name)
        book, _ := getBook(db, m.BookID)
        response.Success(c, medicineToDTO(db, m, todayIn(book.Timezone)))
    }
}

// importMedicineItem 是「药盒 / 缴费单识别后批量建药品」的单条输入。
// 每条要么新建药品并入库，要么挂到已有药品上追加库存。
type importMedicineItem struct {
    Name               string  `json:"name"`
    GenericName        string  `json:"generic_name"`
    Category           string  `json:"category"`
    Form               string  `json:"form"`
    Spec               string  `json:"spec"`
    Unit               string  `json:"unit"`
    PackageSpec        string  `json:"package_spec"`
    Manufacturer       string  `json:"manufacturer"`
    Notes              string  `json:"notes"`
    Quantity           float64 `json:"quantity"`
    BatchNo            string  `json:"batch_no"`
    ExpiryDate         string  `json:"expiry_date"`
    Supplier           string  `json:"supplier"`
    LowStockThreshold  float64 `json:"low_stock_threshold"`
    ExpiryWarnDays     int     `json:"expiry_warn_days"`
    UnitPrice          float64 `json:"unit_price"`
    ReceivedAt         string  `json:"received_at"`
    ExistingMedicineID uint    `json:"existing_medicine_id"`
}

// handleImportMedicines 处理「药盒识别 / 缴费单识别」后的批量建药品与入库。
// 整批在一个事务里做：任一条失败就整批回滚，不会留下半截数据。

// normalizeMedicineName 归一化药品名用于查重：去空白、统一小写（中文不受影响）。
func normalizeMedicineName(s string) string {
    return strings.ToLower(strings.Join(strings.Fields(s), ""))
}

// matchMedicineInList 在已加载的本用药本药品里按名字找未归档档案：
// 识别导入默认合并到同名药品补库存，避免重复建档（2026-11-17 用户要求「不能重复加入」）。
// 同名多条时优先规格完全一致的那条，否则合并到最早的一条。
func matchMedicineInList(list []Medicine, name, spec string) (Medicine, bool) {
    norm := normalizeMedicineName(name)
    if norm == "" {
        return Medicine{}, false
    }
    var matches []Medicine
    for _, m := range list {
        if normalizeMedicineName(m.Name) == norm {
            matches = append(matches, m)
        }
    }
    if len(matches) == 0 {
        return Medicine{}, false
    }
    if len(matches) > 1 && strings.TrimSpace(spec) != "" {
        for _, m := range matches {
            if strings.TrimSpace(m.Spec) == strings.TrimSpace(spec) {
                return m, true
            }
        }
    }
    return matches[0], true
}

func handleImportMedicines(db *gorm.DB) gin.HandlerFunc {
    return func(c *gin.Context) {
        var in struct {
            Source   string               `json:"source"`
            Channel  string               `json:"channel"`
            ImportID uint                 `json:"import_id"`
            Items    []importMedicineItem `json:"items"`
        }
        if err := c.ShouldBindJSON(&in); err != nil {
            response.ErrorBadRequest(c, "参数无效")
            return
        }
        if len(in.Items) == 0 {
            response.ErrorBadRequest(c, "请至少选择一种药品")
            return
        }
        if len(in.Items) > 100 {
            response.ErrorBadRequest(c, "一次最多导入 100 种药品")
            return
        }
        bookID := contextBookID(c)
        userID := currentUserID(c)
        // 识别记录导入防重：已录入的单据不允许再导一次；还在识别中的等完成再来。
        if in.ImportID > 0 {
            var rec PrescriptionImport
            if err := db.Where("id = ? AND book_id = ?", in.ImportID, bookID).First(&rec).Error; err != nil {
                response.ErrorBadRequest(c, "识别记录不存在，请重新识别")
                return
            }
            if rec.Status == PrescriptionConfirmed {
                response.ErrorBadRequest(c, "该识别记录已导入过，不能重复加入；如需补库存请走出入库")
                return
            }
            if rec.Status != PrescriptionRecognized {
                response.ErrorBadRequest(c, "识别尚未完成或已失败，请先完成识别再导入")
                return
            }
        }
        channel := normalizeStockChannel(in.Channel)
        if channel == "" {
            channel = StockChannelOther
        }
        entrySource := StockEntryManual
        switch normalizeStockEntrySource(in.Source) {
        case StockEntryBox:
            entrySource = StockEntryBox
        case StockEntryReceipt:
            entrySource = StockEntryReceipt
        }
        created := make([]map[string]interface{}, 0, len(in.Items))
        mergedCount := 0
        err := db.Transaction(func(tx *gorm.DB) error {
            now := nowUTC()
            var existing []Medicine
            if err := tx.Where("book_id = ? AND archived_at IS NULL", bookID).Find(&existing).Error; err != nil {
                return err
            }
            for _, item := range in.Items {
                name := strings.TrimSpace(item.Name)
                if name == "" {
                    return errors.New("药品名称不能为空")
                }
                if len([]rune(name)) > 100 {
                    return errors.New("药品名称过长：" + trimRunes(name, 20))
                }
                unit := strings.TrimSpace(item.Unit)
                if unit == "" {
                    unit = "盒"
                }
                if item.ExpiryDate != "" {
                    if _, err := time.Parse("2006-01-02", item.ExpiryDate); err != nil {
                        return errors.New("有效期格式应为 YYYY-MM-DD：" + name)
                    }
                }
                category := strings.TrimSpace(item.Category)
                switch category {
                case CatOral, CatInjection, CatInfusion, CatTopical, CatOther:
                default:
                    category = CatOther
                }
                var medicine Medicine
                merged := false
                if item.ExistingMedicineID > 0 {
                    if err := tx.Where("id = ? AND book_id = ? AND archived_at IS NULL", item.ExistingMedicineID, bookID).First(&medicine).Error; err != nil {
                        return errors.New("选择的已有药品不存在：" + name)
                    }
                } else if m, ok := matchMedicineInList(existing, name, item.Spec); ok {
                    // 同名药品已在库里：合并补库存，不重复建档。
                    medicine = m
                    merged = true
                    mergedCount++
                } else {
                    warnDays := item.ExpiryWarnDays
                    if warnDays <= 0 {
                        warnDays = 30
                    }
                    lowStock := item.LowStockThreshold
                    if lowStock < 0 {
                        lowStock = 0
                    }
                    medicine = Medicine{
                        BookID: bookID, Name: name, GenericName: strings.TrimSpace(item.GenericName),
                        Category: category, Form: strings.TrimSpace(item.Form), Spec: strings.TrimSpace(item.Spec),
                        Unit: unit, PackageSpec: strings.TrimSpace(item.PackageSpec),
                        Manufacturer: strings.TrimSpace(item.Manufacturer), Notes: strings.TrimSpace(item.Notes),
                        LowStockThreshold: lowStock, ExpiryWarnDays: warnDays, UnitPrice: clampPrice(item.UnitPrice),
                        Version: 1, CreatedBy: userID, UpdatedBy: userID, CreatedAt: now, UpdatedAt: now,
                    }
                    if err := tx.Create(&medicine).Error; err != nil {
                        return err
                    }
                    existing = append(existing, medicine)
                }
                var batchID uint
                if item.Quantity > 0 {
                    batch := MedicineBatch{
                        BookID: bookID, MedicineID: medicine.ID, BatchNo: strings.TrimSpace(item.BatchNo),
                        QuantityInitial: item.Quantity, QuantityRemaining: item.Quantity,
                        ExpiryDate: strings.TrimSpace(item.ExpiryDate), ReceivedAt: parseBatchTime(item.ReceivedAt, now),
                        Supplier: strings.TrimSpace(item.Supplier), Note: importSourceNote(in.Source),
                        CreatedBy: userID, CreatedAt: now, UpdatedAt: now,
                    }
                    if err := tx.Create(&batch).Error; err != nil {
                        return err
                    }
                    mv := StockMovement{BookID: bookID, MedicineID: medicine.ID, BatchID: &batch.ID, Type: "in", QuantityDelta: item.Quantity, QuantityAfter: item.Quantity, Reason: importSourceNote(in.Source), Channel: channel, EntrySource: entrySource, ActorUserID: userID, CreatedAt: now}
                    if err := tx.Create(&mv).Error; err != nil {
                        return err
                    }
                    updateMedicineStock(tx, bookID, medicine.ID)
                    batchID = batch.ID
                }
                created = append(created, map[string]interface{}{"medicine_id": medicine.ID, "batch_id": batchID, "name": medicine.Name, "merged": merged})
            }
            // 识别记录同步标为已录入，避免同一条单据被重复导入。
            if in.ImportID > 0 {
                _ = tx.Model(&PrescriptionImport{}).Where("id = ? AND book_id = ?", in.ImportID, bookID).Updates(map[string]interface{}{
                    "status": PrescriptionConfirmed, "confirmed_at": now, "updated_at": now,
                }).Error
            }
            return nil
        })
        if err != nil {
            response.ErrorBadRequest(c, err.Error())
            return
        }
        audit.Log(db, c, "medcare_medicine_import", "medicine", 0, fmt.Sprintf("%d 种药品（%s）", len(created), importSourceNote(in.Source)))
        response.Success(c, gin.H{"created": created, "count": len(created), "merged": mergedCount})
    }
}

func importSourceNote(source string) string {
    switch source {
    case RecognizeKindBox:
        return "药盒识别导入"
    case RecognizeKindReceipt:
        return "缴费单识别导入"
    default:
        return "识别导入"
    }
}

func handleGetMedicine(db *gorm.DB) gin.HandlerFunc {
    return func(c *gin.Context) {
        id, ok := parseUintParam(c, "id")
        if !ok {
            return
        }
        var m Medicine
        if err := db.Where("id = ? AND book_id = ?", id, contextBookID(c)).First(&m).Error; err != nil {
            response.ErrorNotFound(c, "药品不存在")
            return
        }
        book, _ := getBook(db, m.BookID)
        response.Success(c, medicineToDTO(db, m, todayIn(book.Timezone)))
    }
}

func handleUpdateMedicine(db *gorm.DB) gin.HandlerFunc {
    return func(c *gin.Context) {
        id, ok := parseUintParam(c, "id")
        if !ok {
            return
        }
        var m Medicine
        if err := db.Where("id = ? AND book_id = ?", id, contextBookID(c)).First(&m).Error; err != nil {
            response.ErrorNotFound(c, "药品不存在")
            return
        }
        var in struct {
            Name              string   `json:"name"`
            GenericName       string   `json:"generic_name"`
            Aliases           []string `json:"aliases"`
            Category          string   `json:"category"`
            Form              string   `json:"form"`
            Spec              string   `json:"spec"`
            Unit              string   `json:"unit"`
            PackageSpec       string   `json:"package_spec"`
            Manufacturer      string   `json:"manufacturer"`
            Color             string   `json:"color"`
            Notes             string   `json:"notes"`
            LowStockThreshold float64  `json:"low_stock_threshold"`
            LowStockNotifyEnabled *bool `json:"low_stock_notify_enabled"`
            ExpiryWarnDays    int      `json:"expiry_warn_days"`
            UnitPrice         *float64 `json:"unit_price"`
            Version           uint     `json:"version"`
        }
        if err := c.ShouldBindJSON(&in); err != nil {
            response.ErrorBadRequest(c, "参数无效")
            return
        }
        if in.Version > 0 && in.Version != m.Version {
            response.Error(c, http.StatusConflict, 2007, ErrVersionConflict.Error())
            return
        }
        if name := strings.TrimSpace(in.Name); name != "" {
            m.Name = name
        }
        if in.Unit != "" {
            m.Unit = in.Unit
        }
        m.GenericName = in.GenericName
        m.Aliases = encodeJSON(in.Aliases)
        if in.Category != "" {
            m.Category = in.Category
        }
        m.Form = in.Form
        m.Spec = in.Spec
        m.PackageSpec = in.PackageSpec
        m.Manufacturer = in.Manufacturer
        m.Color = in.Color
        m.Notes = in.Notes
        if in.LowStockThreshold >= 0 {
            m.LowStockThreshold = in.LowStockThreshold
        }
        if in.LowStockNotifyEnabled != nil {
            m.LowStockNotifyDisabled = !*in.LowStockNotifyEnabled
        }
        if in.ExpiryWarnDays > 0 {
            m.ExpiryWarnDays = in.ExpiryWarnDays
        }
        if in.UnitPrice != nil {
            m.UnitPrice = clampPrice(*in.UnitPrice)
        }
        m.Version++
        m.UpdatedBy = currentUserID(c)
        m.UpdatedAt = nowUTC()
        if err := db.Save(&m).Error; err != nil {
            response.ErrorInternal(c, "保存药品失败")
            return
        }
        audit.Log(db, c, "medcare_medicine_update", "medicine", m.ID, m.Name)
        book, _ := getBook(db, m.BookID)
        response.Success(c, medicineToDTO(db, m, todayIn(book.Timezone)))
    }
}

// medicineRelations 汇总「删除药品」前要看的绑定情况：计划、库存批次、出入库流水。
// 前端用它决定弹窗里提示什么、能不能删；归档接口也用同一口径兜底。
func medicineRelations(db *gorm.DB, m Medicine) (activePlans, planTotal, batchCount int64, remaining float64, movementCount int64) {
    db.Model(&Plan{}).Where("book_id = ? AND medicine_id = ? AND status IN ?", m.BookID, m.ID, []string{PlanActive, PlanPaused}).Count(&activePlans)
    db.Model(&Plan{}).Where("book_id = ? AND medicine_id = ?", m.BookID, m.ID).Count(&planTotal)
    db.Model(&MedicineBatch{}).Where("book_id = ? AND medicine_id = ?", m.BookID, m.ID).Count(&batchCount)
    var sum struct{ Total float64 }
    db.Model(&MedicineBatch{}).Where("book_id = ? AND medicine_id = ?", m.BookID, m.ID).
        Select("COALESCE(SUM(quantity_remaining),0) AS total").Scan(&sum)
    remaining = sum.Total
    db.Model(&StockMovement{}).Where("book_id = ? AND medicine_id = ?", m.BookID, m.ID).Count(&movementCount)
    return
}

// handleMedicineRelations 删除前的绑定清单：有进行中计划或还有库存就不让删，
// 出入库流水只做提示（软删除后流水保留，不影响历史）。
func handleMedicineRelations(db *gorm.DB) gin.HandlerFunc {
    return func(c *gin.Context) {
        id, ok := parseUintParam(c, "id")
        if !ok {
            return
        }
        var m Medicine
        if err := db.Where("id = ? AND book_id = ? AND archived_at IS NULL", id, contextBookID(c)).First(&m).Error; err != nil {
            response.ErrorNotFound(c, "药品不存在")
            return
        }
        activePlans, planTotal, batchCount, remaining, movementCount := medicineRelations(db, m)
        blockers := make([]gin.H, 0, 2)
        notes := make([]string, 0, 2)
        if activePlans > 0 {
            blockers = append(blockers, gin.H{"key": "plan", "label": fmt.Sprintf("进行中或暂停的计划 %d 个", activePlans), "hint": "删除后这些计划会一并结束"})
        }
        if remaining > 0.000001 {
            blockers = append(blockers, gin.H{"key": "stock", "label": fmt.Sprintf("当前库存 %g %s（%d 个批次）", remaining, m.Unit, batchCount), "hint": "删除后库存不再显示，批次与流水记录保留"})
        }
        if movementCount > 0 {
            notes = append(notes, fmt.Sprintf("出入库记录 %d 条：删除后历史记录仍保留", movementCount))
        }
        if planTotal > activePlans {
            notes = append(notes, fmt.Sprintf("已结束的历史计划 %d 个", planTotal-activePlans))
        }
        response.Success(c, gin.H{
            "medicine_id": m.ID, "name": m.Name, "unit": m.Unit,
            "active_plan_count": activePlans, "plan_count": planTotal,
            "batch_count": batchCount, "stock_quantity": remaining, "movement_count": movementCount,
            "can_delete": true, "has_bindings": len(blockers) > 0, "blockers": blockers, "notes": notes,
        })
    }
}

func handleArchiveMedicine(db *gorm.DB, archive bool) gin.HandlerFunc {
    return func(c *gin.Context) {
        archiveDetail := ""
        id, ok := parseUintParam(c, "id")
        if !ok {
            return
        }
        var m Medicine
        if err := db.Where("id = ? AND book_id = ?", id, contextBookID(c)).First(&m).Error; err != nil {
            response.ErrorNotFound(c, "药品不存在")
            return
        }
        if archive {
            activePlans, _, batchCount, remaining, movementCount := medicineRelations(db, m)
            now := nowUTC()
            // 绑定关系只做提示，用户确认后允许删除：进行中的计划一并结束，
            // 库存与流水按软删除保留在库里（列表与库存页不再显示该药）。
            if activePlans > 0 {
                if err := db.Model(&Plan{}).Where("book_id = ? AND medicine_id = ? AND status IN ?", m.BookID, m.ID, []string{PlanActive, PlanPaused}).
                    Updates(map[string]interface{}{"status": PlanEnded, "updated_at": now}).Error; err != nil {
                    response.ErrorInternal(c, "结束关联计划失败")
                    return
                }
            }
            m.ArchivedAt = &now
            m.UpdatedAt = now
            archiveDetail = fmt.Sprintf("%s（删除时：库存 %g %s／批次 %d／进行中计划 %d／出入库 %d 条）",
                m.Name, remaining, m.Unit, batchCount, activePlans, movementCount)
        } else {
            m.ArchivedAt = nil
        }
        m.UpdatedAt = nowUTC()
        if err := db.Save(&m).Error; err != nil {
            response.ErrorInternal(c, "更新药品失败")
            return
        }
        if archiveDetail == "" {
            archiveDetail = m.Name
        }
        audit.Log(db, c, "medcare_medicine_archive", "medicine", m.ID, archiveDetail)
        response.Success(c, gin.H{"ok": true})
    }
}

func updateMedicineStock(db *gorm.DB, bookID, medicineID uint) {
    var total float64
    db.Model(&MedicineBatch{}).Where("book_id = ? AND medicine_id = ?", bookID, medicineID).Select("COALESCE(SUM(quantity_remaining),0)").Scan(&total)
    db.Model(&Medicine{}).Where("id = ? AND book_id = ?", medicineID, bookID).Update("stock_quantity", total)
}

func reloadMedicine(db *gorm.DB, m *Medicine) {
    _ = db.Where("id = ? AND book_id = ?", m.ID, m.BookID).First(m).Error
}

func handleStockIn(db *gorm.DB) gin.HandlerFunc {
    return func(c *gin.Context) {
        id, ok := parseUintParam(c, "id")
        if !ok {
            return
        }
        var m Medicine
        if err := db.Where("id = ? AND book_id = ?", id, contextBookID(c)).First(&m).Error; err != nil {
            response.ErrorNotFound(c, "药品不存在")
            return
        }
        var in struct {
            Packages        float64 `json:"packages"`
            UnitsPerPackage float64 `json:"units_per_package"`
            Quantity        float64 `json:"quantity"`
            BatchNo         string  `json:"batch_no"`
            ExpiryDate      string  `json:"expiry_date"`
            Supplier        string  `json:"supplier"`
            Note            string  `json:"note"`
            Channel         string  `json:"channel"`
        }
        if err := c.ShouldBindJSON(&in); err != nil {
            response.ErrorBadRequest(c, "参数无效")
            return
        }
        qty := in.Quantity
        if qty <= 0 && in.Packages > 0 && in.UnitsPerPackage > 0 {
            qty = in.Packages * in.UnitsPerPackage
        }
        if qty <= 0 {
            response.ErrorBadRequest(c, "入库数量必须大于 0")
            return
        }
        if in.ExpiryDate != "" {
            if _, err := time.Parse("2006-01-02", in.ExpiryDate); err != nil {
                response.ErrorBadRequest(c, "有效期格式应为 YYYY-MM-DD")
                return
            }
        }
        now := nowUTC()
        channel := normalizeStockChannel(in.Channel)
        if channel == "" {
            channel = StockChannelOther
        }
        batch := MedicineBatch{
            BookID: m.BookID, MedicineID: m.ID, BatchNo: in.BatchNo, QuantityInitial: qty,
            QuantityRemaining: qty, ExpiryDate: in.ExpiryDate, ReceivedAt: now, Supplier: in.Supplier,
            Note: in.Note, CreatedBy: currentUserID(c), CreatedAt: now, UpdatedAt: now,
        }
        err := db.Transaction(func(tx *gorm.DB) error {
            if err := tx.Create(&batch).Error; err != nil {
                return err
            }
            mv := StockMovement{BookID: m.BookID, MedicineID: m.ID, BatchID: &batch.ID, Type: "in", QuantityDelta: qty, QuantityAfter: qty, Reason: "入库", Channel: channel, EntrySource: StockEntryManual, ActorUserID: currentUserID(c), CreatedAt: now}
            if err := tx.Create(&mv).Error; err != nil {
                return err
            }
            updateMedicineStock(tx, m.BookID, m.ID)
            return nil
        })
        if err != nil {
            response.ErrorInternal(c, "入库失败")
            return
        }
        audit.Log(db, c, "medcare_stock_in", "medicine", m.ID, fmt.Sprintf("%.3f %s", qty, m.Unit))
        book, _ := getBook(db, m.BookID)
        reloadMedicine(db, &m)
        response.Success(c, gin.H{"batch": batch, "medicine": medicineToDTO(db, m, todayIn(book.Timezone))})
    }
}

// handleStockOut 出库：按先进先出扣批次并记一条 out 流水。
// 与「盘点」不同，这里是「出 X 个」，不是「调整到 X」；库存不足直接拒绝并告知余量。
// handleConvertMedicineUnit 修改药品的库存单位，并按倍数把已有数据一起换算。
// 典型场景：入库时把「片」录成了「盒」，改成正确单位时把 2 盒按 1 盒 = 20 片换算成 40 片，
// 批次、流水与当前库存同步换算，避免单位与数量对不上。factor=1 表示只换单位、数量不变。
func handleConvertMedicineUnit(db *gorm.DB) gin.HandlerFunc {
    return func(c *gin.Context) {
        id, ok := parseUintParam(c, "id")
        if !ok {
            return
        }
        var m Medicine
        if err := db.Where("id = ? AND book_id = ? AND archived_at IS NULL", id, contextBookID(c)).First(&m).Error; err != nil {
            response.ErrorNotFound(c, "药品不存在")
            return
        }
        var in struct {
            Unit   string  `json:"unit"`
            Factor float64 `json:"factor"`
        }
        if err := c.ShouldBindJSON(&in); err != nil {
            response.ErrorBadRequest(c, "参数无效")
            return
        }
        unit := strings.TrimSpace(in.Unit)
        if unit == "" || len([]rune(unit)) > 20 {
            response.ErrorBadRequest(c, "库存单位应为 1–20 个字符")
            return
        }
        factor := in.Factor
        if factor <= 0 {
            factor = 1
        }
        oldUnit := m.Unit
        if oldUnit == unit && math.Abs(factor-1) < 1e-9 {
            response.Success(c, gin.H{"changed": false, "unit": unit})
            return
        }
        now := nowUTC()
        err := db.Transaction(func(tx *gorm.DB) error {
            if math.Abs(factor-1) > 1e-9 {
                if err := tx.Model(&MedicineBatch{}).Where("book_id = ? AND medicine_id = ?", m.BookID, m.ID).
                    UpdateColumns(map[string]interface{}{
                        "quantity_initial":   gorm.Expr("quantity_initial * ?", factor),
                        "quantity_remaining": gorm.Expr("quantity_remaining * ?", factor),
                    }).Error; err != nil {
                    return err
                }
                // 历史流水的数量与「变动后余量」一起换算，否则明细里的余量会和新库存对不上。
                if err := tx.Model(&StockMovement{}).Where("book_id = ? AND medicine_id = ?", m.BookID, m.ID).
                    UpdateColumns(map[string]interface{}{
                        "quantity_delta": gorm.Expr("quantity_delta * ?", factor),
                        "quantity_after": gorm.Expr("quantity_after * ?", factor),
                    }).Error; err != nil {
                    return err
                }
            }
            updates := map[string]interface{}{"unit": unit, "updated_at": now}
            if math.Abs(factor-1) > 1e-9 {
                updates["stock_quantity"] = gorm.Expr("stock_quantity * ?", factor)
            }
            return tx.Model(&Medicine{}).Where("id = ? AND book_id = ?", m.ID, m.BookID).Updates(updates).Error
        })
        if err != nil {
            response.ErrorInternal(c, "修改库存单位失败")
            return
        }
        audit.Log(db, c, "medcare_medicine_unit_convert", "medicine", m.ID,
            fmt.Sprintf("%s：%s → %s（1 %s = %g %s）", m.Name, oldUnit, unit, oldUnit, factor, unit))
        book, _ := getBook(db, m.BookID)
        reloadMedicine(db, &m)
        response.Success(c, gin.H{"changed": true, "unit": unit, "factor": factor, "medicine": medicineToDTO(db, m, todayIn(book.Timezone))})
    }
}

// handleSetMedicineCover 设置或清除药品封面：只动 cover_image_id，不碰其它字段与版本号，
// 免得和「编辑药品」表单的乐观锁互相打架。image_id 传 0 表示清除封面。
func handleSetMedicineCover(db *gorm.DB) gin.HandlerFunc {
    return func(c *gin.Context) {
        id, ok := parseUintParam(c, "id")
        if !ok {
            return
        }
        var m Medicine
        if err := db.Where("id = ? AND book_id = ? AND archived_at IS NULL", id, contextBookID(c)).First(&m).Error; err != nil {
            response.ErrorNotFound(c, "药品不存在")
            return
        }
        var in struct {
            ImageID uint `json:"image_id"`
        }
        if err := c.ShouldBindJSON(&in); err != nil {
            response.ErrorBadRequest(c, "参数无效")
            return
        }
        if in.ImageID > 0 {
            var img BookImage
            if err := db.Where("id = ? AND book_id = ?", in.ImageID, m.BookID).First(&img).Error; err != nil {
                response.ErrorBadRequest(c, "图片不存在")
                return
            }
            // 顺手把图片挂到该药品下，便于在药品详情里一起管理。
            if img.MedicineID != m.ID {
                db.Model(&img).Updates(map[string]interface{}{"medicine_id": m.ID, "updated_at": nowUTC()})
            }
        }
        if err := db.Model(&m).Updates(map[string]interface{}{"cover_image_id": in.ImageID, "updated_at": nowUTC()}).Error; err != nil {
            response.ErrorInternal(c, "设置封面失败")
            return
        }
        audit.Log(db, c, "medcare_medicine_cover", "medicine", m.ID, fmt.Sprintf("image=%d", in.ImageID))
        book, _ := getBook(db, m.BookID)
        reloadMedicine(db, &m)
        response.Success(c, gin.H{"medicine": medicineToDTO(db, m, todayIn(book.Timezone))})
    }
}

func handleStockOut(db *gorm.DB) gin.HandlerFunc {
    return func(c *gin.Context) {
        id, ok := parseUintParam(c, "id")
        if !ok {
            return
        }
        var m Medicine
        if err := db.Where("id = ? AND book_id = ? AND archived_at IS NULL", id, contextBookID(c)).First(&m).Error; err != nil {
            response.ErrorNotFound(c, "药品不存在")
            return
        }
        var in struct {
            Quantity float64 `json:"quantity"`
            Reason   string  `json:"reason"`
            Channel  string  `json:"channel"`
        }
        if err := c.ShouldBindJSON(&in); err != nil || in.Quantity <= 0 {
            response.ErrorBadRequest(c, "出库数量必须大于 0")
            return
        }
        reason := strings.TrimSpace(in.Reason)
        if reason == "" {
            reason = "出库"
        }
        now := nowUTC()
        channel := normalizeStockChannel(in.Channel)
        if channel == "" {
            channel = StockChannelOther
        }
        var short bool
        var remainQty float64
        err := db.Transaction(func(tx *gorm.DB) error {
            var batches []MedicineBatch
            tx.Where("book_id = ? AND medicine_id = ?", m.BookID, m.ID).
                Order("CASE WHEN expiry_date = '' THEN 1 ELSE 0 END, expiry_date ASC, received_at ASC, id ASC").Find(&batches)
            total := 0.0
            for _, b := range batches {
                total += b.QuantityRemaining
            }
            if total < in.Quantity-0.000001 {
                short = true
                remainQty = total
                return nil
            }
            remain := in.Quantity
            for i := range batches {
                if remain <= 0 {
                    break
                }
                b := batches[i]
                take := math.Min(b.QuantityRemaining, remain)
                if take <= 0 {
                    continue
                }
                b.QuantityRemaining -= take
                b.UpdatedAt = now
                if err := tx.Save(&b).Error; err != nil {
                    return err
                }
                mv := StockMovement{BookID: m.BookID, MedicineID: m.ID, BatchID: &b.ID, Type: "out",
                    QuantityDelta: -take, QuantityAfter: b.QuantityRemaining, Reason: reason,
                    Channel: channel, EntrySource: StockEntryManual, ActorUserID: currentUserID(c), CreatedAt: now}
                if err := tx.Create(&mv).Error; err != nil {
                    return err
                }
                remain -= take
            }
            updateMedicineStock(tx, m.BookID, m.ID)
            return nil
        })
        if err != nil {
            response.ErrorInternal(c, "出库失败")
            return
        }
        if short {
            response.ErrorBadRequest(c, fmt.Sprintf("库存不足，当前只剩 %.3g %s", remainQty, m.Unit))
            return
        }
        audit.Log(db, c, "medcare_stock_out", "medicine", m.ID, fmt.Sprintf("%.3f %s %s", in.Quantity, m.Unit, reason))
        book, _ := getBook(db, m.BookID)
        reloadMedicine(db, &m)
        response.Success(c, gin.H{"ok": true, "medicine": medicineToDTO(db, m, todayIn(book.Timezone))})
    }
}

func handleStockAdjust(db *gorm.DB) gin.HandlerFunc {
    return func(c *gin.Context) {
        id, ok := parseUintParam(c, "id")
        if !ok {
            return
        }
        var m Medicine
        if err := db.Where("id = ? AND book_id = ?", id, contextBookID(c)).First(&m).Error; err != nil {
            response.ErrorNotFound(c, "药品不存在")
            return
        }
        var in struct {
            BatchID     uint    `json:"batch_id"`
            NewQuantity float64 `json:"new_quantity"`
            Reason      string  `json:"reason"`
        }
        if err := c.ShouldBindJSON(&in); err != nil || in.BatchID == 0 {
            response.ErrorBadRequest(c, "参数无效")
            return
        }
        if in.NewQuantity < 0 {
            response.ErrorBadRequest(c, "数量不能为负数")
            return
        }
        if strings.TrimSpace(in.Reason) == "" {
            response.ErrorBadRequest(c, "请填写调整原因")
            return
        }
        var batch MedicineBatch
        if err := db.Where("id = ? AND book_id = ? AND medicine_id = ?", in.BatchID, m.BookID, m.ID).First(&batch).Error; err != nil {
            response.ErrorNotFound(c, "批次不存在")
            return
        }
        delta := in.NewQuantity - batch.QuantityRemaining
        if math.Abs(delta) < 0.000001 {
            response.Success(c, gin.H{"ok": true})
            return
        }
        now := nowUTC()
        err := db.Transaction(func(tx *gorm.DB) error {
            batch.QuantityRemaining = in.NewQuantity
            batch.UpdatedAt = now
            if err := tx.Save(&batch).Error; err != nil {
                return err
            }
            mvType := "adjust"
            if delta < 0 {
                mvType = "waste"
            }
            entrySource := StockEntryAdjust
            if mvType == "waste" {
                entrySource = StockEntryWaste
            }
            mv := StockMovement{BookID: m.BookID, MedicineID: m.ID, BatchID: &batch.ID, Type: mvType, QuantityDelta: delta, QuantityAfter: in.NewQuantity, Reason: in.Reason, EntrySource: entrySource, ActorUserID: currentUserID(c), CreatedAt: now}
            if err := tx.Create(&mv).Error; err != nil {
                return err
            }
            updateMedicineStock(tx, m.BookID, m.ID)
            return nil
        })
        if err != nil {
            response.ErrorInternal(c, "调整库存失败")
            return
        }
        audit.Log(db, c, "medcare_stock_adjust", "medicine", m.ID, in.Reason)
        book, _ := getBook(db, m.BookID)
        reloadMedicine(db, &m)
        response.Success(c, gin.H{"ok": true, "medicine": medicineToDTO(db, m, todayIn(book.Timezone))})
    }
}

func handleStocktake(db *gorm.DB) gin.HandlerFunc {
    return func(c *gin.Context) {
        id, ok := parseUintParam(c, "id")
        if !ok {
            return
        }
        var m Medicine
        if err := db.Where("id = ? AND book_id = ?", id, contextBookID(c)).First(&m).Error; err != nil {
            response.ErrorNotFound(c, "药品不存在")
            return
        }
        var in struct {
            TargetQuantity float64 `json:"target_quantity"`
            Reason         string  `json:"reason"`
        }
        if err := c.ShouldBindJSON(&in); err != nil || in.TargetQuantity < 0 {
            response.ErrorBadRequest(c, "盘点数量无效")
            return
        }
        if strings.TrimSpace(in.Reason) == "" {
            response.ErrorBadRequest(c, "请填写盘点原因")
            return
        }
        now := nowUTC()
        err := db.Transaction(func(tx *gorm.DB) error {
            var batches []MedicineBatch
            tx.Where("book_id = ? AND medicine_id = ?", m.BookID, m.ID).Order("CASE WHEN expiry_date = '' THEN 1 ELSE 0 END, expiry_date ASC, received_at ASC, id ASC").Find(&batches)
            current := 0.0
            for _, b := range batches {
                current += b.QuantityRemaining
            }
            delta := in.TargetQuantity - current
            if math.Abs(delta) < 0.000001 {
                return nil
            }
            if delta > 0 {
                batch := MedicineBatch{BookID: m.BookID, MedicineID: m.ID, QuantityInitial: delta, QuantityRemaining: delta, ReceivedAt: now, Note: in.Reason, CreatedBy: currentUserID(c), CreatedAt: now, UpdatedAt: now}
                if err := tx.Create(&batch).Error; err != nil {
                    return err
                }
                mv := StockMovement{BookID: m.BookID, MedicineID: m.ID, BatchID: &batch.ID, Type: "adjust", QuantityDelta: delta, QuantityAfter: delta, Reason: in.Reason, EntrySource: StockEntryStocktake, ActorUserID: currentUserID(c), CreatedAt: now}
                if err := tx.Create(&mv).Error; err != nil {
                    return err
                }
            } else {
                remain := -delta
                for i := range batches {
                    if remain <= 0 {
                        break
                    }
                    b := batches[i]
                    take := math.Min(b.QuantityRemaining, remain)
                    if take <= 0 {
                        continue
                    }
                    b.QuantityRemaining -= take
                    b.UpdatedAt = now
                    if err := tx.Save(&b).Error; err != nil {
                        return err
                    }
                    mv := StockMovement{BookID: m.BookID, MedicineID: m.ID, BatchID: &b.ID, Type: "waste", QuantityDelta: -take, QuantityAfter: b.QuantityRemaining, Reason: in.Reason, EntrySource: StockEntryStocktake, ActorUserID: currentUserID(c), CreatedAt: now}
                    if err := tx.Create(&mv).Error; err != nil {
                        return err
                    }
                    remain -= take
                }
            }
            updateMedicineStock(tx, m.BookID, m.ID)
            return nil
        })
        if err != nil {
            response.ErrorInternal(c, "盘点失败")
            return
        }
        audit.Log(db, c, "medcare_stocktake", "medicine", m.ID, in.Reason)
        book, _ := getBook(db, m.BookID)
        reloadMedicine(db, &m)
        response.Success(c, gin.H{"ok": true, "medicine": medicineToDTO(db, m, todayIn(book.Timezone))})
    }
}

func handleStockMovements(db *gorm.DB) gin.HandlerFunc {
    return func(c *gin.Context) {
        id, ok := parseUintParam(c, "id")
        if !ok {
            return
        }
        page := utils.Atoi(c.Query("page"), 1)
        pageSize := utils.Atoi(c.Query("pageSize"), 20)
        if pageSize > 100 {
            pageSize = 100
        }
        q := db.Model(&StockMovement{}).Where("book_id = ? AND medicine_id = ?", contextBookID(c), id)
        var total int64
        q.Count(&total)
        var rows []StockMovement
        if err := q.Order("created_at DESC").Offset(utils.Offset(page, pageSize)).Limit(pageSize).Find(&rows).Error; err != nil {
            response.ErrorInternal(c, "读取库存流水失败")
            return
        }
        response.SuccessPage(c, rows, total, page, pageSize)
    }
}

func handleStockAlerts(db *gorm.DB) gin.HandlerFunc {
    return func(c *gin.Context) {
        book, _ := getBook(db, contextBookID(c))
        today := todayIn(book.Timezone)
        var rows []Medicine
        if err := db.Where("book_id = ? AND archived_at IS NULL", book.ID).Find(&rows).Error; err != nil {
            response.ErrorInternal(c, "读取库存预警失败")
            return
        }
        low, expiring, expired := make([]MedicineDTO, 0), make([]MedicineDTO, 0), make([]MedicineDTO, 0)
        for _, m := range rows {
            dto := medicineToDTO(db, m, today)
            switch dto.StockStatus {
            case "out", "low":
                low = append(low, dto)
            case "expiring":
                expiring = append(expiring, dto)
            case "expired":
                expired = append(expired, dto)
            }
        }
        sort.Slice(expiring, func(i, j int) bool { return expiring[i].NearestExpiry < expiring[j].NearestExpiry })
        response.Success(c, gin.H{"low_stock": low, "expiring": expiring, "expired": expired})
    }
}

func handleListLocations(db *gorm.DB) gin.HandlerFunc {
    return func(c *gin.Context) {
        var rows []CareLocation
        q := db.Where("book_id = ?", contextBookID(c))
        if c.Query("include_archived") != "1" {
            q = q.Where("archived_at IS NULL")
        }
        if err := q.Order("type ASC, name ASC").Find(&rows).Error; err != nil {
            response.ErrorInternal(c, "读取地点失败")
            return
        }
        response.Success(c, rows)
    }
}

func handleCreateLocation(db *gorm.DB) gin.HandlerFunc {
    return func(c *gin.Context) {
        var in struct {
            Name         string   `json:"name"`
            Type         string   `json:"type"`
            Address      string   `json:"address"`
            ContactName  string   `json:"contact_name"`
            ContactPhone string   `json:"contact_phone"`
            Latitude     *float64 `json:"latitude"`
            Longitude    *float64 `json:"longitude"`
            Notes        string   `json:"notes"`
        }
        if err := c.ShouldBindJSON(&in); err != nil || strings.TrimSpace(in.Name) == "" {
            response.ErrorBadRequest(c, "地点名称不能为空")
            return
        }
        if in.Type == "" {
            in.Type = "other"
        }
        now := nowUTC()
        loc := CareLocation{BookID: contextBookID(c), Name: strings.TrimSpace(in.Name), Type: in.Type, Address: in.Address, ContactName: in.ContactName, ContactPhone: in.ContactPhone, Latitude: in.Latitude, Longitude: in.Longitude, Notes: in.Notes, CreatedBy: currentUserID(c), CreatedAt: now, UpdatedAt: now}
        if err := db.Create(&loc).Error; err != nil {
            response.ErrorInternal(c, "创建地点失败")
            return
        }
        audit.Log(db, c, "medcare_location_create", "care_location", loc.ID, loc.Name)
        response.Success(c, loc)
    }
}

func handleUpdateLocation(db *gorm.DB) gin.HandlerFunc {
    return func(c *gin.Context) {
        id, ok := parseUintParam(c, "id")
        if !ok {
            return
        }
        var loc CareLocation
        if err := db.Where("id = ? AND book_id = ?", id, contextBookID(c)).First(&loc).Error; err != nil {
            response.ErrorNotFound(c, "地点不存在")
            return
        }
        var in struct {
            Name         string   `json:"name"`
            Type         string   `json:"type"`
            Address      string   `json:"address"`
            ContactName  string   `json:"contact_name"`
            ContactPhone string   `json:"contact_phone"`
            Latitude     *float64 `json:"latitude"`
            Longitude    *float64 `json:"longitude"`
            Notes        string   `json:"notes"`
        }
        if err := c.ShouldBindJSON(&in); err != nil {
            response.ErrorBadRequest(c, "参数无效")
            return
        }
        if name := strings.TrimSpace(in.Name); name != "" {
            loc.Name = name
        }
        if in.Type != "" {
            loc.Type = in.Type
        }
        loc.Address = in.Address
        loc.ContactName = in.ContactName
        loc.ContactPhone = in.ContactPhone
        loc.Latitude = in.Latitude
        loc.Longitude = in.Longitude
        loc.Notes = in.Notes
        loc.UpdatedAt = nowUTC()
        if err := db.Save(&loc).Error; err != nil {
            response.ErrorInternal(c, "保存地点失败")
            return
        }
        audit.Log(db, c, "medcare_location_update", "care_location", loc.ID, loc.Name)
        response.Success(c, loc)
    }
}

func handleArchiveLocation(db *gorm.DB, archive bool) gin.HandlerFunc {
    return func(c *gin.Context) {
        id, ok := parseUintParam(c, "id")
        if !ok {
            return
        }
        var loc CareLocation
        if err := db.Where("id = ? AND book_id = ?", id, contextBookID(c)).First(&loc).Error; err != nil {
            response.ErrorNotFound(c, "地点不存在")
            return
        }
        if archive {
            now := nowUTC()
            loc.ArchivedAt = &now
        } else {
            loc.ArchivedAt = nil
        }
        loc.UpdatedAt = nowUTC()
        if err := db.Save(&loc).Error; err != nil {
            response.ErrorInternal(c, "更新地点失败")
            return
        }
        response.Success(c, gin.H{"ok": true})
    }
}

// BatchDTO 是批次对外的结构：在批次字段之上补「来源渠道」与「是否还能改入库记录」，
// 前端据此决定批次卡片上要不要显示「修改入库」。
type BatchDTO struct {
    MedicineBatch
    Channel   string `json:"channel"`
    Editable  bool   `json:"editable"`
    EditUntil string `json:"edit_until,omitempty"`
    // MedicineName 是这条入库记录当前归属的药品名；
    // MedicineMissing 为 true 表示该药品已被删除（归档），需要用户重新选择归属。
    MedicineName    string `json:"medicine_name"`
    MedicineMissing bool   `json:"medicine_missing"`
}

// batchEditWindow 是入库记录允许修改的时间窗。
const batchEditWindow = 7 * 24 * time.Hour

// buildBatchDTOs 把批次补上来源渠道与可修改状态（以该批次最早那条入库流水的创建时间为准）。
func buildBatchDTOs(db *gorm.DB, bookID uint, rows []MedicineBatch) []BatchDTO {
    out := make([]BatchDTO, 0, len(rows))
    if len(rows) == 0 {
        return out
    }
    ids := make([]uint, 0, len(rows))
    for _, b := range rows {
        ids = append(ids, b.ID)
    }
    var mvs []StockMovement
    db.Where("book_id = ? AND type = ? AND batch_id IN ?", bookID, "in", ids).Order("id ASC").Find(&mvs)
    mvByBatch := map[uint]StockMovement{}
    for _, mv := range mvs {
        if mv.BatchID == nil {
            continue
        }
        if _, ok := mvByBatch[*mv.BatchID]; !ok {
            mvByBatch[*mv.BatchID] = mv
        }
    }
    // 一次把涉及的药品查出来（含已归档的），用于显示归属与判断是否还存在。
    medIDs := make([]uint, 0, len(rows))
    seenMed := map[uint]bool{}
    for _, b := range rows {
        if !seenMed[b.MedicineID] {
            seenMed[b.MedicineID] = true
            medIDs = append(medIDs, b.MedicineID)
        }
    }
    type medBrief struct {
        ID         uint
        Name       string
        ArchivedAt *time.Time
    }
    var medRows []medBrief
    medByID := map[uint]medBrief{}
    if len(medIDs) > 0 {
        db.Model(&Medicine{}).Select("id, name, archived_at").Where("book_id = ? AND id IN ?", bookID, medIDs).Scan(&medRows)
        for _, m := range medRows {
            medByID[m.ID] = m
        }
    }
    now := nowUTC()
    for _, b := range rows {
        dto := BatchDTO{MedicineBatch: b}
        if m, ok := medByID[b.MedicineID]; ok {
            dto.MedicineName = m.Name
            dto.MedicineMissing = m.ArchivedAt != nil
        } else {
            dto.MedicineMissing = true
        }
        if mv, ok := mvByBatch[b.ID]; ok {
            dto.Channel = mv.Channel
            if now.Sub(mv.CreatedAt) <= batchEditWindow {
                dto.Editable = true
                dto.EditUntil = mv.CreatedAt.Add(batchEditWindow).In(loadLocation(BeijingTimezone)).Format("2006-01-02 15:04")
            } else if dto.MedicineMissing {
                // 药品已被删除：允许随时把它改挂到其它药品上，属于数据修复。
                dto.Editable = true
            }
        }
        out = append(out, dto)
    }
    return out
}

func handleListBatches(db *gorm.DB) gin.HandlerFunc {
    return func(c *gin.Context) {
        id, ok := parseUintParam(c, "id")
        if !ok {
            return
        }
        var rows []MedicineBatch
        if err := db.Where("book_id = ? AND medicine_id = ?", contextBookID(c), id).Order("CASE WHEN expiry_date = '' THEN 1 ELSE 0 END, expiry_date ASC, received_at ASC").Find(&rows).Error; err != nil {
            response.ErrorInternal(c, "读取批次失败")
            return
        }
        response.Success(c, buildBatchDTOs(db, contextBookID(c), rows))
    }
}

func handleGetBatch(db *gorm.DB) gin.HandlerFunc {
    return func(c *gin.Context) {
        batchID, ok := parseUintParam(c, "batchId")
        if !ok {
            return
        }
        var b MedicineBatch
        if err := db.Where("id = ? AND book_id = ?", batchID, contextBookID(c)).First(&b).Error; err != nil {
            response.ErrorNotFound(c, "批次不存在")
            return
        }
        dtos := buildBatchDTOs(db, contextBookID(c), []MedicineBatch{b})
        response.Success(c, dtos[0])
    }
}

// handleUpdateStockInBatch 修改一条入库记录（仅限入库后 7 天内）。
// 只允许在「已消耗数量」之上调整入库总量：batch 的剩余量随差额一起变，
// 药品库存按批次重算，对应的入库流水数量同步改写，避免账实不符。
func handleUpdateStockInBatch(db *gorm.DB) gin.HandlerFunc {
    return func(c *gin.Context) {
        batchID, ok := parseUintParam(c, "batchId")
        if !ok {
            return
        }
        var b MedicineBatch
        if err := db.Where("id = ? AND book_id = ?", batchID, contextBookID(c)).First(&b).Error; err != nil {
            response.ErrorNotFound(c, "批次不存在")
            return
        }
        var in struct {
            Quantity   float64 `json:"quantity"`
            BatchNo    string  `json:"batch_no"`
            ExpiryDate string  `json:"expiry_date"`
            Supplier   string  `json:"supplier"`
            Note       string  `json:"note"`
            Channel    string  `json:"channel"`
            // MedicineID 非 0 时把这条入库记录改挂到另一个药品上（录错药品时用）。
            MedicineID uint `json:"medicine_id"`
        }
        if err := c.ShouldBindJSON(&in); err != nil {
            response.ErrorBadRequest(c, "参数无效")
            return
        }
        if in.Quantity <= 0 {
            response.ErrorBadRequest(c, "入库数量必须大于 0")
            return
        }
        if in.ExpiryDate != "" {
            if _, err := time.Parse("2006-01-02", in.ExpiryDate); err != nil {
                response.ErrorBadRequest(c, "有效期格式应为 YYYY-MM-DD")
                return
            }
        }
        var mv StockMovement
        if err := db.Where("book_id = ? AND batch_id = ? AND type = ?", b.BookID, b.ID, "in").Order("id ASC").First(&mv).Error; err != nil {
            response.ErrorBadRequest(c, "这条批次不是手动入库记录，不能在这里修改")
            return
        }
        now := nowUTC()
        // 归属药品已被删除时属于数据修复，不受 7 天限制：否则这条库存永远改不回来。
        var owner Medicine
        ownerMissing := true
        if err := db.Where("id = ? AND book_id = ?", b.MedicineID, b.BookID).First(&owner).Error; err == nil {
            ownerMissing = owner.ArchivedAt != nil
        }
        if !ownerMissing && now.Sub(mv.CreatedAt) > batchEditWindow {
            response.ErrorBadRequest(c, "入库已超过 7 天，不能再修改")
            return
        }
        consumed := b.QuantityInitial - b.QuantityRemaining
        if consumed < 0 {
            consumed = 0
        }
        if in.Quantity < consumed-0.000001 {
            response.ErrorBadRequest(c, fmt.Sprintf("该批次已消耗 %.3g，入库数量不能小于它", consumed))
            return
        }
        channel := normalizeStockChannel(in.Channel)
        if channel == "" {
            channel = mv.Channel
        }
        if channel == "" {
            channel = StockChannelOther
        }
        // 改归属药品：新药品必须存在、属于同一个用药本且未被删除。
        newMedID := b.MedicineID
        if in.MedicineID > 0 && in.MedicineID != b.MedicineID {
            var target Medicine
            if err := db.Where("id = ? AND book_id = ? AND archived_at IS NULL", in.MedicineID, b.BookID).First(&target).Error; err != nil {
                response.ErrorBadRequest(c, "目标药品不存在或已被删除")
                return
            }
            newMedID = target.ID
        }
        err := db.Transaction(func(tx *gorm.DB) error {
            oldMedID := b.MedicineID
            if newMedID != b.MedicineID {
                b.MedicineID = newMedID
            }
            b.QuantityInitial = in.Quantity
            b.QuantityRemaining = in.Quantity - consumed
            b.BatchNo = strings.TrimSpace(in.BatchNo)
            b.ExpiryDate = in.ExpiryDate
            b.Supplier = strings.TrimSpace(in.Supplier)
            b.Note = in.Note
            b.UpdatedAt = now
            if err := tx.Save(&b).Error; err != nil {
                return err
            }
            // 归属药品改了，流水也跟着改：这里必须写进 mv 本身，
            // 否则下面 Save(&mv) 会用旧 medicine_id 把批量更新覆盖回去。
            mv.MedicineID = b.MedicineID
            mv.QuantityDelta = in.Quantity
            mv.QuantityAfter = in.Quantity
            mv.Channel = channel
            if err := tx.Save(&mv).Error; err != nil {
                return err
            }
            // 同一批次可能还有其它流水（比如盘点/丢弃），一并搬走保证归属一致。
            if oldMedID != b.MedicineID {
                if err := tx.Model(&StockMovement{}).Where("book_id = ? AND batch_id = ? AND id <> ?", b.BookID, b.ID, mv.ID).Update("medicine_id", b.MedicineID).Error; err != nil {
                    return err
                }
            }
            updateMedicineStock(tx, b.BookID, b.MedicineID)
            // 换了归属药品时，原药品的库存也要重算，否则旧药还挂着这份库存。
            if oldMedID != b.MedicineID {
                updateMedicineStock(tx, b.BookID, oldMedID)
            }
            return nil
        })
        if err != nil {
            response.ErrorInternal(c, "保存入库记录失败")
            return
        }
        audit.Log(db, c, "medcare_stock_in_update", "medicine", b.MedicineID, fmt.Sprintf("%.3g", in.Quantity))
        var m Medicine
        m.ID = b.MedicineID
        m.BookID = b.BookID
        reloadMedicine(db, &m)
        book, _ := getBook(db, b.BookID)
        dtos := buildBatchDTOs(db, b.BookID, []MedicineBatch{b})
        response.Success(c, gin.H{"batch": dtos[0], "medicine": medicineToDTO(db, m, todayIn(book.Timezone))})
    }
}
