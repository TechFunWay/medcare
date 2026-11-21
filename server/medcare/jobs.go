package medcare

import (
    "smallgo/server/logger"
)

func runDayEnd() {
    db := realtimeDB
    if db == nil {
        return
    }
    var books []MedicationBook
    if err := db.Where("status = ? AND deleted_at IS NULL", BookActive).Find(&books).Error; err != nil {
        logger.Error("medcare: day-end load books: %v", err)
        return
    }
    for _, book := range books {
        today := todayIn(book.Timezone)
        var events []DoseEvent
        if err := db.Where("book_id = ? AND status = ? AND local_date < ?", book.ID, EventPending, today).Find(&events).Error; err != nil {
            logger.Error("medcare: day-end load events book=%d: %v", book.ID, err)
            continue
        }
        now := nowUTC()
        for _, ev := range events {
            if err := db.Model(&ev).Updates(map[string]interface{}{"status": EventMissed, "missed_at": &now, "updated_at": now}).Error; err != nil {
                logger.Error("medcare: day-end mark event=%d: %v", ev.ID, err)
                continue
            }
            cancelEventJobs(db, ev.ID)
        }
        if len(events) > 0 {
            publishEventChanged(book.ID, "event", "day_end", 0, "")
        }
    }
}

func runStockReconcile() {
    db := realtimeDB
    if db == nil {
        return
    }
    var medicines []Medicine
    if err := db.Find(&medicines).Error; err != nil {
        logger.Error("medcare: stock reconcile load medicines: %v", err)
        return
    }
    for _, m := range medicines {
        var total float64
        db.Model(&MedicineBatch{}).Where("book_id = ? AND medicine_id = ?", m.BookID, m.ID).Select("COALESCE(SUM(quantity_remaining),0)").Scan(&total)
        if total != m.StockQuantity {
            logger.Warn("medcare: stock drift medicine=%d cached=%.3f actual=%.3f", m.ID, m.StockQuantity, total)
            db.Model(&Medicine{}).Where("id = ?", m.ID).Update("stock_quantity", total)
        }
    }
}

// runDailySummary 每分钟检查一次，到达成员配置的总结时间就生成总结任务。
func runDailySummary() {
    db := realtimeDB
    if db == nil {
        return
    }
    var prefs []UserPreference
    if err := db.Where("daily_summary_enabled = ?", true).Find(&prefs).Error; err != nil {
        logger.Error("medcare: daily summary load prefs: %v", err)
        return
    }
    now := nowUTC()
    for _, pref := range prefs {
        loc := loadLocation(pref.Timezone)
        local := now.In(loc)
        if local.Format("15:04") != pref.DailySummaryTime {
            continue
        }
        var memberships []BookMember
        db.Where("user_id = ? AND notifications_enabled = ?", pref.UserID, true).Find(&memberships)
        for _, member := range memberships {
            book, err := getBook(db, member.BookID)
            if err != nil || book.Status != BookActive {
                continue
            }
            localDate := todayIn(book.Timezone)
            createSummaryJob(db, book.ID, pref.UserID, 0, ChannelInApp, localDate, now)
            var channels []NotificationChannel
            db.Where("user_id = ? AND enabled = ? AND archived_at IS NULL AND status = ?", pref.UserID, true, "active").Find(&channels)
            for _, ch := range channels {
                createSummaryJob(db, book.ID, pref.UserID, ch.ID, ch.Type, localDate, now)
            }
        }
    }
}
