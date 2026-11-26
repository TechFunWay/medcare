package medcare

import (
    "context"
    "errors"
    "fmt"
    "sort"
    "strings"
    "time"

    "smallgo/server/logger"
    "smallgo/server/response"
    "smallgo/server/utils"

    "github.com/gin-gonic/gin"
    "gorm.io/gorm"
)

type NotificationDTO struct {
    ID        uint       `json:"id"`
    BookID    uint       `json:"book_id"`
    BookName  string     `json:"book_name"`
    EventID   *uint      `json:"event_id,omitempty"`
    Type      string     `json:"type"`
    Title     string     `json:"title"`
    Body      string     `json:"body"`
    ReadAt    *time.Time `json:"read_at,omitempty"`
    CreatedAt time.Time  `json:"created_at"`
}

type DeliveryDTO struct {
    ID          uint      `json:"id"`
    BookID      uint      `json:"book_id"`
    BookName    string    `json:"book_name"`
    ChannelType string    `json:"channel_type"`
    EventID     *uint     `json:"event_id,omitempty"`
    Kind        string    `json:"kind"`
    Status      string    `json:"status"`
    Attempts    int       `json:"attempts"`
    RunAt       time.Time `json:"run_at"`
    LastError   string    `json:"last_error,omitempty"`
    CreatedAt   time.Time `json:"created_at"`
}

func createNotificationJob(tx *gorm.DB, bookID, recipientUserID, channelID uint, channelType string, eventID *uint, kind string, runAt time.Time, payload string) {
    dedupe := fmt.Sprintf("event:%d:%s:member:%d:channel:%d", valueOrZero(eventID), kind, recipientUserID, channelID)
    upsertNotificationJob(tx, dedupe, bookID, recipientUserID, channelID, channelType, eventID, kind, runAt, payload)
}

// createDedupeJob 用调用方给定的 dedupe_key 建任务，供库存提醒、重复逾期提醒等
// 非「事件 × 固定 kind」场景使用，保证同一周期只发一次。
func createDedupeJob(tx *gorm.DB, dedupe string, bookID, recipientUserID, channelID uint, channelType string, eventID *uint, kind string, runAt time.Time, payload string) {
    upsertNotificationJob(tx, dedupe, bookID, recipientUserID, channelID, channelType, eventID, kind, runAt, payload)
}

func upsertNotificationJob(tx *gorm.DB, dedupe string, bookID, recipientUserID, channelID uint, channelType string, eventID *uint, kind string, runAt time.Time, payload string) {
    if runAt.Before(nowUTC().Add(-1 * time.Minute)) {
        return
    }
    var existing NotificationJob
    err := tx.Where("dedupe_key = ?", dedupe).First(&existing).Error
    if err == nil {
        if existing.Status == JobCanceled || existing.Status == JobFailed {
            tx.Model(&existing).Updates(map[string]interface{}{
                "status": JobPending, "run_at": runAt, "attempts": 0, "next_retry_at": nil,
                "locked_at": nil, "worker_id": "", "last_error_code": "", "last_error_message": "",
            })
        } else if existing.Status == JobPending || existing.Status == JobRetry {
            tx.Model(&existing).Update("run_at", runAt)
        }
        return
    }
    job := NotificationJob{
        BookID: bookID, RecipientUserID: recipientUserID, ChannelID: channelID, ChannelType: channelType,
        EventID: eventID, Kind: kind, ScheduledAt: runAt, RunAt: runAt, Status: JobPending,
        DedupeKey: dedupe, PayloadJSON: payload, CreatedAt: nowUTC(), UpdatedAt: nowUTC(),
    }
    _ = tx.Create(&job).Error
}

func valueOrZero(v *uint) uint {
    if v == nil {
        return 0
    }
    return *v
}

func ensureEventJobs(db *gorm.DB, ev DoseEvent) {
    var plan Plan
    if err := db.Where("id = ? AND book_id = ?", ev.PlanID, ev.BookID).First(&plan).Error; err != nil {
        return
    }
    if !plan.NotificationsEnabled {
        return
    }
    var members []BookMember
    db.Where("book_id = ? AND notifications_enabled = ?", ev.BookID, true).Find(&members)
    payload := encodeJSON(gin.H{"event_id": ev.ID, "medicine_name": ev.MedicineName, "dose_amount": ev.DoseAmount, "dose_unit": ev.DoseUnit, "type": ev.Type, "scheduled_at": ev.ScheduledAt})
    for _, member := range members {
        pref := getPreferences(db, member.UserID)
        base := ev.ScheduledAt
        if ev.SnoozeUntil != nil && ev.SnoozeUntil.After(base) {
            base = *ev.SnoozeUntil
        }
        lead := base.Add(-time.Duration(pref.LeadMinutes) * time.Minute)
        firstOverdue := base.Add(time.Duration(pref.OverdueFirstMinutes) * time.Minute)
        kinds := []struct {
            kind string
            at   time.Time
        }{
            {JobLead, lead}, {JobDue, base}, {JobOverdue1, firstOverdue},
        }
        for _, k := range kinds {
            createNotificationJob(db, ev.BookID, member.UserID, 0, ChannelInApp, &ev.ID, k.kind, k.at, payload)
        }
        var channels []NotificationChannel
        db.Where("user_id = ? AND enabled = ? AND archived_at IS NULL AND status = ?", member.UserID, true, "active").Find(&channels)
        for _, ch := range channels {
            for _, k := range kinds {
                createNotificationJob(db, ev.BookID, member.UserID, ch.ID, ch.Type, &ev.ID, k.kind, k.at, payload)
            }
        }
    }
}

func cancelEventJobs(db *gorm.DB, eventID uint) {
    db.Model(&NotificationJob{}).Where("event_id = ? AND status IN ?", eventID, []string{JobPending, JobRetry}).Update("status", JobCanceled)
}

func cancelBookJobs(db *gorm.DB, bookID uint, reason string) {
    db.Model(&NotificationJob{}).Where("book_id = ? AND status IN ?", bookID, []string{JobPending, JobRetry}).Updates(map[string]interface{}{"status": JobCanceled, "last_error_message": trimRunes(reason, 300)})
}

func cancelMemberJobs(db *gorm.DB, bookID, userID uint, reason string) {
    db.Model(&NotificationJob{}).Where("book_id = ? AND recipient_user_id = ? AND status IN ?", bookID, userID, []string{JobPending, JobRetry}).Updates(map[string]interface{}{"status": JobCanceled, "last_error_message": trimRunes(reason, 300)})
}

func cancelChannelJobs(db *gorm.DB, channelID uint, reason string) {
    db.Model(&NotificationJob{}).Where("channel_id = ? AND status IN ?", channelID, []string{JobPending, JobRetry}).Updates(map[string]interface{}{"status": JobCanceled, "last_error_message": trimRunes(reason, 300)})
}

// backfillMemberJobs 在成员加入或重新开启提醒后，补齐未来 61 天的事件任务。
// ensureEventJobs 幂等（dedupe_key 唯一），重复调用不会产生重复任务。
func backfillMemberJobs(db *gorm.DB, bookID, userID uint) {
    var events []DoseEvent
    now := nowUTC()
    db.Where("book_id = ? AND status = ? AND scheduled_at >= ? AND scheduled_at <= ?", bookID, EventPending, now, now.AddDate(0, 0, 61)).
        Order("scheduled_at ASC").Limit(600).Find(&events)
    for _, ev := range events {
        ensureEventJobs(db, ev)
    }
    _ = userID
}

// createSummaryJob 生成每日总结任务；dedupe 里带日期，保证同一天只有一条。
func createSummaryJob(tx *gorm.DB, bookID, userID, channelID uint, channelType, localDate string, runAt time.Time) {
    dedupe := fmt.Sprintf("summary:%s:member:%d:channel:%d", localDate, userID, channelID)
    var existing NotificationJob
    if err := tx.Where("dedupe_key = ?", dedupe).First(&existing).Error; err == nil {
        if existing.Status == JobCanceled || existing.Status == JobFailed {
            tx.Model(&existing).Updates(map[string]interface{}{"status": JobPending, "run_at": runAt, "attempts": 0, "next_retry_at": nil, "locked_at": nil, "worker_id": "", "last_error_code": "", "last_error_message": ""})
        }
        return
    }
    job := NotificationJob{
        BookID: bookID, RecipientUserID: userID, ChannelID: channelID, ChannelType: channelType,
        Kind: JobSummary, ScheduledAt: runAt, RunAt: runAt, Status: JobPending,
        DedupeKey: dedupe, PayloadJSON: encodeJSON(gin.H{"local_date": localDate}), CreatedAt: nowUTC(), UpdatedAt: nowUTC(),
    }
    _ = tx.Create(&job).Error
}

func handleListNotifications(db *gorm.DB) gin.HandlerFunc {
    return func(c *gin.Context) {
        page := utils.Atoi(c.Query("page"), 1)
        pageSize := utils.Atoi(c.Query("pageSize"), 30)
        if pageSize > 100 {
            pageSize = 100
        }
        q := db.Model(&InAppNotification{}).Where("user_id = ?", currentUserID(c))
        if bookID := c.Query("book_id"); bookID != "" {
            q = q.Where("book_id = ?", bookID)
        }
        var total int64
        q.Count(&total)
        var rows []InAppNotification
        if err := q.Order("created_at DESC").Offset(utils.Offset(page, pageSize)).Limit(pageSize).Find(&rows).Error; err != nil {
            response.ErrorInternal(c, "读取通知失败")
            return
        }
        items := make([]NotificationDTO, 0, len(rows))
        for _, n := range rows {
            items = append(items, NotificationDTO{ID: n.ID, BookID: n.BookID, BookName: bookName(db, n.BookID), EventID: n.EventID, Type: n.Type, Title: n.Title, Body: n.Body, ReadAt: n.ReadAt, CreatedAt: n.CreatedAt})
        }
        response.SuccessPage(c, items, total, page, pageSize)
    }
}

func handleUnreadCount(db *gorm.DB) gin.HandlerFunc {
    return func(c *gin.Context) {
        var count int64
        db.Model(&InAppNotification{}).Where("user_id = ? AND read_at IS NULL", currentUserID(c)).Count(&count)
        response.Success(c, gin.H{"count": count})
    }
}

func handleReadNotification(db *gorm.DB) gin.HandlerFunc {
    return func(c *gin.Context) {
        id, ok := parseUintParam(c, "id")
        if !ok {
            return
        }
        now := nowUTC()
        result := db.Model(&InAppNotification{}).Where("id = ? AND user_id = ?", id, currentUserID(c)).Update("read_at", &now)
        if result.RowsAffected == 0 {
            response.ErrorNotFound(c, "通知不存在")
            return
        }
        response.Success(c, gin.H{"ok": true})
    }
}

func handleReadAllNotifications(db *gorm.DB) gin.HandlerFunc {
    return func(c *gin.Context) {
        now := nowUTC()
        db.Model(&InAppNotification{}).Where("user_id = ? AND read_at IS NULL", currentUserID(c)).Update("read_at", &now)
        response.Success(c, gin.H{"ok": true})
    }
}

func handleListDeliveries(db *gorm.DB) gin.HandlerFunc {
    return func(c *gin.Context) {
        page := utils.Atoi(c.Query("page"), 1)
        pageSize := utils.Atoi(c.Query("pageSize"), 30)
        if pageSize > 100 {
            pageSize = 100
        }
        q := db.Model(&NotificationJob{}).Where("recipient_user_id = ?", currentUserID(c))
        if bookID := c.Query("book_id"); bookID != "" {
            q = q.Where("book_id = ?", bookID)
        }
        var total int64
        q.Count(&total)
        var rows []NotificationJob
        if err := q.Order("created_at DESC").Offset(utils.Offset(page, pageSize)).Limit(pageSize).Find(&rows).Error; err != nil {
            response.ErrorInternal(c, "读取投递记录失败")
            return
        }
        items := make([]DeliveryDTO, 0, len(rows))
        for _, j := range rows {
            items = append(items, DeliveryDTO{ID: j.ID, BookID: j.BookID, BookName: bookName(db, j.BookID), ChannelType: j.ChannelType, EventID: j.EventID, Kind: j.Kind, Status: j.Status, Attempts: j.Attempts, RunAt: j.RunAt, LastError: j.LastErrorMessage, CreatedAt: j.CreatedAt})
        }
        response.SuccessPage(c, items, total, page, pageSize)
    }
}

func handleRetryDelivery(db *gorm.DB) gin.HandlerFunc {
    return func(c *gin.Context) {
        id, ok := parseUintParam(c, "id")
        if !ok {
            return
        }
        now := nowUTC()
        // 卡死的 sending 也允许手动重试，但只认锁已过期的，避免把正在发送的
        // 任务重复触发；同时清掉锁字段，防止残留。
        result := db.Model(&NotificationJob{}).Where("id = ? AND recipient_user_id = ? AND status IN ?", id, currentUserID(c), []string{JobFailed, JobCanceled, JobSending}).
            Where("status <> ? OR locked_at IS NULL OR locked_at < ?", JobSending, now.Add(-staleSendingAfter)).
            Updates(map[string]interface{}{"status": JobPending, "run_at": now, "attempts": 0, "next_retry_at": nil, "locked_at": nil, "worker_id": "", "last_error_code": "", "last_error_message": ""})
        if result.Error != nil {
            response.ErrorInternal(c, "重试失败")
            return
        }
        if result.RowsAffected == 0 {
            response.ErrorBadRequest(c, "该投递不能重试")
            return
        }
        response.Success(c, gin.H{"ok": true})
    }
}

// staleSendingAfter 判定「发送中」任务卡死的时长：远大于单次发送超时（20 秒），
// 避免把正在发送的任务误抢走。
const staleSendingAfter = 5 * time.Minute

// reclaimStaleSendingJobs 把进程重启/崩溃遗留在 sending 的任务放回 retry。
// 没有这一步，卡在 sending 的任务既不会被 worker 捞起（只捞 pending/retry），
// 也不满足手动重试的状态条件，提醒会永久丢失且无法补救。
func reclaimStaleSendingJobs(db *gorm.DB) int64 {
    now := nowUTC()
    res := db.Model(&NotificationJob{}).
        Where("status = ? AND (locked_at IS NULL OR locked_at < ?)", JobSending, now.Add(-staleSendingAfter)).
        Updates(map[string]interface{}{"status": JobRetry, "locked_at": nil, "worker_id": "", "run_at": now, "updated_at": now})
    if res.Error != nil {
        logger.Error("medcare: reclaim stale notification jobs: %v", res.Error)
        return 0
    }
    if res.RowsAffected > 0 {
        logger.Info("medcare: 回收了 %d 个卡在发送中的通知任务", res.RowsAffected)
    }
    return res.RowsAffected
}

// mergeableKindGroup 把任务按「能否合并进同一条消息」归类：
// 用药提醒类的多个事件可以合成一条；每日总结、库存提醒、测试各自独立。
func mergeableKindGroup(kind string) string {
    switch kind {
    case JobSummary:
        return "summary"
    case JobLowStock:
        return "lowstock"
    case JobTest:
        return "test"
    default:
        return "reminder"
    }
}

func runNotificationWorker() {
    db := realtimeDB
    if db == nil {
        return
    }
    reclaimStaleSendingJobs(db)
    var jobs []NotificationJob
    if err := db.Where("status IN ? AND run_at <= ?", []string{JobPending, JobRetry}, nowUTC()).Order("run_at ASC").Limit(80).Find(&jobs).Error; err != nil {
        logger.Error("medcare: load notification jobs: %v", err)
        return
    }
    batches, singles := partitionReminderJobs(jobs)
    for _, batch := range batches {
        processNotificationJobBatch(db, batch)
    }
    for _, job := range singles {
        processNotificationJob(db, job)
    }
}

// buildMergedReminderMessage 把多条用药提醒合成一条消息：
// 每条占两行（药品（类型）剂量 / 原计划时间与超时时长），按计划时间排序。
func buildMergedReminderMessage(bookName string, events []DoseEvent, allOverdue bool, now time.Time) notificationMessage {
    sorted := make([]DoseEvent, len(events))
    copy(sorted, events)
    sort.Slice(sorted, func(a, b int) bool { return sorted[a].ScheduledAt.Before(sorted[b].ScheduledAt) })
    lines := make([]string, 0, len(sorted)*2)
    for _, ev := range sorted {
        lines = append(lines, fmt.Sprintf("%s（%s） %.3g %s", ev.MedicineName, categoryLabelCN(ev.Type), ev.DoseAmount, ev.DoseUnit))
        planLine := "原计划 " + ev.LocalTime
        if !ev.ScheduledAt.IsZero() {
            if overdue := now.Sub(ev.ScheduledAt); overdue > time.Minute {
                planLine += "，已超时 " + humanDurationCN(overdue)
            }
        }
        lines = append(lines, planLine)
    }
    title := fmt.Sprintf("用药提醒 · %d 项", len(sorted))
    if allOverdue {
        title = fmt.Sprintf("还没吃药 · 再次提醒（%d 项）", len(sorted))
    }
    return notificationMessage{BookName: bookName, Title: title, Lines: lines}
}

// partitionReminderJobs 把「能合并进一条消息」的用药提醒按
// 用药本 + 收件人 + 渠道 分组；其余任务原样返回、逐条处理。
// 只有一个成员的批次也并入 singles，避免为了单条走批处理。
func partitionReminderJobs(jobs []NotificationJob) (batches [][]NotificationJob, singles []NotificationJob) {
    type batchKey struct {
        bookID    uint
        userID    uint
        channelID uint
        channel   string
    }
    grouped := map[batchKey][]NotificationJob{}
    order := make([]batchKey, 0)
    for _, job := range jobs {
        if job.EventID != nil && job.ChannelType != ChannelInApp && mergeableKindGroup(job.Kind) == "reminder" {
            k := batchKey{job.BookID, job.RecipientUserID, job.ChannelID, job.ChannelType}
            if _, ok := grouped[k]; !ok {
                order = append(order, k)
            }
            grouped[k] = append(grouped[k], job)
            continue
        }
        singles = append(singles, job)
    }
    for _, k := range order {
        if len(grouped[k]) >= 2 {
            batches = append(batches, grouped[k])
        } else {
            singles = append(singles, grouped[k]...)
        }
    }
    return batches, singles
}

// processNotificationJobBatch 把同一渠道的一批用药提醒合成一条消息发出。
// 已被完成/跳过的条目直接取消，不计入正文；全部失效时整批取消、不发空消息。
func processNotificationJobBatch(db *gorm.DB, jobs []NotificationJob) {
    if len(jobs) == 0 {
        return
    }
    now := nowUTC()
    ids := make([]uint, 0, len(jobs))
    for _, j := range jobs {
        ids = append(ids, j.ID)
    }
    res := db.Model(&NotificationJob{}).Where("id IN ? AND status IN ?", ids, []string{JobPending, JobRetry}).
        Updates(map[string]interface{}{"status": JobSending, "locked_at": &now, "worker_id": "medcare-worker"})
    if res.RowsAffected == 0 {
        return
    }
    var claimed []NotificationJob
    db.Where("id IN ? AND status = ?", ids, JobSending).Find(&claimed)
    if len(claimed) == 0 {
        return
    }
    first := claimed[0]
    book, err := getBook(db, first.BookID)
    if err != nil {
        for _, j := range claimed {
            markJobFailed(db, j, "BOOK_NOT_FOUND", "用药本不存在", 0, true)
        }
        return
    }
    type item struct {
        job NotificationJob
        ev  DoseEvent
    }
    items := make([]item, 0, len(claimed))
    var canceled []uint
    allOverdue := true
    for _, j := range claimed {
        if j.EventID == nil {
            canceled = append(canceled, j.ID)
            continue
        }
        var ev DoseEvent
        if err := db.Where("id = ? AND book_id = ?", *j.EventID, j.BookID).First(&ev).Error; err != nil {
            canceled = append(canceled, j.ID)
            continue
        }
        if ev.Status == EventDone || ev.Status == EventSkipped {
            canceled = append(canceled, j.ID)
            continue
        }
        if mergeableKindGroup(j.Kind) == "reminder" {
            switch j.Kind {
            case JobOverdue1, JobOverdue2:
            default:
                if !strings.HasPrefix(j.Kind, JobOverdueRepeatPrefix) {
                    allOverdue = false
                }
            }
        }
        items = append(items, item{job: j, ev: ev})
    }
    if len(canceled) > 0 {
        for _, id := range canceled {
            markJobCanceled(db, id)
        }
    }
    if len(items) == 0 {
        return
    }
    events := make([]DoseEvent, 0, len(items))
    for _, it := range items {
        events = append(events, it.ev)
    }
    msg := buildMergedReminderMessage(book.Name, events, allOverdue, now)
    msg.Link = publicBaseURL(db, nil) + "/today"

    start := nowUTC()
    var ch NotificationChannel
    if err := db.Where("id = ? AND user_id = ? AND archived_at IS NULL", first.ChannelID, first.RecipientUserID).First(&ch).Error; err != nil {
        for _, it := range items {
            markJobFailed(db, it.job, "CHANNEL_NOT_FOUND", "通知渠道不存在", 0, true)
        }
        return
    }
    if !ch.Enabled || ch.Status != "active" {
        for _, it := range items {
            markJobFailed(db, it.job, "CHANNEL_DISABLED", "通知渠道已停用", 0, true)
        }
        return
    }
    sendCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
    defer cancel()
    _, sendErr := sendChannelMessage(sendCtx, db, ch, msg)
    latency := time.Since(start).Milliseconds()
    if sendErr == nil {
        for _, it := range items {
            db.Model(&NotificationJob{}).Where("id = ?", it.job.ID).Updates(map[string]interface{}{
                "status": JobSent, "attempts": it.job.Attempts + 1, "locked_at": nil,
                "last_error_code": "", "last_error_message": "", "updated_at": nowUTC(),
            })
            recordAttempt(db, it.job.ID, it.job.Attempts+1, start, "success", "", latency, fmt.Sprintf("merged:%d", len(items)))
        }
        return
    }
    var de *deliveryError
    permanent := errors.As(sendErr, &de) && de.Permanent
    code := "SEND_FAILED"
    if errors.As(sendErr, &de) {
        code = de.Code
    }
    for _, it := range items {
        markJobFailed(db, it.job, code, sendErr.Error(), latency, permanent)
    }
}

func processNotificationJob(db *gorm.DB, job NotificationJob) {
    now := nowUTC()
    res := db.Model(&NotificationJob{}).Where("id = ? AND status IN ?", job.ID, []string{JobPending, JobRetry}).Updates(map[string]interface{}{"status": JobSending, "locked_at": &now, "worker_id": "medcare-worker"})
    if res.RowsAffected == 0 {
        return
    }
    var ev DoseEvent
    if job.EventID != nil {
        if err := db.Where("id = ? AND book_id = ?", *job.EventID, job.BookID).First(&ev).Error; err != nil {
            markJobCanceled(db, job.ID)
            return
        }
        if ev.Status == EventDone || ev.Status == EventSkipped {
            markJobCanceled(db, job.ID)
            return
        }
    }
    book, err := getBook(db, job.BookID)
    if err != nil {
        markJobFailed(db, job, "BOOK_NOT_FOUND", "用药本不存在", 0, true)
        return
    }
    pref := getPreferences(db, job.RecipientUserID)
    msg := notificationMessage{BookName: book.Name, Title: notificationJobTitle(job.Kind, ev), Lines: notificationJobLines(ev, pref), Link: publicBaseURL(db, nil) + "/today"}
    if job.Kind == JobSummary {
        localDate := todayIn(book.Timezone)
        var payload struct {
            LocalDate string `json:"local_date"`
        }
        _ = decodeJSON(job.PayloadJSON, &payload)
        if payload.LocalDate != "" {
            localDate = payload.LocalDate
        }
        msg.Title = "今日用药总结 · " + localDate
        msg.Lines = dailySummaryLines(db, book.ID, localDate)
    }
    if job.Kind == JobLowStock {
        var p struct {
            MedicineName string  `json:"medicine_name"`
            Stock        float64 `json:"stock"`
            Unit         string  `json:"unit"`
            Threshold    float64 `json:"threshold"`
            Level        string  `json:"level"`
        }
        _ = decodeJSON(job.PayloadJSON, &p)
        if p.Level == "out" {
            msg.Title = "药品缺货提醒 · " + p.MedicineName
        } else {
            msg.Title = "库存不足提醒 · " + p.MedicineName
        }
        line := fmt.Sprintf("当前库存 %.3g %s", p.Stock, p.Unit)
        if p.Threshold > 0 {
            line += fmt.Sprintf("，低库存阈值 %.3g %s", p.Threshold, p.Unit)
        }
        msg.Lines = []string{line, "记得及时补货"}
        msg.Link = publicBaseURL(db, nil) + "/medicines"
    }
    if strings.HasPrefix(job.Kind, JobOverdueRepeatPrefix) {
        msg.Title = "还没吃药 · 再次提醒"
    }
    start := nowUTC()
    var sendErr error
    if job.ChannelType == ChannelInApp {
        n := InAppNotification{BookID: job.BookID, UserID: job.RecipientUserID, EventID: job.EventID, Type: notificationInAppType(job.Kind), Title: msg.Title, Body: strings.Join(msg.Lines, " · "), CreatedAt: nowUTC()}
        if err := db.Create(&n).Error; err != nil {
            sendErr = &deliveryError{Code: "INAPP_WRITE_FAILED", Message: "写入站内通知失败"}
        } else {
            publishNotification(n)
        }
    } else {
        var ch NotificationChannel
        if err := db.Where("id = ? AND user_id = ? AND archived_at IS NULL", job.ChannelID, job.RecipientUserID).First(&ch).Error; err != nil {
            sendErr = &deliveryError{Code: "CHANNEL_NOT_FOUND", Message: "通知渠道不存在", Permanent: true}
        } else if !ch.Enabled || ch.Status != "active" {
            sendErr = &deliveryError{Code: "CHANNEL_DISABLED", Message: "通知渠道已停用", Permanent: true}
        } else {
            sendCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
            defer cancel()
            _, sendErr = sendChannelMessage(sendCtx, db, ch, msg)
        }
    }
    latency := time.Since(start).Milliseconds()
    if sendErr == nil {
        db.Model(&job).Updates(map[string]interface{}{"status": JobSent, "attempts": job.Attempts + 1, "locked_at": nil, "last_error_code": "", "last_error_message": "", "updated_at": nowUTC()})
        recordAttempt(db, job.ID, job.Attempts+1, start, "success", "", latency, "")
        return
    }
    var de *deliveryError
    permanent := errors.As(sendErr, &de) && de.Permanent
    code := "SEND_FAILED"
    if errors.As(sendErr, &de) {
        code = de.Code
    }
    markJobFailed(db, job, code, sendErr.Error(), latency, permanent)
}

func markJobCanceled(db *gorm.DB, id uint) {
    db.Model(&NotificationJob{}).Where("id = ?", id).Update("status", JobCanceled)
}

func markJobFailed(db *gorm.DB, job NotificationJob, code, message string, latency int64, permanent bool) {
    attempts := job.Attempts + 1
    now := nowUTC()
    if permanent || attempts >= 5 {
        db.Model(&NotificationJob{}).Where("id = ?", job.ID).Updates(map[string]interface{}{"status": JobFailed, "attempts": attempts, "locked_at": nil, "last_error_code": code, "last_error_message": trimRunes(message, 300), "updated_at": now})
        recordAttempt(db, job.ID, attempts, now, "permanent", code, latency, message)
        return
    }
    backoff := []int{1, 2, 4, 8, 16}
    minutes := backoff[0]
    if attempts-1 < len(backoff) {
        minutes = backoff[attempts-1]
    }
    next := now.Add(time.Duration(minutes) * time.Minute)
    db.Model(&NotificationJob{}).Where("id = ?", job.ID).Updates(map[string]interface{}{"status": JobRetry, "attempts": attempts, "run_at": next, "next_retry_at": &next, "locked_at": nil, "last_error_code": code, "last_error_message": trimRunes(message, 300), "updated_at": now})
    recordAttempt(db, job.ID, attempts, now, "temporary", code, latency, message)
}

func recordAttempt(db *gorm.DB, jobID uint, attemptNo int, started time.Time, result, code string, latency int64, summary string) {
    db.Create(&NotificationAttempt{JobID: jobID, AttemptNo: attemptNo, StartedAt: started, FinishedAt: nowUTC(), Result: result, ProviderCode: code, LatencyMS: latency, ResponseSummary: trimRunes(summary, 300)})
}

func notificationJobTitle(kind string, ev DoseEvent) string {
    switch kind {
    case JobLead:
        return "用药提醒 · 即将到点"
    case JobOverdue1:
        return "逾期提醒"
    case JobOverdue2:
        return "再次提醒"
    case JobSummary:
        return "今日用药总结"
    case JobRemind:
        return "有人提醒你用药"
    case JobTest:
        return "用药管理测试提醒"
    default:
        return "用药提醒"
    }
}

// categoryLabelCN 把药品分类翻成中文，用在通知里说明这是一条口服/注射/输液的药。
func categoryLabelCN(cat string) string {
    switch cat {
    case CatOral:
        return "口服"
    case CatInjection:
        return "注射"
    case CatInfusion:
        return "输液"
    case CatTopical:
        return "外用"
    default:
        return "其他"
    }
}

// humanDurationCN 把时长写成「X 天 Y 小时 Z 分钟」，用于告诉用户超时了多久。
func humanDurationCN(d time.Duration) string {
    if d < 0 {
        d = 0
    }
    total := int(d.Minutes())
    days := total / (24 * 60)
    hours := (total % (24 * 60)) / 60
    mins := total % 60
    parts := make([]string, 0, 3)
    if days > 0 {
        parts = append(parts, fmt.Sprintf("%d 天", days))
    }
    if hours > 0 {
        parts = append(parts, fmt.Sprintf("%d 小时", hours))
    }
    if mins > 0 || len(parts) == 0 {
        parts = append(parts, fmt.Sprintf("%d 分钟", mins))
    }
    return strings.Join(parts, " ")
}

func notificationJobLines(ev DoseEvent, pref UserPreference) []string {
    if ev.ID == 0 {
        return []string{"这是一条测试消息。"}
    }
    // 第一行先说明药品与剂型，用户一眼能分清这条是吃药还是打针输液。
    lines := []string{fmt.Sprintf("%s（%s） %.3g %s", ev.MedicineName, categoryLabelCN(ev.Type), ev.DoseAmount, ev.DoseUnit)}
    // 原计划时间 + 已超时时长：到点前只显示计划时间，过点后补上超时多久。
    planLine := "原计划 " + ev.LocalTime
    if !ev.ScheduledAt.IsZero() {
        if overdue := nowUTC().Sub(ev.ScheduledAt); overdue > time.Minute {
            planLine += "，已超时 " + humanDurationCN(overdue)
        }
    }
    lines = append(lines, planLine)
    if ev.LocationName != "" && pref.IncludeLocationInNotifications {
        line := "地点：" + ev.LocationName
        if ev.Address != "" {
            line += " · " + ev.Address
        }
        lines = append(lines, line)
    }
    if ev.ExecutorName != "" {
        label := map[string]string{"self": "自己", "family": "家人", "nurse": "护士", "doctor": "医生", "other": "其他"}[ev.ExecutorType]
        if label == "" {
            label = "执行人"
        }
        lines = append(lines, "执行："+label+" "+ev.ExecutorName)
    }
    if ev.Type == "infusion" && (ev.DripRate > 0 || ev.DurationMinutes > 0) {
        detail := "输液："
        if ev.DripRate > 0 {
            detail += fmt.Sprintf("滴速 %d 滴/分", ev.DripRate)
        }
        if ev.DurationMinutes > 0 {
            if ev.DripRate > 0 {
                detail += " · "
            }
            detail += fmt.Sprintf("预计 %d 分钟", ev.DurationMinutes)
        }
        lines = append(lines, detail)
    }
    if ev.Note != "" && pref.IncludeNotesInNotifications {
        lines = append(lines, "备注："+ev.Note)
    }
    return lines
}

func dailySummaryLines(db *gorm.DB, bookID uint, localDate string) []string {
    var events []DoseEvent
    db.Where("book_id = ? AND local_date = ?", bookID, localDate).Find(&events)
    var done, missed, doctor, self, other, pending int
    for _, ev := range events {
        switch ev.Status {
        case EventDone:
            done++
        case EventMissed:
            missed++
        case EventPending:
            pending++
        case EventSkipped:
            switch ev.SkipReason {
            case SkipDoctor:
                doctor++
            case SkipSelf:
                self++
            default:
                other++
            }
        }
    }
    denom := done + missed + doctor + self + other
    rate := 0
    if denom > 0 {
        rate = int(float64(done)/float64(denom)*100 + 0.5)
    }
    return []string{
        fmt.Sprintf("完成 %d 项 · 漏服 %d 项 · 待完成 %d 项", done, missed, pending),
        fmt.Sprintf("医生调整 %d 项 · 自己调整 %d 项 · 其他跳过 %d 项", doctor, self, other),
        fmt.Sprintf("完成率 %d%%", rate),
    }
}

func notificationInAppType(kind string) string {
    switch {
    case kind == JobOverdue1 || kind == JobOverdue2 || strings.HasPrefix(kind, JobOverdueRepeatPrefix):
        return "dose_overdue"
    case kind == JobLowStock:
        return "low_stock"
    case kind == JobSummary:
        return "daily_summary"
    case kind == JobRemind:
        return "dose_remind"
    default:
        return "dose_due"
    }
}

// runOverdueReminders 每分钟检查一次：没按时完成、还在待完成的用药，
// 按成员设置的重复间隔（最小 1 分钟）反复生成提醒任务，直到点完成/漏服/跳过。
// 用「时间桶」做 dedupe，同一周期只会生成一条，不会重复轰炸。
func runOverdueReminders() {
    db := realtimeDB
    if db == nil {
        return
    }
    now := nowUTC()
    var events []DoseEvent
    if err := db.Where("status = ? AND scheduled_at <= ?", EventPending, now).Limit(500).Find(&events).Error; err != nil {
        logger.Error("medcare: load overdue events: %v", err)
        return
    }
    for _, ev := range events {
        var plan Plan
        if err := db.Where("id = ? AND book_id = ?", ev.PlanID, ev.BookID).First(&plan).Error; err != nil {
            continue
        }
        if !plan.NotificationsEnabled {
            continue
        }
        var members []BookMember
        db.Where("book_id = ? AND notifications_enabled = ?", ev.BookID, true).Find(&members)
        if len(members) == 0 {
            continue
        }
        payload := encodeJSON(gin.H{"event_id": ev.ID, "medicine_name": ev.MedicineName, "dose_amount": ev.DoseAmount, "dose_unit": ev.DoseUnit, "type": ev.Type, "scheduled_at": ev.ScheduledAt})
        for _, member := range members {
            pref := getPreferences(db, member.UserID)
            repeat := pref.OverdueRepeatMinutes
            if repeat < 1 {
                repeat = 1
            }
            base := ev.ScheduledAt
            if ev.SnoozeUntil != nil && ev.SnoozeUntil.After(base) {
                base = *ev.SnoozeUntil
            }
            first := base.Add(time.Duration(pref.OverdueFirstMinutes) * time.Minute)
            step := time.Duration(repeat) * time.Minute
            // 第一次逾期由预生成的 overdue_1 负责，这里只补后续重复。
            if now.Before(first.Add(step)) {
                continue
            }
            bucket := int(now.Sub(first) / step)
            if bucket < 1 || bucket > 500 {
                continue
            }
            kind := fmt.Sprintf("%s_%d", JobOverdueRepeatPrefix, bucket)
            runAt := now
            eventID := ev.ID
            createDedupeJob(db, fmt.Sprintf("event:%d:%s:member:%d:channel:0", ev.ID, kind, member.UserID), ev.BookID, member.UserID, 0, ChannelInApp, &eventID, kind, runAt, payload)
            var channels []NotificationChannel
            db.Where("user_id = ? AND enabled = ? AND archived_at IS NULL AND status = ?", member.UserID, true, "active").Find(&channels)
            for _, ch := range channels {
                createDedupeJob(db, fmt.Sprintf("event:%d:%s:member:%d:channel:%d", ev.ID, kind, member.UserID, ch.ID), ev.BookID, member.UserID, ch.ID, ch.Type, &eventID, kind, runAt, payload)
            }
        }
    }
}

// runLowStockNotify 每 10 分钟检查一次库存：低于阈值或缺货时经消息渠道提醒，
// 同一药品同一级别每天只提醒一次，避免刷屏。
func runLowStockNotify() {
    db := realtimeDB
    if db == nil {
        return
    }
    now := nowUTC()
    var books []MedicationBook
    if err := db.Where("status = ?", BookActive).Find(&books).Error; err != nil {
        return
    }
    for _, bk := range books {
        today := todayIn(bk.Timezone)
        var meds []Medicine
        db.Where("book_id = ? AND archived_at IS NULL", bk.ID).Find(&meds)
        for _, m := range meds {
            // 该药品关掉了低库存提醒就整条跳过（不再使用的药不必天天提醒补货）。
            if m.LowStockNotifyDisabled {
                continue
            }
            level := ""
            if m.StockQuantity <= 0 {
                level = "out"
            } else if m.LowStockThreshold > 0 && m.StockQuantity <= m.LowStockThreshold {
                level = "low"
            }
            if level == "" {
                continue
            }
            var members []BookMember
            db.Where("book_id = ? AND notifications_enabled = ?", bk.ID, true).Find(&members)
            if len(members) == 0 {
                continue
            }
            payload := encodeJSON(gin.H{"medicine_id": m.ID, "medicine_name": m.Name, "stock": m.StockQuantity, "unit": m.Unit, "threshold": m.LowStockThreshold, "level": level})
            for _, mb := range members {
                pref := getPreferences(db, mb.UserID)
                if !pref.LowStockNotifyEnabled {
                    continue
                }
                createDedupeJob(db, fmt.Sprintf("lowstock:%d:%s:%s:member:%d:channel:0", m.ID, level, today, mb.UserID), bk.ID, mb.UserID, 0, ChannelInApp, nil, JobLowStock, now, payload)
                var channels []NotificationChannel
                db.Where("user_id = ? AND enabled = ? AND archived_at IS NULL AND status = ?", mb.UserID, true, "active").Find(&channels)
                for _, ch := range channels {
                    createDedupeJob(db, fmt.Sprintf("lowstock:%d:%s:%s:member:%d:channel:%d", m.ID, level, today, mb.UserID, ch.ID), bk.ID, mb.UserID, ch.ID, ch.Type, nil, JobLowStock, now, payload)
                }
            }
        }
    }
}
