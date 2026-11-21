package medcare

import (
    "sort"
    "time"

    "smallgo/server/response"

    "github.com/gin-gonic/gin"
    "gorm.io/gorm"
)

type StatsOverview struct {
    From            string            `json:"from"`
    To              string            `json:"to"`
    Total           int64             `json:"total"`
    Done            int64             `json:"done"`
    Missed          int64             `json:"missed"`
    MissedEver      int64             `json:"missed_ever"`
    SkippedDoctor   int64             `json:"skipped_doctor"`
    SkippedSelf     int64             `json:"skipped_self"`
    SkippedOther    int64             `json:"skipped_other"`
    Pending         int64             `json:"pending"`
    CompletionRate  float64           `json:"completion_rate"`
    OnTimeRate      float64           `json:"on_time_rate"`
    MissedDays      int               `json:"missed_days"`
    CleanStreakDays int               `json:"clean_streak_days"`
    Definitions     map[string]string `json:"definitions"`
}

func statsRange(c *gin.Context) (string, string, bool) {
    to := c.Query("to")
    if to == "" {
        to = nowUTC().Format("2006-01-02")
    }
    from := c.Query("from")
    if from == "" {
        t, err := time.Parse("2006-01-02", to)
        if err != nil {
            response.ErrorBadRequest(c, "日期格式无效")
            return "", "", false
        }
        from = t.AddDate(0, 0, -29).Format("2006-01-02")
    }
    f, err1 := time.Parse("2006-01-02", from)
    t, err2 := time.Parse("2006-01-02", to)
    if err1 != nil || err2 != nil || t.Before(f) {
        response.ErrorBadRequest(c, "日期范围无效")
        return "", "", false
    }
    if t.Sub(f) > 366*24*time.Hour {
        response.ErrorBadRequest(c, "统计范围最长 366 天")
        return "", "", false
    }
    return from, to, true
}

func statsEvents(db *gorm.DB, bookID uint, from, to string) []DoseEvent {
    var rows []DoseEvent
    db.Where("book_id = ? AND local_date BETWEEN ? AND ?", bookID, from, to).Order("scheduled_at ASC").Find(&rows)
    return rows
}

func handleStatsOverview(db *gorm.DB) gin.HandlerFunc {
    return func(c *gin.Context) {
        from, to, ok := statsRange(c)
        if !ok {
            return
        }
        events := statsEvents(db, contextBookID(c), from, to)
        out := StatsOverview{From: from, To: to, Definitions: map[string]string{"completion_rate": "done / (done + skipped + missed)"}}
        missedDays := map[string]bool{}
        for _, ev := range events {
            out.Total++
            switch ev.Status {
            case EventDone:
                out.Done++
            case EventMissed:
                out.Missed++
                missedDays[ev.LocalDate] = true
            case EventSkipped:
                switch ev.SkipReason {
                case SkipDoctor:
                    out.SkippedDoctor++
                case SkipSelf:
                    out.SkippedSelf++
                default:
                    out.SkippedOther++
                }
            case EventPending:
                out.Pending++
            }
            if ev.Status == EventMissed || ev.WasMissed {
                out.MissedEver++
            }
        }
        denom := out.Done + out.SkippedDoctor + out.SkippedSelf + out.SkippedOther + out.Missed
        if denom > 0 {
            out.CompletionRate = float64(out.Done) / float64(denom)
        }
        out.MissedDays = len(missedDays)
        out.CleanStreakDays = cleanStreak(events, from, to)
        response.Success(c, out)
    }
}

// cleanStreak 连续无漏服天数：从最近一天往前数，只统计「有用药记录且无漏服」
// 的天；遇到漏服或完全没有用药记录的一天就停。当天只有待完成（还没到点）时
// 不算断，也不计入，从昨天继续数——避免新建用药本显示整个统计窗口的天数。
func cleanStreak(events []DoseEvent, from, to string) int {
    missedDate := map[string]bool{}
    processed := map[string]bool{}
    for _, ev := range events {
        if ev.Status == EventMissed {
            missedDate[ev.LocalDate] = true
        }
        if ev.Status != EventPending {
            processed[ev.LocalDate] = true
        }
    }
    streak := 0
    day, err := time.Parse("2006-01-02", to)
    if err != nil {
        return 0
    }
    start, err := time.Parse("2006-01-02", from)
    if err != nil {
        return 0
    }
    today := nowUTC().In(loadLocation(BeijingTimezone)).Format("2006-01-02")
    for !day.Before(start) {
        key := day.Format("2006-01-02")
        if missedDate[key] {
            break
        }
        if !processed[key] {
            // 当天还没过完：只有待完成或还没有事件时跳过，不算也不断 streak
            if key == today {
                day = day.AddDate(0, 0, -1)
                continue
            }
            break
        }
        streak++
        day = day.AddDate(0, 0, -1)
    }
    return streak
}

func handleStatsTrend(db *gorm.DB) gin.HandlerFunc {
    return func(c *gin.Context) {
        from, to, ok := statsRange(c)
        if !ok {
            return
        }
        events := statsEvents(db, contextBookID(c), from, to)
        type row struct {
            Date          string `json:"date"`
            Done          int64  `json:"done"`
            Missed        int64  `json:"missed"`
            SkippedDoctor int64  `json:"skipped_doctor"`
            SkippedSelf   int64  `json:"skipped_self"`
            SkippedOther  int64  `json:"skipped_other"`
        }
        m := map[string]*row{}
        for _, ev := range events {
            if m[ev.LocalDate] == nil {
                m[ev.LocalDate] = &row{Date: ev.LocalDate}
            }
            r := m[ev.LocalDate]
            switch ev.Status {
            case EventDone:
                r.Done++
            case EventMissed:
                r.Missed++
            case EventSkipped:
                switch ev.SkipReason {
                case SkipDoctor:
                    r.SkippedDoctor++
                case SkipSelf:
                    r.SkippedSelf++
                default:
                    r.SkippedOther++
                }
            }
        }
        rows := make([]row, 0, len(m))
        for _, r := range m {
            rows = append(rows, *r)
        }
        sort.Slice(rows, func(i, j int) bool { return rows[i].Date < rows[j].Date })
        response.Success(c, rows)
    }
}

func handleStatsHeatmap(db *gorm.DB) gin.HandlerFunc {
    return func(c *gin.Context) {
        from, to, ok := statsRange(c)
        if !ok {
            return
        }
        events := statsEvents(db, contextBookID(c), from, to)
        type row struct {
            Date    string `json:"date"`
            Total   int64  `json:"total"`
            Done    int64  `json:"done"`
            Missed  int64  `json:"missed"`
            Skipped int64  `json:"skipped"`
        }
        m := map[string]*row{}
        for _, ev := range events {
            if m[ev.LocalDate] == nil {
                m[ev.LocalDate] = &row{Date: ev.LocalDate}
            }
            r := m[ev.LocalDate]
            r.Total++
            switch ev.Status {
            case EventDone:
                r.Done++
            case EventMissed:
                r.Missed++
            case EventSkipped:
                r.Skipped++
            }
        }
        rows := make([]row, 0, len(m))
        for _, r := range m {
            rows = append(rows, *r)
        }
        sort.Slice(rows, func(i, j int) bool { return rows[i].Date < rows[j].Date })
        response.Success(c, rows)
    }
}

func handleStatsBreakdown(db *gorm.DB) gin.HandlerFunc {
    return func(c *gin.Context) {
        from, to, ok := statsRange(c)
        if !ok {
            return
        }
        events := statsEvents(db, contextBookID(c), from, to)
        byMedicine := map[string]int64{}
        byType := map[string]int64{}
        byPlan := map[string]int64{}
        byTime := map[string]int64{}
        byReason := map[string]int64{}
        byLocation := map[string]int64{}
        byLocationType := map[string]int64{}
        byExecutor := map[string]int64{}
        // 地点类型来自地点库；事件只存快照名称，统计类型需要按 location_id 关联。
        var locations []CareLocation
        db.Where("book_id = ?", contextBookID(c)).Find(&locations)
        locationTypes := make(map[uint]string, len(locations))
        for _, loc := range locations {
            locationTypes[loc.ID] = loc.Type
        }
        for _, ev := range events {
            byMedicine[ev.MedicineName]++
            byType[ev.Type]++
            byPlan[ev.PlanTitle]++
            byTime[timeBucket(ev.LocalTime)]++
            if ev.Status == EventSkipped {
                byReason[ev.SkipReason]++
            }
            // 地点与执行人是打针/输液场景的维度，口服等不计入，
            // 避免纯口服用户看到一堆「未指定地点/未指定执行人」。
            if ev.Type != "injection" && ev.Type != "infusion" {
                continue
            }
            locationName := ev.LocationName
            if locationName == "" {
                locationName = "未指定地点"
            }
            byLocation[locationName]++
            locationType := "unspecified"
            if ev.LocationID != nil {
                if t, ok := locationTypes[*ev.LocationID]; ok && t != "" {
                    locationType = t
                } else {
                    locationType = "other"
                }
            }
            byLocationType[locationType]++
            executor := ev.ExecutorName
            if executor == "" {
                switch ev.ExecutorType {
                case "self":
                    executor = "自己"
                case "family":
                    executor = "家人"
                case "nurse":
                    executor = "护士"
                case "doctor":
                    executor = "医生"
                case "other":
                    executor = "其他执行人"
                default:
                    executor = "未指定执行人"
                }
            }
            byExecutor[executor]++
        }
        response.Success(c, gin.H{"by_medicine": byMedicine, "by_type": byType, "by_plan": byPlan, "by_time": byTime, "by_reason": byReason, "by_location": byLocation, "by_location_type": byLocationType, "by_executor": byExecutor})
    }
}

func handleStatsStock(db *gorm.DB) gin.HandlerFunc {
    return func(c *gin.Context) {
        book, _ := getBook(db, contextBookID(c))
        var medicines []Medicine
        if err := db.Where("book_id = ? AND archived_at IS NULL", book.ID).Find(&medicines).Error; err != nil {
            response.ErrorInternal(c, "读取库存统计失败")
            return
        }
        type row struct {
            ID            uint    `json:"id"`
            Name          string  `json:"name"`
            StockQuantity float64 `json:"stock_quantity"`
            Unit          string  `json:"unit"`
            LowStock      bool    `json:"low_stock"`
            NearestExpiry string  `json:"nearest_expiry"`
        }
        // 整本药品的最近效期一次查回，避免按药品逐条查询
        medicineIDs := make([]uint, 0, len(medicines))
        for _, m := range medicines {
            medicineIDs = append(medicineIDs, m.ID)
        }
        nearest, _, err := batchExpiryStats(db, book.ID, medicineIDs, todayIn(book.Timezone))
        if err != nil {
            response.ErrorInternal(c, "读取库存统计失败")
            return
        }
        rows := make([]row, 0, len(medicines))
        for _, m := range medicines {
            rows = append(rows, row{ID: m.ID, Name: m.Name, StockQuantity: m.StockQuantity, Unit: m.Unit, LowStock: m.LowStockThreshold > 0 && m.StockQuantity <= m.LowStockThreshold, NearestExpiry: nearest[m.ID]})
        }
        response.Success(c, rows)
    }
}
