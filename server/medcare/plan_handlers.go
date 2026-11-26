package medcare

import (
    "errors"
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
    "gorm.io/gorm/clause"
)

// ScheduleDay 是「指定日期」频率里的一天：日期 + 当天的时间点。
type ScheduleDay struct {
    Date  string   `json:"date"`
    Times []string `json:"times"`
}

type FrequencyDTO struct {
    Type           string        `json:"type"`
    Times          []string      `json:"times,omitempty"`
    IntervalDays   int           `json:"interval_days,omitempty"`
    Weekdays       []int         `json:"weekdays,omitempty"`
    IntervalHours  int           `json:"interval_hours,omitempty"`
    StartAt        string        `json:"start_at,omitempty"`
    CustomSchedule []ScheduleDay `json:"custom_schedule,omitempty"`
}

type PlanDTO struct {
    ID                   uint         `json:"id"`
    BookID               uint         `json:"book_id"`
    MedicineID           uint         `json:"medicine_id"`
    MedicineName         string       `json:"medicine_name"`
    // MedicineMissing 表示这个计划关联的药品已被删除（归档），计划仍保留但需要改挂到别的药品，
    // 否则打卡时会因库存为 0 而提示库存不足。
    MedicineMissing bool `json:"medicine_missing"`
    LocationID           *uint        `json:"location_id,omitempty"`
    LocationName         string       `json:"location_name"`
    Address              string       `json:"address"`
    Title                string       `json:"title"`
    Type                 string       `json:"type"`
    DoseAmount           float64      `json:"dose_amount"`
    DoseUnit             string       `json:"dose_unit"`
    Frequency            FrequencyDTO `json:"frequency"`
    StartDate            string       `json:"start_date"`
    EndDate              string       `json:"end_date"`
    Timezone             string       `json:"timezone"`
    Status               string       `json:"status"`
    Doctor               string       `json:"doctor"`
    Hospital             string       `json:"hospital"`
    ExecutorType         string       `json:"executor_type"`
    ExecutorName         string       `json:"executor_name"`
    DripRate             int          `json:"drip_rate"`
    DurationMinutes      int          `json:"duration_minutes"`
    Notes                string       `json:"notes"`
    ReminderLeadMinutes  *int         `json:"reminder_lead_minutes,omitempty"`
    NotificationsEnabled bool         `json:"notifications_enabled"`
    NextEventAt          *time.Time   `json:"next_event_at,omitempty"`
    CompletedCount       int64        `json:"completed_count"`
    TotalCount           int64        `json:"total_count"`
    // 今日该计划的应完成 / 已完成数，用于「今日已完成」标记。
    TodayTotal    int64     `json:"today_total"`
    TodayDone     int64     `json:"today_done"`
    CreatedByName string    `json:"created_by_name"`
    UpdatedByName string    `json:"updated_by_name"`
    Version       uint      `json:"version"`
    CreatedAt     time.Time `json:"created_at"`
    UpdatedAt     time.Time `json:"updated_at"`
}

type DoseEventDTO struct {
    ID              uint       `json:"id"`
    BookID          uint       `json:"book_id"`
    PlanID          uint       `json:"plan_id"`
    MedicineID      uint       `json:"medicine_id"`
    MedicineName    string     `json:"medicine_name"`
    // MedicineMissing 表示这条用药记录的药品已被删除（归档）或查不到，打卡会提示库存不足。
    MedicineMissing bool   `json:"medicine_missing"`
    PlanTitle       string     `json:"plan_title"`
    Type            string     `json:"type"`
    DoseAmount      float64    `json:"dose_amount"`
    DoseUnit        string     `json:"dose_unit"`
    LocationID      *uint      `json:"location_id,omitempty"`
    LocationName    string     `json:"location_name"`
    Address         string     `json:"address"`
    ExecutorType    string     `json:"executor_type"`
    ExecutorName    string     `json:"executor_name"`
    DripRate        int        `json:"drip_rate"`
    DurationMinutes int        `json:"duration_minutes"`
    ActualStartAt   *time.Time `json:"actual_start_at,omitempty"`
    ActualEndAt     *time.Time `json:"actual_end_at,omitempty"`
    ScheduledAt     time.Time  `json:"scheduled_at"`
    LocalDate       string     `json:"local_date"`
    LocalTime       string     `json:"local_time"`
    Status          string     `json:"status"`
    OverdueMinutes  int64      `json:"overdue_minutes"`
    CompletedAt     *time.Time `json:"completed_at,omitempty"`
    SkippedAt       *time.Time `json:"skipped_at,omitempty"`
    MissedAt        *time.Time `json:"missed_at,omitempty"`
    SkipReason      string     `json:"skip_reason,omitempty"`
    Note            string     `json:"note"`
    SnoozeUntil     *time.Time `json:"snooze_until,omitempty"`
    WasMissed       bool       `json:"was_missed"`
    StockShortage   bool       `json:"stock_shortage"`
    Version         uint       `json:"version"`
}

type DashboardGroup struct {
    Key    string         `json:"key"`
    Label  string         `json:"label"`
    Events []DoseEventDTO `json:"events"`
}

type DashboardDTO struct {
    Date      string           `json:"date"`
    Book      BookDTO          `json:"book"`
    Summary   map[string]int64 `json:"summary"`
    Overdue   []DoseEventDTO   `json:"overdue"`
    Groups    []DashboardGroup `json:"groups"`
    NextEvent *DoseEventDTO    `json:"next_event,omitempty"`
    // ActivePlanCount 进行中的计划数；NextUpcoming 未来最近一次待用药（不限当天），
    // 供「今天没有安排但已有计划」的空态提示使用。
    ActivePlanCount int64         `json:"active_plan_count"`
    NextUpcoming    *DoseEventDTO `json:"next_upcoming,omitempty"`
    ServerTime      time.Time     `json:"server_time"`
}

func parsePlanFrequency(plan Plan) FrequencyDTO {
    f := FrequencyDTO{Type: plan.FrequencyType, IntervalDays: plan.IntervalDays, IntervalHours: plan.IntervalHours}
    _ = decodeJSON(plan.Times, &f.Times)
    _ = decodeJSON(plan.Weekdays, &f.Weekdays)
    if plan.FrequencyType == FreqCustomDates {
        _ = decodeJSON(plan.CustomSchedule, &f.CustomSchedule)
    }
    if plan.StartAt != nil {
        f.StartAt = plan.StartAt.Format(time.RFC3339)
    }
    return f
}

func planToDTO(db *gorm.DB, plan Plan) PlanDTO {
    var medicine Medicine
    medicineMissing := true
    if err := db.Where("id = ? AND book_id = ?", plan.MedicineID, plan.BookID).First(&medicine).Error; err == nil && medicine.ArchivedAt == nil {
        medicineMissing = false
    }
    var completed, total int64
    db.Model(&DoseEvent{}).Where("plan_id = ? AND book_id = ?", plan.ID, plan.BookID).Count(&total)
    db.Model(&DoseEvent{}).Where("plan_id = ? AND book_id = ? AND status = ?", plan.ID, plan.BookID, EventDone).Count(&completed)
    book, _ := getBook(db, plan.BookID)
    today := todayIn(book.Timezone)
    var todayTotal, todayDone int64
    db.Model(&DoseEvent{}).Where("plan_id = ? AND book_id = ? AND local_date = ?", plan.ID, plan.BookID, today).Count(&todayTotal)
    db.Model(&DoseEvent{}).Where("plan_id = ? AND book_id = ? AND local_date = ? AND status IN ?", plan.ID, plan.BookID, today, []string{EventDone, EventSkipped, EventMissed}).Count(&todayDone)
    var next DoseEvent
    var nextAt *time.Time
    if db.Where("plan_id = ? AND book_id = ? AND status = ? AND scheduled_at >= ?", plan.ID, plan.BookID, EventPending, nowUTC()).Order("scheduled_at ASC").First(&next).Error == nil {
        t := next.ScheduledAt
        nextAt = &t
    }
    return PlanDTO{
        ID: plan.ID, BookID: plan.BookID, MedicineID: plan.MedicineID, MedicineName: medicine.Name, MedicineMissing: medicineMissing,
        LocationID: plan.LocationID, LocationName: plan.LocationName, Address: plan.Address,
        Title: plan.Title, Type: plan.Type, DoseAmount: plan.DoseAmount, DoseUnit: plan.DoseUnit,
        Frequency: parsePlanFrequency(plan), StartDate: plan.StartDate, EndDate: plan.EndDate,
        Timezone: plan.Timezone, Status: plan.Status, Doctor: plan.Doctor, Hospital: plan.Hospital,
        ExecutorType: plan.ExecutorType, ExecutorName: plan.ExecutorName,
        DripRate: plan.DripRate, DurationMinutes: plan.DurationMinutes,
        Notes: plan.Notes, ReminderLeadMinutes: plan.ReminderLeadMinutes, NotificationsEnabled: plan.NotificationsEnabled,
        NextEventAt: nextAt, CompletedCount: completed, TotalCount: total,
        TodayTotal: todayTotal, TodayDone: todayDone,
        CreatedByName: userNameOrEmpty(db, plan.CreatedBy), UpdatedByName: userNameOrEmpty(db, plan.UpdatedBy),
        Version:   plan.Version,
        CreatedAt: plan.CreatedAt, UpdatedAt: plan.UpdatedAt,
    }
}

func eventToDTO(ev DoseEvent) DoseEventDTO {
    overdue := int64(0)
    if ev.Status == EventPending && ev.ScheduledAt.Before(nowUTC()) {
        overdue = int64(time.Since(ev.ScheduledAt).Minutes())
        if overdue < 0 {
            overdue = 0
        }
    }
    return DoseEventDTO{
        ID: ev.ID, BookID: ev.BookID, PlanID: ev.PlanID, MedicineID: ev.MedicineID,
        MedicineName: ev.MedicineName, PlanTitle: ev.PlanTitle, Type: ev.Type,
        DoseAmount: ev.DoseAmount, DoseUnit: ev.DoseUnit, LocationID: ev.LocationID,
        LocationName: ev.LocationName, Address: ev.Address,
        ExecutorType: ev.ExecutorType, ExecutorName: ev.ExecutorName,
        DripRate: ev.DripRate, DurationMinutes: ev.DurationMinutes,
        ActualStartAt: ev.ActualStartAt, ActualEndAt: ev.ActualEndAt, ScheduledAt: ev.ScheduledAt,
        LocalDate: ev.LocalDate, LocalTime: ev.LocalTime, Status: ev.Status, OverdueMinutes: overdue,
        CompletedAt: ev.CompletedAt, SkippedAt: ev.SkippedAt, MissedAt: ev.MissedAt,
        SkipReason: ev.SkipReason, Note: ev.Note, SnoozeUntil: ev.SnoozeUntil,
        WasMissed: ev.WasMissed, StockShortage: ev.StockShortage, Version: ev.Version,
    }
}

type planInput struct {
    MedicineID           uint         `json:"medicine_id"`
    LocationID           *uint        `json:"location_id"`
    Title                string       `json:"title"`
    Type                 string       `json:"type"`
    DoseAmount           float64      `json:"dose_amount"`
    DoseUnit             string       `json:"dose_unit"`
    Frequency            FrequencyDTO `json:"frequency"`
    StartDate            string       `json:"start_date"`
    EndDate              string       `json:"end_date"`
    Timezone             string       `json:"timezone"`
    Doctor               string       `json:"doctor"`
    Hospital             string       `json:"hospital"`
    ExecutorType         string       `json:"executor_type"`
    ExecutorName         string       `json:"executor_name"`
    DripRate             int          `json:"drip_rate"`
    DurationMinutes      int          `json:"duration_minutes"`
    Notes                string       `json:"notes"`
    ReminderLeadMinutes  *int         `json:"reminder_lead_minutes"`
    NotificationsEnabled *bool        `json:"notifications_enabled"`
    Version              uint         `json:"version"`
}

func (in *planInput) normalize(db *gorm.DB, book MedicationBook) error {
    if in.MedicineID == 0 {
        return errors.New("请选择药品")
    }
    var medicine Medicine
    if err := db.Where("id = ? AND book_id = ? AND archived_at IS NULL", in.MedicineID, book.ID).First(&medicine).Error; err != nil {
        return errors.New("药品不存在或已归档")
    }
    if in.Type == "" {
        in.Type = medicine.Category
    }
    if in.Type == "" {
        in.Type = CatOther
    }
    if in.DoseAmount <= 0 {
        return errors.New("剂量必须大于 0")
    }
    if in.DoseUnit == "" {
        in.DoseUnit = medicine.Unit
    }
    if in.StartDate == "" {
        in.StartDate = todayIn(book.Timezone)
    }
    if _, err := time.Parse("2006-01-02", in.StartDate); err != nil {
        return errors.New("开始日期格式应为 YYYY-MM-DD")
    }
    if in.EndDate != "" {
        if _, err := time.Parse("2006-01-02", in.EndDate); err != nil {
            return errors.New("结束日期格式应为 YYYY-MM-DD")
        }
        if in.EndDate < in.StartDate {
            return errors.New("结束日期不能早于开始日期")
        }
    }
    in.Timezone = normalizeTimezone(firstNonEmpty(in.Timezone, book.Timezone))
    if in.Frequency.Type == "" {
        in.Frequency.Type = FreqDaily
    }
    switch in.Frequency.Type {
    case FreqDaily, FreqEveryNDays, FreqWeekly:
        times, err := parseClock(in.Frequency.Times)
        if err != nil {
            return err
        }
        in.Frequency.Times = times
        if in.Frequency.Type == FreqEveryNDays {
            if in.Frequency.IntervalDays < 1 || in.Frequency.IntervalDays > 365 {
                return errors.New("间隔天数应为 1–365")
            }
        }
        if in.Frequency.Type == FreqWeekly {
            days := uniqueInts(in.Frequency.Weekdays)
            if len(days) == 0 {
                return errors.New("请选择星期")
            }
            for _, d := range days {
                if d < 1 || d > 7 {
                    return errors.New("星期取值应为 1–7")
                }
            }
            in.Frequency.Weekdays = days
        }
    case FreqCustomDates:
        if len(in.Frequency.CustomSchedule) == 0 {
            return errors.New("请至少添加一个用药日期")
        }
        cleaned := make([]ScheduleDay, 0, len(in.Frequency.CustomSchedule))
        seenDates := map[string]bool{}
        for _, d := range in.Frequency.CustomSchedule {
            date := strings.TrimSpace(d.Date)
            if _, err := time.Parse("2006-01-02", date); err != nil {
                return errors.New("指定日期的格式应为 YYYY-MM-DD")
            }
            if seenDates[date] {
                return errors.New("指定日期不能重复")
            }
            times, err := parseClock(d.Times)
            if err != nil || len(times) == 0 {
                return errors.New("每个指定日期至少填写一个时间")
            }
            seenDates[date] = true
            cleaned = append(cleaned, ScheduleDay{Date: date, Times: times})
        }
        sort.Slice(cleaned, func(i, j int) bool { return cleaned[i].Date < cleaned[j].Date })
        in.Frequency.CustomSchedule = cleaned
    case FreqIntervalHours:
        if in.Frequency.IntervalHours < 1 || in.Frequency.IntervalHours > 72 {
            return errors.New("间隔小时应为 1–72")
        }
        if in.Frequency.StartAt == "" {
            return errors.New("请选择起始时间")
        }
        if _, err := time.Parse(time.RFC3339, in.Frequency.StartAt); err != nil {
            return errors.New("起始时间格式无效")
        }
    default:
        return errors.New("不支持的频率")
    }
    if in.LocationID != nil && *in.LocationID > 0 {
        var loc CareLocation
        if err := db.Where("id = ? AND book_id = ?", *in.LocationID, book.ID).First(&loc).Error; err != nil {
            return errors.New("地点不存在")
        }
    } else {
        in.LocationID = nil
    }
    if in.Title == "" {
        in.Title = medicine.Name + "用药"
    }
    return nil
}

func firstNonEmpty(values ...string) string {
    for _, v := range values {
        if strings.TrimSpace(v) != "" {
            return v
        }
    }
    return ""
}

func uniqueInts(values []int) []int {
    seen := map[int]bool{}
    out := make([]int, 0, len(values))
    for _, v := range values {
        if !seen[v] {
            seen[v] = true
            out = append(out, v)
        }
    }
    sort.Ints(out)
    return out
}

func applyPlanInput(db *gorm.DB, plan *Plan, in planInput, book MedicationBook, actor uint) {
    plan.MedicineID = in.MedicineID
    plan.LocationID = in.LocationID
    if in.LocationID != nil {
        var loc CareLocation
        if err := db.Where("id = ? AND book_id = ?", *in.LocationID, book.ID).First(&loc).Error; err == nil {
            plan.LocationName = loc.Name
            plan.Address = loc.Address
        }
    } else {
        plan.LocationName = ""
        plan.Address = ""
    }
    plan.Title = in.Title
    plan.Type = in.Type
    plan.DoseAmount = in.DoseAmount
    plan.DoseUnit = in.DoseUnit
    plan.FrequencyType = in.Frequency.Type
    plan.IntervalDays = in.Frequency.IntervalDays
    plan.Weekdays = encodeJSON(in.Frequency.Weekdays)
    plan.IntervalHours = in.Frequency.IntervalHours
    plan.Times = encodeJSON(in.Frequency.Times)
    if in.Frequency.Type == FreqCustomDates {
        plan.CustomSchedule = encodeJSON(in.Frequency.CustomSchedule)
    } else {
        plan.CustomSchedule = ""
    }
    plan.AnchorDate = in.StartDate
    if in.Frequency.StartAt != "" {
        t, _ := time.Parse(time.RFC3339, in.Frequency.StartAt)
        plan.StartAt = &t
    } else {
        plan.StartAt = nil
    }
    plan.StartDate = in.StartDate
    plan.EndDate = in.EndDate
    plan.Timezone = in.Timezone
    plan.Doctor = in.Doctor
    plan.Hospital = in.Hospital
    switch in.ExecutorType {
    case "self", "family", "nurse", "doctor", "other":
        plan.ExecutorType = in.ExecutorType
    default:
        plan.ExecutorType = ""
    }
    plan.ExecutorName = in.ExecutorName
    if in.DripRate < 0 {
        in.DripRate = 0
    }
    if in.DurationMinutes < 0 {
        in.DurationMinutes = 0
    }
    plan.DripRate = in.DripRate
    plan.DurationMinutes = in.DurationMinutes
    plan.Notes = in.Notes
    plan.ReminderLeadMinutes = in.ReminderLeadMinutes
    if in.NotificationsEnabled != nil {
        plan.NotificationsEnabled = *in.NotificationsEnabled
    }
    plan.UpdatedBy = actor
    plan.UpdatedAt = nowUTC()
}

func frequencyTimes(plan Plan) []string {
    var times []string
    _ = decodeJSON(plan.Times, &times)
    return times
}

func frequencyWeekdays(plan Plan) []int {
    var days []int
    _ = decodeJSON(plan.Weekdays, &days)
    return days
}

func expandPlan(plan Plan, fromDate, toDate string) ([]time.Time, error) {
    loc := loadLocation(plan.Timezone)
    from, err := parseLocalDate(loc, fromDate)
    if err != nil {
        return nil, err
    }
    to, err := parseLocalDate(loc, toDate)
    if err != nil {
        return nil, err
    }
    if plan.StartDate != "" {
        start, err := parseLocalDate(loc, plan.StartDate)
        if err != nil {
            return nil, err
        }
        if from.Before(start) {
            from = start
        }
    }
    if plan.EndDate != "" {
        end, err := parseLocalDate(loc, plan.EndDate)
        if err == nil && to.After(end) {
            to = end
        }
    }
    if to.Before(from) {
        return []time.Time{}, nil
    }
    out := make([]time.Time, 0)
    add := func(d time.Time, clock string) {
        if len(out) >= 2000 {
            return
        }
        parts := strings.Split(clock, ":")
        if len(parts) != 2 {
            return
        }
        h, m := utils.Atoi(parts[0], 0), utils.Atoi(parts[1], 0)
        local := time.Date(d.Year(), d.Month(), d.Day(), h, m, 0, 0, loc)
        out = append(out, local.UTC())
    }
    switch plan.FrequencyType {
    case FreqCustomDates:
        var days []ScheduleDay
        _ = decodeJSON(plan.CustomSchedule, &days)
        for _, sd := range days {
            d, err := parseLocalDate(loc, sd.Date)
            if err != nil || d.Before(from) || d.After(to) {
                continue
            }
            for _, clock := range sd.Times {
                add(d, clock)
            }
        }
    case FreqDaily, FreqEveryNDays, FreqWeekly:
        times := frequencyTimes(plan)
        weekdays := map[int]bool{}
        for _, d := range frequencyWeekdays(plan) {
            weekdays[d] = true
        }
        anchor, _ := parseLocalDate(loc, firstNonEmpty(plan.AnchorDate, plan.StartDate))
        for d := from; !d.After(to); d = d.AddDate(0, 0, 1) {
            ok := true
            switch plan.FrequencyType {
            case FreqEveryNDays:
                days := int(d.Sub(anchor).Hours() / 24)
                if days < 0 || plan.IntervalDays <= 0 || days%plan.IntervalDays != 0 {
                    ok = false
                }
            case FreqWeekly:
                wd := int(d.Weekday())
                if wd == 0 {
                    wd = 7
                }
                ok = weekdays[wd]
            }
            if !ok {
                continue
            }
            for _, t := range times {
                add(d, t)
            }
        }
    case FreqIntervalHours:
        if plan.StartAt == nil || plan.IntervalHours <= 0 {
            return out, nil
        }
        cur := plan.StartAt.In(loc)
        if cur.Before(from) {
            step := time.Duration(plan.IntervalHours) * time.Hour
            delta := from.Sub(cur)
            n := int(math.Ceil(float64(delta) / float64(step)))
            cur = cur.Add(time.Duration(n) * step)
        }
        step := time.Duration(plan.IntervalHours) * time.Hour
        for !cur.After(to.Add(24*time.Hour-time.Second)) && len(out) < 2000 {
            localDate := cur.In(loc).Format("2006-01-02")
            if localDate >= fromDate && localDate <= toDate {
                out = append(out, cur.UTC())
            }
            cur = cur.Add(step)
        }
    }
    sort.Slice(out, func(i, j int) bool { return out[i].Before(out[j]) })
    dedup := out[:0]
    for _, t := range out {
        if len(dedup) == 0 || !dedup[len(dedup)-1].Equal(t) {
            dedup = append(dedup, t)
        }
    }
    return dedup, nil
}

func generatePlanEvents(tx *gorm.DB, plan Plan, fromDate, toDate string) (int, error) {
    times, err := expandPlan(plan, fromDate, toDate)
    if err != nil {
        return 0, err
    }
    return insertPlanEvents(tx, plan, times)
}

func insertPlanEvents(tx *gorm.DB, plan Plan, times []time.Time) (int, error) {
    var medicine Medicine
    if err := tx.Where("id = ? AND book_id = ?", plan.MedicineID, plan.BookID).First(&medicine).Error; err != nil {
        return 0, err
    }
    loc := loadLocation(plan.Timezone)
    count := 0
    for _, at := range times {
        local := at.In(loc)
        ev := DoseEvent{
            BookID: plan.BookID, PlanID: plan.ID, MedicineID: plan.MedicineID,
            MedicineName: medicine.Name, PlanTitle: plan.Title, Type: plan.Type,
            DoseAmount: plan.DoseAmount, DoseUnit: plan.DoseUnit,
            LocationID: plan.LocationID, LocationName: plan.LocationName, Address: plan.Address,
            ExecutorType: plan.ExecutorType, ExecutorName: plan.ExecutorName,
            DripRate: plan.DripRate, DurationMinutes: plan.DurationMinutes,
            ScheduledAt: at, LocalDate: local.Format("2006-01-02"), LocalTime: local.Format("15:04"),
            Status: EventPending, Version: 1, CreatedAt: nowUTC(), UpdatedAt: nowUTC(),
        }
        res := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&ev)
        if res.Error != nil {
            return count, res.Error
        }
        if res.RowsAffected > 0 {
            count++
            if plan.NotificationsEnabled {
                ensureEventJobs(tx, ev)
            }
        }
    }
    return count, nil
}

func generateForPlanRange(tx *gorm.DB, plan Plan) (int, error) {
    return generateForPlanRangeFrom(tx, plan, time.Time{})
}

// generateForPlanRangeFrom 生成 [今天, 今天+60] 的事件；skipBefore 非零时跳过更早的
// 时间点，避免新建/恢复计划时立刻产生一堆“逾期”噪音。
func generateForPlanRangeFrom(tx *gorm.DB, plan Plan, skipBefore time.Time) (int, error) {
    book, _ := getBook(tx, plan.BookID)
    today := todayIn(book.Timezone)
    from := today
    if plan.StartDate > today {
        from = plan.StartDate
    } else if plan.StartDate != "" {
        // 计划开始日期在过去（比如今天补录 20 号的计划）：从开始日期补生成，
        // 否则过去那天在「今日 / 计划」里一条事件都没有。最多往前补 30 天，避免一次生成过多。
        earliest := addDays(today, -30)
        from = plan.StartDate
        if from < earliest {
            from = earliest
        }
    }
    to := addDays(from, 60)
    if plan.EndDate != "" && plan.EndDate < to {
        to = plan.EndDate
    }
    if skipBefore.IsZero() {
        return generatePlanEvents(tx, plan, from, to)
    }
    times, err := expandPlan(plan, from, to)
    if err != nil {
        return 0, err
    }
    filtered := make([]time.Time, 0, len(times))
    for _, at := range times {
        if at.Before(skipBefore) {
            continue
        }
        filtered = append(filtered, at)
    }
    return insertPlanEvents(tx, plan, filtered)
}

func handlePreviewPlan(db *gorm.DB) gin.HandlerFunc {
    return func(c *gin.Context) {
        id, ok := parseUintParam(c, "id")
        if !ok {
            return
        }
        var plan Plan
        if err := db.Where("id = ? AND book_id = ?", id, contextBookID(c)).First(&plan).Error; err != nil {
            response.ErrorNotFound(c, "计划不存在")
            return
        }
        book, _ := getBook(db, plan.BookID)
        days := utils.Atoi(c.Query("days"), 7)
        if days < 1 || days > 30 {
            days = 7
        }
        from := todayIn(book.Timezone)
        to := addDays(from, days-1)
        times, err := expandPlan(plan, from, to)
        if err != nil {
            response.ErrorBadRequest(c, "计划频率无效")
            return
        }
        response.Success(c, gin.H{"from": from, "to": to, "times": times})
    }
}

// autoCompletePlan 疗程（有结束日期）内所有事件都已处理完（完成/跳过/漏服）时，
// 把计划置为「已完成」；长期计划不自动完成，需手动结束。
func autoCompletePlan(db *gorm.DB, planID uint) {
    var plan Plan
    if err := db.Where("id = ?", planID).First(&plan).Error; err != nil {
        return
    }
    if plan.Status != PlanActive || plan.EndDate == "" {
        return
    }
    var total, pending int64
    db.Model(&DoseEvent{}).Where("plan_id = ?", plan.ID).Count(&total)
    db.Model(&DoseEvent{}).Where("plan_id = ? AND status = ?", plan.ID, EventPending).Count(&pending)
    if total == 0 || pending > 0 {
        return
    }
    db.Model(&Plan{}).Where("id = ? AND book_id = ?", plan.ID, plan.BookID).Updates(map[string]interface{}{"status": PlanCompleted, "updated_at": nowUTC()})
}

// repairMissingPlanEvents 修复「计划建了但一条事件都没有」的历史数据：
// 早期版本对开始日期在过去的新计划会跳过所有过去时间点，导致那些日子在今日/计划页看不到安排。
// 这里对「进行中且开始日期已到、却零事件」的计划补生成一次（插入用 OnConflict DoNothing，幂等）。
// repairStalePlanEvents 清掉「计划已结束/已归档，但今天及以后还挂着待用药」的历史遗留事件。
// 这些条目在今日页会显示成待用药甚至超时，实际计划早就停了。
func repairStalePlanEvents(db *gorm.DB, bookID uint) {
    book, err := getBook(db, bookID)
    if err != nil {
        return
    }
    _ = book
    // 计划已不在进行中：它名下所有未完成事件都不该再挂着（历史已打卡的保留）。
    var stale []DoseEvent
    db.Model(&DoseEvent{}).
        Joins("JOIN plans ON plans.id = dose_events.plan_id").
        Where("dose_events.book_id = ? AND dose_events.status = ? AND plans.status <> ?",
            bookID, EventPending, PlanActive).
        Find(&stale)
    for _, ev := range stale {
        cancelEventJobs(db, ev.ID)
        db.Delete(&DoseEvent{}, ev.ID)
    }
}

func repairMissingPlanEvents(db *gorm.DB) {
    var plans []Plan
    if err := db.Where("status = ?", PlanActive).Find(&plans).Error; err != nil {
        return
    }
    for _, plan := range plans {
        book, err := getBook(db, plan.BookID)
        if err != nil {
            continue
        }
        today := todayIn(book.Timezone)
        if plan.StartDate == "" || plan.StartDate > today {
            continue
        }
        var cnt int64
        db.Model(&DoseEvent{}).Where("plan_id = ?", plan.ID).Count(&cnt)
        if cnt > 0 {
            continue
        }
        if _, err := generateForPlanRangeFrom(db, plan, time.Time{}); err != nil {
            logger.Error("medcare: 补生成计划事件失败 plan=%d: %v", plan.ID, err)
        }
    }
}

// autoCompleteBookPlans 兜底扫描一个用药本下所有可自动完成的计划。
func autoCompleteBookPlans(db *gorm.DB, bookID uint) {
    var plans []Plan
    db.Where("book_id = ? AND status = ? AND end_date <> ''", bookID, PlanActive).Find(&plans)
    for _, p := range plans {
        autoCompletePlan(db, p.ID)
    }
}

func handleListPlans(db *gorm.DB) gin.HandlerFunc {
    return func(c *gin.Context) {
        repairMissingPlanEvents(db)
        autoCompleteBookPlans(db, contextBookID(c))
        page := utils.Atoi(c.Query("page"), 1)
        pageSize := utils.Atoi(c.Query("pageSize"), 20)
        if pageSize > 100 {
            pageSize = 100
        }
        q := db.Model(&Plan{}).Where("book_id = ?", contextBookID(c))
        if s := c.Query("status"); s != "" {
            q = q.Where("status = ?", s)
        } else if c.Query("include_archived") != "true" {
            // 删除计划是软删除（status=archived）：默认不返回，否则药品详情里
            // 会一直挂着「已归档」的计划，看起来像删除没生效。
            q = q.Where("status <> ?", PlanArchived)
        }
        if m := c.Query("medicine_id"); m != "" {
            q = q.Where("medicine_id = ?", m)
        }
        if t := c.Query("type"); t != "" {
            q = q.Where("type = ?", t)
        }
        var total int64
        q.Count(&total)
        var rows []Plan
        if err := q.Order("status ASC, updated_at DESC").Offset(utils.Offset(page, pageSize)).Limit(pageSize).Find(&rows).Error; err != nil {
            response.ErrorInternal(c, "读取计划失败")
            return
        }
        items := make([]PlanDTO, 0, len(rows))
        for _, p := range rows {
            items = append(items, planToDTO(db, p))
        }
        response.SuccessPage(c, items, total, page, pageSize)
    }
}

func handleCreatePlan(db *gorm.DB) gin.HandlerFunc {
    return func(c *gin.Context) {
        var in planInput
        if err := c.ShouldBindJSON(&in); err != nil {
            response.ErrorBadRequest(c, "参数无效")
            return
        }
        book, _ := getBook(db, contextBookID(c))
        if err := in.normalize(db, book); err != nil {
            response.ErrorBadRequest(c, err.Error())
            return
        }
        userID := currentUserID(c)
        now := nowUTC()
        plan := Plan{BookID: book.ID, Status: PlanActive, NotificationsEnabled: true, Version: 1, CreatedBy: userID, CreatedAt: now}
        applyPlanInput(db, &plan, in, book, userID)
        var created int
        err := db.Transaction(func(tx *gorm.DB) error {
            if err := tx.Create(&plan).Error; err != nil {
                return err
            }
            // 新建计划不跳过过去时间点：补录「昨天/前几天」的计划也要能看到当天安排。
            n, err := generateForPlanRangeFrom(tx, plan, time.Time{})
            created = n
            return err
        })
        if err != nil {
            response.ErrorInternal(c, "创建计划失败")
            return
        }
        audit.Log(db, c, "medcare_plan_create", "plan", plan.ID, plan.Title)
        dto := planToDTO(db, plan)
        // 新建计划会跳过「2 小时前」的时间点；把今天因此被跳过的时刻告诉前端，
        // 避免用户以为当天就会提醒。
        skippedToday := []string{}
        bookToday := todayIn(book.Timezone)
        if plan.StartDate <= bookToday && (plan.EndDate == "" || plan.EndDate >= bookToday) {
            if times, err := expandPlan(plan, bookToday, bookToday); err == nil {
                cutoff := nowUTC().Add(-2 * time.Hour)
                loc := loadLocation(plan.Timezone)
                for _, at := range times {
                    if at.Before(cutoff) {
                        skippedToday = append(skippedToday, at.In(loc).Format("15:04"))
                    }
                }
            }
        }
        response.Success(c, gin.H{"plan": dto, "created_events": created, "skipped_today_times": skippedToday})
    }
}

func handleGetPlan(db *gorm.DB) gin.HandlerFunc {
    return func(c *gin.Context) {
        id, ok := parseUintParam(c, "id")
        if !ok {
            return
        }
        var plan Plan
        if err := db.Where("id = ? AND book_id = ?", id, contextBookID(c)).First(&plan).Error; err != nil {
            response.ErrorNotFound(c, "计划不存在")
            return
        }
        var adjustments []PlanAdjustment
        db.Where("plan_id = ? AND book_id = ?", plan.ID, plan.BookID).Order("created_at DESC").Limit(50).Find(&adjustments)
        response.Success(c, gin.H{"plan": planToDTO(db, plan), "adjustments": adjustments})
    }
}

// startOfToday 返回指定时区「今天 00:00」对应的 UTC 时间，用于「从今天起重新生成」。
func startOfToday(tz string) time.Time {
    loc := loadLocation(tz)
    now := nowUTC().In(loc)
    return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc).UTC()
}

// deleteStalePendingEvents 在计划改动后清理「已过时」的待用药事件。
//
// 规则（用户 2026-11-23 明确：历史已完成的保留，未完成的按最新计划更新）：
//   - 已打卡（完成/漏服/跳过）的历史事件：一律保留，不动；
//   - 未完成（待用药）事件：全部清掉，随后由生成逻辑按新计划重新铺一遍
//     （过去的也会补，最多往前 30 天）。
//
// 这样改了用药时间、频率、剂量或起止日期后，今日页与计划页都只反映最新计划，
// 不会留下旧时间点、旧剂量、旧药品的残留条目。
func deleteStalePendingEvents(tx *gorm.DB, plan Plan) error {
    var events []DoseEvent
    if err := tx.Where("plan_id = ? AND status = ?", plan.ID, EventPending).Find(&events).Error; err != nil {
        return err
    }
    for _, ev := range events {
        cancelEventJobs(tx, ev.ID)
        if err := tx.Delete(&DoseEvent{}, ev.ID).Error; err != nil {
            return err
        }
    }
    return nil
}

func deleteFuturePendingEvents(tx *gorm.DB, planID uint, after time.Time) error {
    var events []DoseEvent
    tx.Where("plan_id = ? AND status = ? AND scheduled_at > ?", planID, EventPending, after).Find(&events)
    for _, ev := range events {
        cancelEventJobs(tx, ev.ID)
    }
    return tx.Where("plan_id = ? AND status = ? AND scheduled_at > ?", planID, EventPending, after).Delete(&DoseEvent{}).Error
}

func handleUpdatePlan(db *gorm.DB) gin.HandlerFunc {
    return func(c *gin.Context) {
        id, ok := parseUintParam(c, "id")
        if !ok {
            return
        }
        var plan Plan
        if err := db.Where("id = ? AND book_id = ?", id, contextBookID(c)).First(&plan).Error; err != nil {
            response.ErrorNotFound(c, "计划不存在")
            return
        }
        var in planInput
        if err := c.ShouldBindJSON(&in); err != nil {
            response.ErrorBadRequest(c, "参数无效")
            return
        }
        if in.Version > 0 && in.Version != plan.Version {
            response.Error(c, http.StatusConflict, 2007, ErrVersionConflict.Error())
            return
        }
        book, _ := getBook(db, plan.BookID)
        if err := in.normalize(db, book); err != nil {
            response.ErrorBadRequest(c, err.Error())
            return
        }
        applyPlanInput(db, &plan, in, book, currentUserID(c))
        plan.Version++
        var created int
        err := db.Transaction(func(tx *gorm.DB) error {
            if err := tx.Save(&plan).Error; err != nil {
                return err
            }
            // 先清掉所有未完成事件（历史已打卡的保留），再按新计划从「今天」往后重铺。
            // 不从开始日期回补：改计划不该在过去的日期上凭空造出一批没吃过的待用药，
            // 那会让历史页面显示一堆本不存在的「超时」。
            if err := deleteStalePendingEvents(tx, plan); err != nil {
                return err
            }
            n, err := generateForPlanRangeFrom(tx, plan, startOfToday(plan.Timezone))
            created = n
            return err
        })
        if err != nil {
            response.ErrorInternal(c, "保存计划失败")
            return
        }
        audit.Log(db, c, "medcare_plan_update", "plan", plan.ID, plan.Title)
        response.Success(c, gin.H{"plan": planToDTO(db, plan), "created_events": created})
    }
}

func handlePlanStatus(db *gorm.DB, status string) gin.HandlerFunc {
    return func(c *gin.Context) {
        id, ok := parseUintParam(c, "id")
        if !ok {
            return
        }
        var plan Plan
        if err := db.Where("id = ? AND book_id = ?", id, contextBookID(c)).First(&plan).Error; err != nil {
            response.ErrorNotFound(c, "计划不存在")
            return
        }
        err := db.Transaction(func(tx *gorm.DB) error {
            plan.Status = status
            plan.UpdatedAt = nowUTC()
            plan.Version++
            if err := tx.Save(&plan).Error; err != nil {
                return err
            }
            if err := deleteStalePendingEvents(tx, plan); err != nil {
                return err
            }
            if status != PlanActive {
                return nil
            }
            _, err := generateForPlanRangeFrom(tx, plan, startOfToday(plan.Timezone))
            return err
        })
        if err != nil {
            response.ErrorInternal(c, "更新计划状态失败")
            return
        }
        audit.Log(db, c, "medcare_plan_status", "plan", plan.ID, status)
        response.Success(c, planToDTO(db, plan))
    }
}

func handleDeletePlan(db *gorm.DB) gin.HandlerFunc {
    return func(c *gin.Context) {
        id, ok := parseUintParam(c, "id")
        if !ok {
            return
        }
        var plan Plan
        if err := db.Where("id = ? AND book_id = ?", id, contextBookID(c)).First(&plan).Error; err != nil {
            response.ErrorNotFound(c, "计划不存在")
            return
        }
        err := db.Transaction(func(tx *gorm.DB) error {
            plan.Status = PlanArchived
            plan.UpdatedAt = nowUTC()
            plan.Version++
            if err := tx.Save(&plan).Error; err != nil {
                return err
            }
            // 删除后这个计划不该再出现在今日页：连今天已过点的待用药一起清掉。
            return deleteStalePendingEvents(tx, plan)
        })
        if err != nil {
            response.ErrorInternal(c, "删除计划失败")
            return
        }
        audit.Log(db, c, "medcare_plan_delete", "plan", plan.ID, plan.Title)
        response.Success(c, gin.H{"deleted": true})
    }
}

// archivedMedicineIDs 返回这个用药本里已归档（删除）的药品 ID 集合。
func archivedMedicineIDs(db *gorm.DB, bookID uint) map[uint]bool {
    var ids []uint
    db.Model(&Medicine{}).Where("book_id = ? AND archived_at IS NOT NULL", bookID).Pluck("id", &ids)
    set := make(map[uint]bool, len(ids))
    for _, id := range ids {
        set[id] = true
    }
    return set
}

// markEventsMedicineMissing 给事件列表补上「药品已删除」标记。
func markEventsMedicineMissing(db *gorm.DB, bookID uint, dtos []DoseEventDTO) {
    if len(dtos) == 0 {
        return
    }
    archived := archivedMedicineIDs(db, bookID)
    for i := range dtos {
        dtos[i].MedicineMissing = archived[dtos[i].MedicineID]
    }
}

// repairOrphanEventMedicines 修复历史遗留：药品被删除后，同计划的待用药事件还挂在旧药品上，
// 而计划本身已经改到现有药品。这里把这类「待用药 + 药品已归档」的事件，
// 按计划当前的药品重新对齐（只动待用药事件，历史完成/漏服/跳过的不动）。
func repairOrphanEventMedicines(db *gorm.DB, bookID uint) {
    archived := archivedMedicineIDs(db, bookID)
    if len(archived) == 0 {
        return
    }
    var events []DoseEvent
    db.Where("book_id = ? AND status = ?", bookID, EventPending).Find(&events)
    if len(events) == 0 {
        return
    }
    planMed := map[uint]Medicine{}
    planIDs := make([]uint, 0, len(events))
    seen := map[uint]bool{}
    for _, ev := range events {
        if archived[ev.MedicineID] && !seen[ev.PlanID] {
            seen[ev.PlanID] = true
            planIDs = append(planIDs, ev.PlanID)
        }
    }
    if len(planIDs) == 0 {
        return
    }
    var plans []Plan
    db.Where("book_id = ? AND id IN ?", bookID, planIDs).Find(&plans)
    validMedIDs := make([]uint, 0, len(plans))
    for _, p := range plans {
        if p.MedicineID != 0 && !archived[p.MedicineID] {
            validMedIDs = append(validMedIDs, p.MedicineID)
        }
    }
    if len(validMedIDs) == 0 {
        return
    }
    var meds []Medicine
    db.Where("book_id = ? AND id IN ?", bookID, validMedIDs).Find(&meds)
    for _, m := range meds {
        planMed[m.ID] = m
    }
    planByID := map[uint]Plan{}
    for _, p := range plans {
        planByID[p.ID] = p
    }
    for _, ev := range events {
        if !archived[ev.MedicineID] {
            continue
        }
        p, ok := planByID[ev.PlanID]
        if !ok {
            continue
        }
        m, ok := planMed[p.MedicineID]
        if !ok {
            continue
        }
        db.Model(&DoseEvent{}).Where("id = ?", ev.ID).Updates(map[string]interface{}{
            "medicine_id":   m.ID,
            "medicine_name": m.Name,
            "type":          m.Category,
            "dose_unit":     firstNonEmpty(ev.DoseUnit, m.Unit),
        })
    }
}

func handleListEvents(db *gorm.DB) gin.HandlerFunc {
    return func(c *gin.Context) {
        page := utils.Atoi(c.Query("page"), 1)
        pageSize := utils.Atoi(c.Query("pageSize"), 50)
        if pageSize > 200 {
            pageSize = 200
        }
        // 「计划页 → 按时间」直接查事件：这里也补一次零事件的计划，
        // 否则没先打开计划列表的入口会看不到刚补录的历史安排。
        repairMissingPlanEvents(db)
        // 药品删掉后遗留在旧药品上的待用药事件，先按计划现有药品对齐，
        // 否则计划页还会列出「已删除药品」的条目、打卡报库存不足。
        repairOrphanEventMedicines(db, contextBookID(c))
        repairStalePlanEvents(db, contextBookID(c))
        q := db.Model(&DoseEvent{}).Where("book_id = ?", contextBookID(c))
        if from := c.Query("from"); from != "" {
            q = q.Where("local_date >= ?", from)
        }
        if to := c.Query("to"); to != "" {
            q = q.Where("local_date <= ?", to)
        }
        if date := c.Query("date"); date != "" {
            q = q.Where("local_date = ?", date)
        }
        if s := c.Query("status"); s != "" {
            q = q.Where("status = ?", s)
        }
        if m := c.Query("medicine_id"); m != "" {
            q = q.Where("medicine_id = ?", m)
        }
        if t := c.Query("type"); t != "" {
            q = q.Where("type = ?", t)
        }
        var total int64
        q.Count(&total)
        var rows []DoseEvent
        if err := q.Order("scheduled_at ASC").Offset(utils.Offset(page, pageSize)).Limit(pageSize).Find(&rows).Error; err != nil {
            response.ErrorInternal(c, "读取日程失败")
            return
        }
        items := make([]DoseEventDTO, 0, len(rows))
        for _, ev := range rows {
            items = append(items, eventToDTO(ev))
        }
        markEventsMedicineMissing(db, contextBookID(c), items)
        response.SuccessPage(c, items, total, page, pageSize)
    }
}

func handleGetEvent(db *gorm.DB) gin.HandlerFunc {
    return func(c *gin.Context) {
        id, ok := parseUintParam(c, "id")
        if !ok {
            return
        }
        var ev DoseEvent
        if err := db.Where("id = ? AND book_id = ?", id, contextBookID(c)).First(&ev).Error; err != nil {
            response.ErrorNotFound(c, "事件不存在")
            return
        }
        response.Success(c, eventToDTO(ev))
    }
}

func handleDashboard(db *gorm.DB) gin.HandlerFunc {
    return func(c *gin.Context) {
        book, err := getBook(db, contextBookID(c))
        if err != nil {
            response.ErrorNotFound(c, "用药本不存在")
            return
        }
        date := c.Query("date")
        if date == "" {
            date = todayIn(book.Timezone)
        }
        // 历史遗留的「计划有、事件没有」先补齐，否则过去日期永远看不到安排。
        repairMissingPlanEvents(db)
        repairOrphanEventMedicines(db, book.ID)
        // 计划已结束/归档却还留着今天及以后的待用药：先清掉，今日页只显示在用的计划。
        repairStalePlanEvents(db, book.ID)
        var events []DoseEvent
        db.Where("book_id = ? AND local_date = ?", book.ID, date).Order("scheduled_at ASC").Find(&events)
        groups := []DashboardGroup{
            {Key: "morning", Label: "早上", Events: []DoseEventDTO{}},
            {Key: "noon", Label: "中午", Events: []DoseEventDTO{}},
            {Key: "afternoon", Label: "下午", Events: []DoseEventDTO{}},
            {Key: "evening", Label: "晚上", Events: []DoseEventDTO{}},
            {Key: "night", Label: "夜间", Events: []DoseEventDTO{}},
        }
        summary := map[string]int64{"total": 0, "done": 0, "pending": 0, "missed": 0, "skipped": 0}
        var next *DoseEventDTO
        now := nowUTC()
        for _, ev := range events {
            dto := eventToDTO(ev)
            summary["total"]++
            summary[ev.Status]++
            if ev.Status == EventPending && next == nil && ev.ScheduledAt.After(now) {
                copy := dto
                next = &copy
            }
            key := timeBucket(ev.LocalTime)
            for i := range groups {
                if groups[i].Key == key {
                    groups[i].Events = append(groups[i].Events, dto)
                    break
                }
            }
        }
        // 超时列表严格绑定当前查看的日期：默认是哪天就只显示那天的安排，
        // 过点未完成算「超时」仍归在它原本的计划日期下（用户 2026-11-23 明确要求）。
        // 之前用 local_date <= date 会把前几天的待完成事件混进今天，导致数量与进度对不上。
        var overdueRows []DoseEvent
        db.Where("book_id = ? AND status = ? AND local_date = ? AND scheduled_at < ?", book.ID, EventPending, date, now).
            Order("scheduled_at ASC").Limit(200).Find(&overdueRows)
        overdue := make([]DoseEventDTO, 0, len(overdueRows))
        for _, ev := range overdueRows {
            overdue = append(overdue, eventToDTO(ev))
        }
        markEventsMedicineMissing(db, book.ID, overdue)
        // 所有分组与「下一项」也补上药品缺失标记。
        for i := range groups {
            markEventsMedicineMissing(db, book.ID, groups[i].Events)
        }
        if next != nil {
            tmp := []DoseEventDTO{*next}
            markEventsMedicineMissing(db, book.ID, tmp)
            next = &tmp[0]
        }
        var member BookMember
        _ = db.Where("book_id = ? AND user_id = ?", book.ID, currentUserID(c)).First(&member).Error
        var activePlans int64
        db.Model(&Plan{}).Where("book_id = ? AND status = ?", book.ID, PlanActive).Count(&activePlans)
        var upcoming *DoseEventDTO
        var nextRow DoseEvent
        if err := db.Where("book_id = ? AND status = ? AND scheduled_at > ?", book.ID, EventPending, now).Order("scheduled_at ASC").First(&nextRow).Error; err == nil {
            dto := eventToDTO(nextRow)
            tmp := []DoseEventDTO{dto}
            markEventsMedicineMissing(db, book.ID, tmp)
            upcoming = &tmp[0]
        }
        response.Success(c, DashboardDTO{
            Date: date, Book: bookToDTO(db, book, member.Role, currentUserID(c)), Summary: summary, Overdue: overdue,
            Groups: groups, NextEvent: next, ActivePlanCount: activePlans, NextUpcoming: upcoming, ServerTime: now,
        })
    }
}

func timeBucket(clock string) string {
    parts := strings.Split(clock, ":")
    h := 0
    if len(parts) > 0 {
        h = utils.Atoi(parts[0], 0)
    }
    switch {
    case h >= 5 && h < 11:
        return "morning"
    case h >= 11 && h < 14:
        return "noon"
    case h >= 14 && h < 18:
        return "afternoon"
    case h >= 18 && h < 22:
        return "evening"
    default:
        return "night"
    }
}

func handleCalendar(db *gorm.DB) gin.HandlerFunc {
    return func(c *gin.Context) {
        from, to := c.Query("from"), c.Query("to")
        var rows []struct {
            LocalDate string `json:"local_date"`
            Status    string `json:"status"`
            Count     int64  `json:"count"`
        }
        db.Model(&DoseEvent{}).Select("local_date, status, count(*) as count").Where("book_id = ? AND local_date BETWEEN ? AND ?", contextBookID(c), from, to).Group("local_date, status").Scan(&rows)
        response.Success(c, rows)
    }
}

func handleEventNote(db *gorm.DB) gin.HandlerFunc {
    return func(c *gin.Context) {
        if !requireCanRecord(c) {
            response.ErrorForbidden(c, "管理员没有把「用药记录 · 点完成」的权限给你，需要的话请联系用药本管理员")
            return
        }
        id, ok := parseUintParam(c, "id")
        if !ok {
            return
        }
        var ev DoseEvent
        if err := db.Where("id = ? AND book_id = ?", id, contextBookID(c)).First(&ev).Error; err != nil {
            response.ErrorNotFound(c, "事件不存在")
            return
        }
        var in struct {
            Note string `json:"note"`
        }
        if err := c.ShouldBindJSON(&in); err != nil {
            response.ErrorBadRequest(c, "参数无效")
            return
        }
        ev.Note = in.Note
        ev.UpdatedAt = nowUTC()
        if err := db.Save(&ev).Error; err != nil {
            response.ErrorInternal(c, "保存备注失败")
            return
        }
        response.Success(c, eventToDTO(ev))
    }
}

func handleSnoozeEvent(db *gorm.DB) gin.HandlerFunc {
    return func(c *gin.Context) {
        if !requireCanRecord(c) {
            response.ErrorForbidden(c, "管理员没有把「用药记录 · 点完成」的权限给你，需要的话请联系用药本管理员")
            return
        }
        id, ok := parseUintParam(c, "id")
        if !ok {
            return
        }
        var ev DoseEvent
        if err := db.Where("id = ? AND book_id = ?", id, contextBookID(c)).First(&ev).Error; err != nil {
            response.ErrorNotFound(c, "事件不存在")
            return
        }
        if ev.Status != EventPending {
            response.Error(c, http.StatusConflict, 2004, "只有待完成的事件可以稍后提醒")
            return
        }
        var in struct {
            Minutes int `json:"minutes"`
        }
        if err := c.ShouldBindJSON(&in); err != nil || in.Minutes < 1 || in.Minutes > 1440 {
            response.ErrorBadRequest(c, "稍后提醒时间应为 1–1440 分钟")
            return
        }
        t := nowUTC().Add(time.Duration(in.Minutes) * time.Minute)
        ev.SnoozeUntil = &t
        ev.UpdatedAt = nowUTC()
        if err := db.Save(&ev).Error; err != nil {
            response.ErrorInternal(c, "保存稍后提醒失败")
            return
        }
        cancelEventJobs(db, ev.ID)
        ensureEventJobs(db, ev)
        response.Success(c, eventToDTO(ev))
    }
}

// handleRemindEvent 成员手动「提醒用药」：给用药人与所有监护人发一条即时提醒。
// 成员本身不能改用药数据，但可以推动照护者。
func handleRemindEvent(db *gorm.DB) gin.HandlerFunc {
    return func(c *gin.Context) {
        id, ok := parseUintParam(c, "id")
        if !ok {
            return
        }
        var ev DoseEvent
        if err := db.Where("id = ? AND book_id = ?", id, contextBookID(c)).First(&ev).Error; err != nil {
            response.ErrorNotFound(c, "事件不存在")
            return
        }
        if ev.Status == EventDone || ev.Status == EventSkipped {
            response.Error(c, http.StatusConflict, 2004, "这一项已经完成，无需提醒")
            return
        }
        book, err := getBook(db, ev.BookID)
        if err != nil {
            response.ErrorNotFound(c, "用药本不存在")
            return
        }
        var members []BookMember
        db.Where("book_id = ? AND notifications_enabled = ?", ev.BookID, true).Find(&members)
        if len(members) == 0 {
            response.ErrorBadRequest(c, "本用药本还没有开启提醒的成员")
            return
        }
        senderID := currentUserID(c)
        senderName := getUserName(db, senderID)
        payload := encodeJSON(gin.H{
            "event_id": ev.ID, "medicine_name": ev.MedicineName, "dose_amount": ev.DoseAmount,
            "dose_unit": ev.DoseUnit, "type": ev.Type, "local_time": ev.LocalTime,
            "reminder_name": senderName,
        })
        sent := 0
        err = db.Transaction(func(tx *gorm.DB) error {
            for _, member := range members {
                // 发起人自己不用再收一遍
                if member.UserID == senderID {
                    continue
                }
                // 只提醒用药人与监护人；普通成员不承担照护职责。
                // 提醒发给用药人和「能帮忙记录」的家人（监护人默认有，管理员单独勾选过的人也算）。
                if !member.IsCareRecipient && !memberHasPermission(member, PermTodayRecord) {
                    continue
                }
                createNotificationJob(tx, ev.BookID, member.UserID, 0, ChannelInApp, &ev.ID, JobRemind, nowUTC(), payload)
                var channels []NotificationChannel
                tx.Where("user_id = ? AND enabled = ? AND archived_at IS NULL AND status = ?", member.UserID, true, "active").Find(&channels)
                for _, ch := range channels {
                    createNotificationJob(tx, ev.BookID, member.UserID, ch.ID, ch.Type, &ev.ID, JobRemind, nowUTC(), payload)
                }
                sent++
            }
            return nil
        })
        if err != nil {
            response.ErrorInternal(c, "发送提醒失败")
            return
        }
        if sent == 0 {
            response.ErrorBadRequest(c, "本用药本暂无其他用药人或监护人可提醒")
            return
        }
        audit.Log(db, c, "medcare_event_remind", "dose_event", ev.ID, book.Name)
        response.Success(c, gin.H{"ok": true, "reminded": sent})
    }
}

func handleEventLocation(db *gorm.DB) gin.HandlerFunc {
    return func(c *gin.Context) {
        if !requireCanRecord(c) {
            response.ErrorForbidden(c, "管理员没有把「用药记录 · 点完成」的权限给你，需要的话请联系用药本管理员")
            return
        }
        id, ok := parseUintParam(c, "id")
        if !ok {
            return
        }
        var ev DoseEvent
        if err := db.Where("id = ? AND book_id = ?", id, contextBookID(c)).First(&ev).Error; err != nil {
            response.ErrorNotFound(c, "事件不存在")
            return
        }
        var in struct {
            LocationID   *uint  `json:"location_id"`
            LocationName string `json:"location_name"`
            Address      string `json:"address"`
            Note         string `json:"note"`
        }
        if err := c.ShouldBindJSON(&in); err != nil {
            response.ErrorBadRequest(c, "参数无效")
            return
        }
        ev.LocationID = in.LocationID
        ev.LocationName = in.LocationName
        ev.Address = in.Address
        if in.Note != "" {
            ev.Note = in.Note
        }
        ev.UpdatedAt = nowUTC()
        if err := db.Save(&ev).Error; err != nil {
            response.ErrorInternal(c, "保存地点失败")
            return
        }
        response.Success(c, eventToDTO(ev))
    }
}

func handleMarkMissed(db *gorm.DB) gin.HandlerFunc {
    return func(c *gin.Context) {
        if !requireCanRecord(c) {
            response.ErrorForbidden(c, "管理员没有把「用药记录 · 点完成」的权限给你，需要的话请联系用药本管理员")
            return
        }
        id, ok := parseUintParam(c, "id")
        if !ok {
            return
        }
        var ev DoseEvent
        if err := db.Where("id = ? AND book_id = ?", id, contextBookID(c)).First(&ev).Error; err != nil {
            response.ErrorNotFound(c, "事件不存在")
            return
        }
        if ev.Status == EventDone || ev.Status == EventSkipped {
            response.Error(c, http.StatusConflict, 2004, "当前状态不能标记漏服")
            return
        }
        var in struct {
            Note string `json:"note"`
        }
        _ = c.ShouldBindJSON(&in)
        now := nowUTC()
        ev.Status = EventMissed
        ev.MissedAt = &now
        ev.MarkedBy = uintPtr(currentUserID(c))
        if in.Note != "" {
            ev.Note = in.Note
        }
        ev.Version++
        ev.UpdatedAt = now
        if err := db.Save(&ev).Error; err != nil {
            response.ErrorInternal(c, "标记漏服失败")
            return
        }
        cancelEventJobs(db, ev.ID)
        publishEventChanged(ev.BookID, "event", "missed", ev.ID, "")
        response.Success(c, eventToDTO(ev))
    }
}

// handleStartEvent 记录输液实际开始时间；重复点击幂等。
func handleStartEvent(db *gorm.DB) gin.HandlerFunc {
    return func(c *gin.Context) {
        if !requireCanRecord(c) {
            response.ErrorForbidden(c, "管理员没有把「用药记录 · 点完成」的权限给你，记录输液开始时间请找管理员在「成员与权限」里开通")
            return
        }
        id, ok := parseUintParam(c, "id")
        if !ok {
            return
        }
        var ev DoseEvent
        err := db.Transaction(func(tx *gorm.DB) error {
            if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND book_id = ?", id, contextBookID(c)).First(&ev).Error; err != nil {
                return err
            }
            if ev.Type != "infusion" {
                return errors.New("只有输液需要记录开始时间")
            }
            if ev.Status == EventDone || ev.Status == EventSkipped {
                return ErrInvalidTransition
            }
            if ev.ActualStartAt == nil {
                now := nowUTC()
                ev.ActualStartAt = &now
                ev.Version++
                ev.UpdatedAt = now
                if err := tx.Save(&ev).Error; err != nil {
                    return err
                }
            }
            return nil
        })
        if err != nil {
            writeError(c, err)
            return
        }
        publishEventChanged(ev.BookID, "event", "started", ev.ID, "")
        audit.Log(db, c, "medcare_event_start", "dose_event", ev.ID, ev.MedicineName)
        response.Success(c, eventToDTO(ev))
    }
}

func handleCompleteEvent(db *gorm.DB) gin.HandlerFunc {
    return func(c *gin.Context) {
        if !requireCanRecord(c) {
            response.ErrorForbidden(c, "管理员没有把「用药记录 · 点完成」的权限给你，打卡请找管理员在「成员与权限」里开通")
            return
        }
        id, ok := parseUintParam(c, "id")
        if !ok {
            return
        }
        var in struct {
            Version       uint   `json:"version"`
            DeductStock   *bool  `json:"deduct_stock"`
            AllowShortage bool   `json:"allow_shortage"`
            Note          string `json:"note"`
        }
        _ = c.ShouldBindJSON(&in)
        var ev DoseEvent
        var allocations []StockAllocation
        var shortage float64
        var resultMedicine Medicine
        err := db.Transaction(func(tx *gorm.DB) error {
            if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND book_id = ?", id, contextBookID(c)).First(&ev).Error; err != nil {
                return err
            }
            if ev.Status == EventDone {
                return nil
            }
            if ev.Status == EventSkipped {
                return ErrInvalidTransition
            }
            if in.Version > 0 && in.Version != ev.Version {
                return ErrVersionConflict
            }
            wasMissed := ev.Status == EventMissed || ev.WasMissed
            deduct := true
            if in.DeductStock != nil {
                deduct = *in.DeductStock
            }
            if deduct {
                book, _ := getBook(tx, ev.BookID)
                var err error
                allocations, shortage, err = deductFEFO(tx, ev.BookID, ev.MedicineID, ev.DoseAmount, ev.ID, todayIn(book.Timezone), currentUserID(c))
                if err != nil {
                    return err
                }
                if shortage > 0 && !in.AllowShortage {
                    return ErrStockShortage
                }
            }
            now := nowUTC()
            ev.Status = EventDone
            ev.CompletedAt = &now
            ev.CompletedBy = uintPtr(currentUserID(c))
            if ev.Type == "infusion" {
                ev.ActualEndAt = &now
            }
            // 打卡不带备注时保留已有备注（可能先经 /note 接口写过），
            // 不要无条件覆盖成空串。
            if in.Note != "" {
                ev.Note = in.Note
            }
            ev.WasMissed = wasMissed
            ev.StockShortage = shortage > 0
            ev.Version++
            ev.UpdatedAt = now
            if err := tx.Save(&ev).Error; err != nil {
                return err
            }
            cancelEventJobs(tx, ev.ID)
            _ = tx.Where("id = ? AND book_id = ?", ev.MedicineID, ev.BookID).First(&resultMedicine).Error
            return nil
        })
        if err != nil {
            writeError(c, err)
            return
        }
        autoCompletePlan(db, ev.PlanID)
        publishEventChanged(ev.BookID, "event", "completed", ev.ID, "")
        audit.Log(db, c, "medcare_event_complete", "dose_event", ev.ID, ev.MedicineName)
        response.Success(c, gin.H{"event": eventToDTO(ev), "stock": gin.H{"deducted": sumAllocations(allocations), "shortage": shortage, "remaining": resultMedicine.StockQuantity, "allocations": allocations}})
    }
}

func sumAllocations(rows []StockAllocation) float64 {
    total := 0.0
    for _, r := range rows {
        total += r.Quantity
    }
    return total
}

func handleUndoEvent(db *gorm.DB) gin.HandlerFunc {
    return func(c *gin.Context) {
        if !requireCanRecord(c) {
            response.ErrorForbidden(c, "管理员没有把「用药记录 · 点完成」的权限给你，需要的话请联系用药本管理员")
            return
        }
        id, ok := parseUintParam(c, "id")
        if !ok {
            return
        }
        var ev DoseEvent
        err := db.Transaction(func(tx *gorm.DB) error {
            if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND book_id = ?", id, contextBookID(c)).First(&ev).Error; err != nil {
                return err
            }
            if ev.Status != EventDone && ev.Status != EventSkipped {
                return ErrInvalidTransition
            }
            if ev.AdjustmentID != nil {
                var adj PlanAdjustment
                if tx.First(&adj, *ev.AdjustmentID).Error == nil && adj.Scope != ScopeEvent {
                    return ErrInvalidTransition
                }
            }
            if ev.Status == EventDone {
                if err := restoreAllocations(tx, ev.ID, ev.BookID, currentUserID(c)); err != nil {
                    return err
                }
            }
            now := nowUTC()
            book, _ := getBook(tx, ev.BookID)
            if ev.LocalDate == todayIn(book.Timezone) {
                ev.Status = EventPending
                ev.CompletedAt = nil
                ev.CompletedBy = nil
                ev.ActualEndAt = nil
                ev.MissedAt = nil
                ev.SkippedAt = nil
                ev.SkipReason = ""
            } else {
                ev.Status = EventMissed
                ev.CompletedAt = nil
                ev.CompletedBy = nil
            }
            ev.WasMissed = false
            ev.Version++
            ev.UpdatedAt = now
            if err := tx.Save(&ev).Error; err != nil {
                return err
            }
            if ev.Status == EventPending {
                ensureEventJobs(tx, ev)
            } else {
                cancelEventJobs(tx, ev.ID)
            }
            return nil
        })
        if err != nil {
            writeError(c, err)
            return
        }
        publishEventChanged(ev.BookID, "event", "undone", ev.ID, "")
        response.Success(c, eventToDTO(ev))
    }
}

func deductFEFO(tx *gorm.DB, bookID, medicineID uint, required float64, eventID uint, today string, actorID uint) ([]StockAllocation, float64, error) {
    var batches []MedicineBatch
    if err := tx.Where("book_id = ? AND medicine_id = ? AND quantity_remaining > 0", bookID, medicineID).
        Order("CASE WHEN expiry_date = '' THEN 1 ELSE 0 END, expiry_date ASC, received_at ASC, id ASC").Find(&batches).Error; err != nil {
        return nil, 0, err
    }
    available, expired := 0.0, 0.0
    for _, b := range batches {
        if b.ExpiryDate != "" && b.ExpiryDate < today {
            expired += b.QuantityRemaining
            continue
        }
        available += b.QuantityRemaining
    }
    if available <= 0 && expired > 0 {
        return nil, 0, ErrStockExpired
    }
    remaining := required
    allocations := make([]StockAllocation, 0)
    type takeRec struct {
        batchID   uint
        qty       float64
        remaining float64
    }
    takes := make([]takeRec, 0)
    for i := range batches {
        if remaining <= 0.000001 {
            break
        }
        b := &batches[i]
        if b.ExpiryDate != "" && b.ExpiryDate < today {
            continue
        }
        take := math.Min(b.QuantityRemaining, remaining)
        if take <= 0 {
            continue
        }
        b.QuantityRemaining -= take
        b.UpdatedAt = nowUTC()
        if err := tx.Save(b).Error; err != nil {
            return nil, 0, err
        }
        takes = append(takes, takeRec{batchID: b.ID, qty: take, remaining: b.QuantityRemaining})
        remaining -= take
    }
    total := required - remaining
    if total > 0 {
        // 一次完成只记一条出库流水（可能跨多个批次），避免列表里出现多条看着像重复扣库存。
        var med Medicine
        if err := tx.Where("id = ? AND book_id = ?", medicineID, bookID).First(&med).Error; err != nil {
            return nil, 0, err
        }
        after := med.StockQuantity - total
        if after < 0 {
            after = 0
        }
        mv := StockMovement{BookID: bookID, MedicineID: medicineID, Type: "out", QuantityDelta: -total, QuantityAfter: after, EventID: &eventID, Reason: "用药完成", EntrySource: StockEntryCompletion, ActorUserID: actorID, CreatedAt: nowUTC()}
        if err := tx.Create(&mv).Error; err != nil {
            return nil, 0, err
        }
        for _, t := range takes {
            alloc := StockAllocation{BookID: bookID, EventID: eventID, BatchID: t.batchID, Quantity: t.qty, MovementID: mv.ID, CreatedAt: nowUTC()}
            if err := tx.Create(&alloc).Error; err != nil {
                return nil, 0, err
            }
            allocations = append(allocations, alloc)
        }
    }
    if err := tx.Model(&Medicine{}).Where("id = ? AND book_id = ?", medicineID, bookID).Update("stock_quantity", gorm.Expr("stock_quantity - ?", total)).Error; err != nil {
        return nil, 0, err
    }
    return allocations, remaining, nil
}

func restoreAllocations(tx *gorm.DB, eventID, bookID, actorID uint) error {
    var allocations []StockAllocation
    if err := tx.Where("event_id = ? AND book_id = ? AND reversed_at IS NULL", eventID, bookID).Find(&allocations).Error; err != nil {
        return err
    }
    if len(allocations) == 0 {
        return nil
    }
    total := 0.0
    var medID uint
    for _, alloc := range allocations {
        var batch MedicineBatch
        if err := tx.Where("id = ? AND book_id = ?", alloc.BatchID, bookID).First(&batch).Error; err != nil {
            return err
        }
        batch.QuantityRemaining += alloc.Quantity
        batch.UpdatedAt = nowUTC()
        if err := tx.Save(&batch).Error; err != nil {
            return err
        }
        total += alloc.Quantity
        medID = batch.MedicineID
    }
    // 一次撤销只记一条退回流水，与完成时的合并口径一致。
    var med Medicine
    afterQty := total
    if err := tx.Where("id = ? AND book_id = ?", medID, bookID).First(&med).Error; err == nil {
        afterQty = med.StockQuantity + total
    }
    mv := StockMovement{BookID: bookID, MedicineID: medID, Type: "return", QuantityDelta: total, QuantityAfter: afterQty, EventID: &eventID, Reason: "撤销完成", EntrySource: StockEntryReturn, ActorUserID: actorID, CreatedAt: nowUTC()}
    if err := tx.Create(&mv).Error; err != nil {
        return err
    }
    for i := range allocations {
        if err := tx.Model(&allocations[i]).Updates(map[string]interface{}{"reversed_at": nowUTC(), "movement_id": mv.ID}).Error; err != nil {
            return err
        }
    }
    if err := tx.Model(&Medicine{}).Where("id = ? AND book_id = ?", medID, bookID).Update("stock_quantity", gorm.Expr("stock_quantity + ?", total)).Error; err != nil {
        return err
    }
    return nil
}

func handleAdjustEvent(db *gorm.DB) gin.HandlerFunc {
    return func(c *gin.Context) {
        if !requireCanRecord(c) {
            response.ErrorForbidden(c, "管理员没有把「用药记录 · 点完成」的权限给你，调整用药请找管理员开通")
            return
        }
        id, ok := parseUintParam(c, "id")
        if !ok {
            return
        }
        var in struct {
            Version        uint       `json:"version"`
            AdjustmentType string     `json:"adjustment_type"`
            Scope          string     `json:"scope"`
            Changes        *planInput `json:"changes"`
            Note           string     `json:"note"`
            DoctorName     string     `json:"doctor_name"`
            Hospital       string     `json:"hospital"`
        }
        if err := c.ShouldBindJSON(&in); err != nil {
            response.ErrorBadRequest(c, "参数无效")
            return
        }
        if in.AdjustmentType != AdjDoctor && in.AdjustmentType != AdjSelf {
            response.ErrorBadRequest(c, "调整类型无效")
            return
        }
        if in.Scope != ScopeEvent && in.Scope != ScopeFromNow && in.Scope != ScopePause && in.Scope != ScopeEnd {
            response.ErrorBadRequest(c, "调整范围无效")
            return
        }
        // 只动这一次属于「用药记录 · 点完成」；改到计划本身（往后调整 / 暂停 / 终止）
        // 另需「用药计划 · 修改」，否则勾了记录的人就能悄悄改掉整条计划。
        if in.Scope != ScopeEvent && !memberHasPermission(contextBookMember(c), PermPlanEdit) {
            response.ErrorForbidden(c, "这个调整会改到计划本身，需要「用药计划 · 修改」权限；只改这一次请选「仅本次」")
            return
        }
        var ev DoseEvent
        var plan Plan
        var adj PlanAdjustment
        err := db.Transaction(func(tx *gorm.DB) error {
            if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND book_id = ?", id, contextBookID(c)).First(&ev).Error; err != nil {
                return err
            }
            if err := tx.Where("id = ? AND book_id = ?", ev.PlanID, ev.BookID).First(&plan).Error; err != nil {
                return err
            }
            if in.Version > 0 && in.Version != ev.Version {
                return ErrVersionConflict
            }
            before := encodeJSON(gin.H{"plan": planToDTO(tx, plan), "event": eventToDTO(ev)})
            reason := SkipSelf
            if in.AdjustmentType == AdjDoctor {
                reason = SkipDoctor
            }
            switch in.Scope {
            case ScopeEvent:
                markEventSkipped(tx, &ev, reason)
            case ScopeFromNow:
                if in.Changes == nil {
                    return errors.New("缺少调整内容")
                }
                book, _ := getBook(tx, plan.BookID)
                if err := in.Changes.normalize(tx, book); err != nil {
                    return err
                }
                applyPlanInput(tx, &plan, *in.Changes, book, currentUserID(c))
                plan.Version++
                if err := tx.Save(&plan).Error; err != nil {
                    return err
                }
                if err := deleteFuturePendingEvents(tx, plan.ID, nowUTC()); err != nil {
                    return err
                }
                if _, err := generateForPlanRange(tx, plan); err != nil {
                    return err
                }
                markEventSkipped(tx, &ev, reason)
            case ScopePause:
                plan.Status = PlanPaused
                plan.Version++
                plan.UpdatedAt = nowUTC()
                if err := tx.Save(&plan).Error; err != nil {
                    return err
                }
                if err := deleteFuturePendingEvents(tx, plan.ID, nowUTC()); err != nil {
                    return err
                }
                markEventSkipped(tx, &ev, reason)
            case ScopeEnd:
                plan.Status = PlanEnded
                plan.Version++
                plan.UpdatedAt = nowUTC()
                if err := tx.Save(&plan).Error; err != nil {
                    return err
                }
                if err := deleteFuturePendingEvents(tx, plan.ID, nowUTC()); err != nil {
                    return err
                }
                markEventSkipped(tx, &ev, reason)
            }
            after := encodeJSON(gin.H{"plan": planToDTO(tx, plan), "event": eventToDTO(ev)})
            adj = PlanAdjustment{
                BookID: ev.BookID, PlanID: ev.PlanID, EventID: &ev.ID, ActorUserID: currentUserID(c),
                AdjustmentType: in.AdjustmentType, Scope: in.Scope, BeforeJSON: before, AfterJSON: after,
                Note: in.Note, DoctorName: in.DoctorName, Hospital: in.Hospital, EffectiveAt: nowUTC(), CreatedAt: nowUTC(),
            }
            if err := tx.Create(&adj).Error; err != nil {
                return err
            }
            ev.AdjustmentID = &adj.ID
            if err := tx.Save(&ev).Error; err != nil {
                return err
            }
            cancelEventJobs(tx, ev.ID)
            return nil
        })
        if err != nil {
            writeError(c, err)
            return
        }
        publishEventChanged(ev.BookID, "event", "adjusted", ev.ID, "")
        audit.Log(db, c, "medcare_event_adjust", "dose_event", ev.ID, in.AdjustmentType+" "+in.Scope)
        response.Success(c, gin.H{"event": eventToDTO(ev), "adjustment": adj, "plan": planToDTO(db, plan)})
    }
}

func markEventSkipped(tx *gorm.DB, ev *DoseEvent, reason string) {
    now := nowUTC()
    ev.Status = EventSkipped
    ev.SkippedAt = &now
    ev.SkipReason = reason
    ev.Version++
    ev.UpdatedAt = now
    _ = tx.Save(ev).Error
}
