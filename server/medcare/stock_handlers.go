package medcare

import (
    "strings"
    "time"

    "smallgo/server/response"
    "smallgo/server/utils"

    "github.com/gin-gonic/gin"
    "gorm.io/gorm"
)

// normalizeStockChannel 收敛入库渠道，未知值一律置空（前端显示「未标注」）。
func normalizeStockChannel(v string) string {
    switch strings.TrimSpace(v) {
    case StockChannelPharmacy, StockChannelHospital, StockChannelOther:
        return strings.TrimSpace(v)
    default:
        return ""
    }
}

// normalizeStockEntrySource 收敛录入来源。
func normalizeStockEntrySource(v string) string {
    switch strings.TrimSpace(v) {
    case StockEntryManual, StockEntryBox, StockEntryReceipt, StockEntryPrescription,
        StockEntryCompletion, StockEntryReturn, StockEntryAdjust, StockEntryWaste, StockEntryStocktake:
        return strings.TrimSpace(v)
    default:
        return ""
    }
}

type stockMovementDTO struct {
    ID            uint      `json:"id"`
    MedicineID    uint      `json:"medicine_id"`
    BatchID       *uint     `json:"batch_id,omitempty"`
    MedicineName  string    `json:"medicine_name"`
    // MedicineMissing 表示这条记录归属的药品已被删除，需要提示用户重新选择。
    MedicineMissing bool    `json:"medicine_missing"`
    Unit          string    `json:"unit"`
    Type          string    `json:"type"`
    QuantityDelta float64   `json:"quantity_delta"`
    QuantityAfter float64   `json:"quantity_after"`
    Reason        string    `json:"reason"`
    Channel       string    `json:"channel"`
    EntrySource   string    `json:"entry_source"`
    ActorUserID   uint      `json:"actor_user_id"`
    ActorName     string    `json:"actor_name"`
    EventID       *uint     `json:"event_id,omitempty"`
    // 关联用药事件的时间：计划时间（应服）与实际点完成的时间，方便核对。
    EventLocalDate   string     `json:"event_local_date,omitempty"`
    EventLocalTime   string     `json:"event_local_time,omitempty"`
    EventStatus      string     `json:"event_status,omitempty"`
    EventCompletedAt *time.Time `json:"event_completed_at,omitempty"`
    CreatedAt     time.Time `json:"created_at"`
}

// handleListBookStockMovements 用药本级出入库明细，支持按药品、类型、渠道、录入来源筛选。
// movementFilterQuery 按列表与统计共用的条件拼查询（药品/类型/来源/关键词/时间范围）。
func movementFilterQuery(db *gorm.DB, c *gin.Context, bookID uint) *gorm.DB {
    q := db.Model(&StockMovement{}).Where("book_id = ?", bookID)
        if v := strings.TrimSpace(c.Query("medicine_id")); v != "" {
            q = q.Where("medicine_id = ?", utils.Atoi(v, 0))
        }
        if v := strings.TrimSpace(c.Query("type")); v != "" {
            q = q.Where("type = ?", v)
        }
        if v := strings.TrimSpace(c.Query("channel")); v != "" {
            q = q.Where("channel = ?", normalizeStockChannel(v))
        }
        if v := strings.TrimSpace(c.Query("entry_source")); v != "" {
            q = q.Where("entry_source = ?", normalizeStockEntrySource(v))
        }
        // 关键词按药品名模糊匹配（先取本用药本里命中的药品 id，避免 join 影响分页计数）。
        if v := strings.TrimSpace(c.Query("keyword")); v != "" {
            var ids []uint
            db.Model(&Medicine{}).Where("book_id = ? AND name LIKE ?", bookID, "%"+v+"%").Pluck("id", &ids)
            if len(ids) == 0 {
                // 关键词没命中任何药品：返回空结果集条件，列表与统计都自然为空。
                return q.Where("1 = 0")
            }
            q = q.Where("medicine_id IN ?", ids)
        }
        // 录入时间段：按流水的创建时间（也就是「这条记录是什么时候录的」）过滤。
        // 时间用 RFC3339（前端按北京时间换算成 UTC）。
        if v := strings.TrimSpace(c.Query("from")); v != "" {
            if t, err := time.Parse(time.RFC3339, v); err == nil {
                q = q.Where("created_at >= ?", t)
            }
        }
        if v := strings.TrimSpace(c.Query("to")); v != "" {
            if t, err := time.Parse(time.RFC3339, v); err == nil {
                q = q.Where("created_at <= ?", t)
            }
        }
        // 批次时间段：按该条流水所对应批次的入库时间（medicine_batches.received_at）过滤。
        // 与「录入时间」分开，是为了查「某段时间入的那批药现在怎么用的」。
        batchFrom := strings.TrimSpace(c.Query("batch_from"))
        batchTo := strings.TrimSpace(c.Query("batch_to"))
        if batchFrom != "" || batchTo != "" {
            bq := db.Model(&MedicineBatch{}).Where("book_id = ?", bookID)
            if batchFrom != "" {
                if t, err := time.Parse(time.RFC3339, batchFrom); err == nil {
                    bq = bq.Where("received_at >= ?", t)
                }
            }
            if batchTo != "" {
                if t, err := time.Parse(time.RFC3339, batchTo); err == nil {
                    bq = bq.Where("received_at <= ?", t)
                }
            }
            var batchIDs []uint
            bq.Pluck("id", &batchIDs)
            if len(batchIDs) == 0 {
                return q.Where("1 = 0")
            }
            q = q.Where("batch_id IN ?", batchIDs)
        }
    return q
}

// handleStockMovementStats 统计出入库明细的条数与数量合计（按类型分组，条件与列表一致）。
// handleStockMovementStats 统计出入库明细的条数与数量合计（按类型分组，条件与列表一致）。
func handleStockMovementStats(db *gorm.DB) gin.HandlerFunc {
    return func(c *gin.Context) {
        q := movementFilterQuery(db, c, contextBookID(c))
        var rows []struct {
            Type  string
            Count int64
            Sum   float64
        }
        if err := q.Select("type, COUNT(*) AS count, COALESCE(SUM(ABS(quantity_delta)),0) AS sum").Group("type").Scan(&rows).Error; err != nil {
            response.ErrorInternal(c, "读取出入库统计失败")
            return
        }
        type agg struct {
            Count int64   `json:"count"`
            Sum   float64 `json:"sum"`
        }
        stats := map[string]agg{"in": {}, "out": {}, "adjust": {}}
        var totalCount int64
        var totalSum float64
        for _, r := range rows {
            key := "adjust"
            switch strings.TrimSpace(r.Type) {
            case "in":
                key = "in"
            case "out":
                key = "out"
            }
            cur := stats[key]
            cur.Count += r.Count
            cur.Sum += r.Sum
            stats[key] = cur
            totalCount += r.Count
            totalSum += r.Sum
        }
        response.Success(c, gin.H{
            "in":     stats["in"],
            "out":    stats["out"],
            "adjust": stats["adjust"],
            "total":  agg{Count: totalCount, Sum: totalSum},
        })
    }
}

func handleListBookStockMovements(db *gorm.DB) gin.HandlerFunc {
    return func(c *gin.Context) {
        bookID := contextBookID(c)
        page := utils.Atoi(c.Query("page"), 1)
        pageSize := utils.Atoi(c.Query("pageSize"), 20)
        if page < 1 {
            page = 1
        }
        if pageSize <= 0 || pageSize > 100 {
            pageSize = 20
        }
        q := movementFilterQuery(db, c, bookID)
        orderBy := "created_at DESC, id DESC"
        switch strings.TrimSpace(c.Query("sort")) {
        case "time_asc":
            orderBy = "created_at ASC, id ASC"
        case "medicine":
            orderBy = "medicine_id ASC, created_at DESC"
        }
        var total int64
        if err := q.Count(&total).Error; err != nil {
            response.ErrorInternal(c, "读取出入库明细失败")
            return
        }
        var rows []StockMovement
        if err := q.Order(orderBy).Offset(utils.Offset(page, pageSize)).Limit(pageSize).Find(&rows).Error; err != nil {
            response.ErrorInternal(c, "读取出入库明细失败")
            return
        }
        items := make([]stockMovementDTO, 0, len(rows))
        if len(rows) == 0 {
            response.SuccessPage(c, items, total, page, pageSize)
            return
        }
        medIDs := make([]uint, 0, len(rows))
        actorIDs := make([]uint, 0, len(rows))
        seenMed := map[uint]bool{}
        seenActor := map[uint]bool{}
        for _, r := range rows {
            if !seenMed[r.MedicineID] {
                seenMed[r.MedicineID] = true
                medIDs = append(medIDs, r.MedicineID)
            }
            if r.ActorUserID > 0 && !seenActor[r.ActorUserID] {
                seenActor[r.ActorUserID] = true
                actorIDs = append(actorIDs, r.ActorUserID)
            }
        }
        meds := map[uint]Medicine{}
        var medRows []Medicine
        if err := db.Where("book_id = ? AND id IN ?", bookID, medIDs).Find(&medRows).Error; err == nil {
            for _, m := range medRows {
                meds[m.ID] = m
            }
        }
        // 已删除（归档）的药品：记录仍然保留，但要告诉前端「归属药品已不存在」。
        missing := map[uint]bool{}
        for _, id := range medIDs {
            if m, ok := meds[id]; !ok || m.ArchivedAt != nil {
                missing[id] = true
            }
        }
        // 关联事件的时间信息（计划时间 / 完成时间），只查本次页面上出现的 event_id。
        evIDs := make([]uint, 0, len(rows))
        seenEv := map[uint]bool{}
        for _, r := range rows {
            if r.EventID != nil && !seenEv[*r.EventID] {
                seenEv[*r.EventID] = true
                evIDs = append(evIDs, *r.EventID)
            }
        }
        events := map[uint]DoseEvent{}
        if len(evIDs) > 0 {
            var evRows []DoseEvent
            if err := db.Where("book_id = ? AND id IN ?", bookID, evIDs).Find(&evRows).Error; err == nil {
                for _, e := range evRows {
                    events[e.ID] = e
                }
            }
        }
        actors := map[uint]string{}
        if len(actorIDs) > 0 {
            var userRows []struct {
                ID       uint
                Username string
            }
            if err := db.Table("users").Select("id, username").Where("id IN ?", actorIDs).Scan(&userRows).Error; err == nil {
                for _, u := range userRows {
                    actors[u.ID] = u.Username
                }
            }
        }
        for _, r := range rows {
            m := meds[r.MedicineID]
            var ev DoseEvent
            if r.EventID != nil {
                ev = events[*r.EventID]
            }
            items = append(items, stockMovementDTO{
                ID:            r.ID,
                MedicineID:    r.MedicineID,
                BatchID:       r.BatchID,
                MedicineName:  m.Name,
                MedicineMissing: missing[r.MedicineID],
                Unit:          m.Unit,
                Type:          r.Type,
                QuantityDelta: r.QuantityDelta,
                QuantityAfter: r.QuantityAfter,
                Reason:        r.Reason,
                Channel:       r.Channel,
                EntrySource:   r.EntrySource,
                ActorUserID:   r.ActorUserID,
                ActorName:     actors[r.ActorUserID],
                EventID:       r.EventID,
                EventLocalDate:   ev.LocalDate,
                EventLocalTime:   ev.LocalTime,
                EventStatus:      ev.Status,
                EventCompletedAt: ev.CompletedAt,
                CreatedAt:     r.CreatedAt,
            })
        }
        response.SuccessPage(c, items, total, page, pageSize)
    }
}
