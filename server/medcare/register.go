package medcare

import (
    "time"

    "smallgo/server/apps"
    "smallgo/server/database"
    "smallgo/server/scheduler"
    "smallgo/server/sysconfig"

    "github.com/gin-gonic/gin"
    "gorm.io/gorm"
)

func init() {
    database.RegisterModels(
        &MedicationBook{}, &BookMember{}, &BookInvite{},
        &Medicine{}, &MedicineBatch{}, &StockMovement{}, &StockAllocation{}, &CareLocation{},
        &Plan{}, &PlanAdjustment{}, &DoseEvent{},
        &PrescriptionImport{}, &BookImage{},
        &NotificationChannel{}, &NotificationJob{}, &NotificationAttempt{}, &InAppNotification{},
        &UserPreference{}, &SystemNotificationConfig{}, &SystemAIConfig{}, &SystemOCRConfig{}, &BookRevision{},
        &OperationLog{},
    )
    apps.Register(apps.App{
        Name:        "medcare",
        DisplayName: "用药管理",
        Icon:        "pill",
        NavPosition: 50,
        SetupAuth:   setupAuthRoutes,
        SetupAdmin:  setupAdminRoutes,
        Migrate:     migrate,
    })
    sysconfig.RegisterConfig(sysconfig.ConfigDef{Key: "medcare_default_theme_mode", Scope: sysconfig.ScopeSystem, Type: sysconfig.TypeSelect, Default: "system", Public: true, Group: "appearance", Label: "默认主题模式", Description: "新用户默认的亮/暗模式：system 跟随系统、light 亮色、dark 暗色", Options: []string{"system", "light", "dark"}})
    sysconfig.RegisterConfig(sysconfig.ConfigDef{Key: "medcare_default_theme_brand", Scope: sysconfig.ScopeSystem, Type: sysconfig.TypeSelect, Default: "a", Public: true, Group: "appearance", Label: "默认主题配色", Description: "新用户默认的配色方案：a 温润药本、b 清风医疗、c 深夜护航、d 柔光珊瑚、e 素简黑白、f 海盐青柠、g 紫丁护理、h 暖咖玫瑰、i 琥珀晨光、j 樱粉心意", Options: []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j"}})
	sysconfig.RegisterConfig(sysconfig.ConfigDef{Key: "medcare_register_approval", Scope: sysconfig.ScopeSystem, Type: sysconfig.TypeBool, Default: "false", Public: true, Group: "security", Label: "普通用户注册需审核", Description: "打开后，新注册的普通账号需管理员在「用户管理」里通过审核才能登录；首个注册的管理员不受影响。"})
	// 药品基础信息的可选字典（剂型 / 规格 / 单位 / 包装规格），由超级管理员在「偏好设置」里维护，
	// 存成逗号分隔的字符串，公开可读，供各页面下拉使用；为空时前端回落到内置默认值。
	for _, d := range []struct{ key, label, desc string }{
		{"medcare_form_options", "剂型选项", "药品剂型下拉的候选值，逗号分隔，例如：片剂,胶囊,注射液"},
		{"medcare_unit_options", "库存单位选项", "库存单位下拉的候选值，逗号分隔，例如：片,盒,支,瓶"},
		{"medcare_spec_options", "规格选项", "规格下拉的候选值，逗号分隔，例如：5mg,10mg,0.2g×10片"},
		{"medcare_package_spec_options", "包装规格选项", "包装规格下拉的候选值，逗号分隔，例如：10片/盒,100片/瓶"},
	} {
		sysconfig.RegisterConfig(sysconfig.ConfigDef{Key: d.key, Scope: sysconfig.ScopeSystem, Type: sysconfig.TypeString, Default: "", Public: true, Group: "medicine", Label: d.label, Description: d.desc})
	}

    scheduler.Register(scheduler.Job{Name: "medcare_notification_worker", Interval: time.Minute, RunAtStart: true, Run: runNotificationWorker})
    scheduler.Register(scheduler.Job{Name: "medcare_day_end", Daily: "00:10", Run: runDayEnd})
    scheduler.Register(scheduler.Job{Name: "medcare_daily_summary", Interval: time.Minute, Run: runDailySummary})
    scheduler.Register(scheduler.Job{Name: "medcare_stock_reconcile", Daily: "03:30", Run: runStockReconcile})
    // 没按时吃药：每分钟按成员设置的间隔反复提醒，直到点完成。
    scheduler.Register(scheduler.Job{Name: "medcare_overdue_reminder", Interval: time.Minute, RunAtStart: true, Run: runOverdueReminders})
    // 库存不足 / 缺货：每 10 分钟扫描一次，按天去重后走消息渠道。
    scheduler.Register(scheduler.Job{Name: "medcare_low_stock", Interval: 10 * time.Minute, RunAtStart: true, Run: runLowStockNotify})
}

func migrate(db *gorm.DB) error {
    ensureSystemConfigs(db)
    // 药品字典的出厂默认值：仅在该键还没有记录时写入（已有则尊重管理员的增删改）。
    seedMedicineDict(db)
    return nil
}

func setupAuthRoutes(api *gin.RouterGroup, db *gorm.DB) {
    realtimeDB = db
    // 处方识别 worker：固定并发 + 有界队列，并收尾上次进程中断的识别记录。
    startPrescriptionWorkers(db)
    g := api.Group("/medcare")

    g.GET("/books", handleListBooks(db))
    g.POST("/books", handleCreateBook(db))
    g.POST("/invites/join-by-code", handleJoinByCode(db))
    g.GET("/invites/:token", handlePreviewInvite(db))
    g.POST("/invites/:token/join", handleJoinInvite(db))

    // 账号与飞牛绑定：解除绑定需要登录态，放在业务鉴权组里。
    g.POST("/account/fnos-unbind", handleUnbindFnOS(db))

    g.GET("/preferences", handleGetPreferences(db))
    g.PUT("/preferences", handleUpdatePreferences(db))
    // 账号级「已支持」：落库到用户偏好，跨设备共享；匿名计数尽力上报、失败不影响落库。
    g.POST("/donate/support", handleDonateSupport(db))

    g.GET("/channels", handleListChannels(db))
    g.POST("/channels", handleCreateChannel(db))
    g.GET("/channels/:id/config", handleGetChannelConfig(db))
    g.PUT("/channels/:id", handleUpdateChannel(db))
    g.DELETE("/channels/:id", handleDeleteChannel(db))
    g.POST("/channels/:id/test", handleTestChannel(db))

    g.GET("/notifications", handleListNotifications(db))
    g.GET("/notifications/unread-count", handleUnreadCount(db))
    g.POST("/notifications/:id/read", handleReadNotification(db))
    g.POST("/notifications/read-all", handleReadAllNotifications(db))
    g.GET("/deliveries", handleListDeliveries(db))
    g.POST("/deliveries/:id/retry", handleRetryDelivery(db))
    g.GET("/revision", handleRevision(db))
    g.GET("/stream", handleRealtimeStream)

    book := g.Group("/books/:bookId")
    // 所有写操作统一记入用药本操作日志，便于家人追溯。
    book.Use(bookOperationLogger(db))
    // 用药本壳接口：只要求是成员——前端要靠它拿到自己的权限勾选，才知道该显示什么。
    book.GET("", requireBookMember(db, RoleMember), handleGetBook(db))
    book.GET("/operation-logs", requirePermission(db, PermStatsView), handleListOperationLogs(db))
    // 用药本设置与归属：永远只认管理员，不进能力表。
    book.PUT("", requireBookMember(db, RoleOwner), handleUpdateBook(db))
    book.POST("/archive", requireBookMember(db, RoleOwner), handleArchiveBook(db, true))
    book.POST("/restore", requireBookMember(db, RoleOwner), handleArchiveBook(db, false))
    book.DELETE("", requireBookMember(db, RoleOwner), handleDeleteBook(db))
    book.POST("/transfer", requireBookMember(db, RoleOwner), handleTransferBook(db))

    book.GET("/members", requirePermission(db, PermMemberView), handleListMembers(db))
    // 自己的通知订阅：本人自助，不需要额外权限。
    book.PUT("/members/me/notifications", requireBookMember(db, RoleMember), handleMyNotification(db))
    // 改角色、设权限、移除成员、指定用药人：只认管理员，避免把管理权下放后收不回来。
    book.PUT("/members/:userId", requireBookMember(db, RoleOwner), handleUpdateMemberRole(db))
    book.PUT("/members/:userId/permissions", requireBookMember(db, RoleOwner), handleUpdateMemberPermissions(db))
    book.DELETE("/members/:userId", requireBookMember(db, RoleOwner), handleRemoveMember(db))
    book.PUT("/members/care-recipient", requireBookMember(db, RoleOwner), handleSetCareRecipient(db))
    // 能力矩阵清单与各角色默认：任何成员可读，便于说明「我为什么点不了」。
    book.GET("/permissions", requireBookMember(db, RoleMember), handlePermissionCatalog)
    book.POST("/leave", requireBookMember(db, RoleMember), handleLeaveBook(db))

    book.GET("/invites", requirePermission(db, PermMemberView), handleListInvites(db))
    book.POST("/invites", requirePermission(db, PermMemberInvite), handleCreateInvite(db))
    book.DELETE("/invites/:inviteId", requirePermission(db, PermMemberView), handleRevokeInvite(db))

    // 药品与库存：查看 / 新增 / 修改 / 删除四格各自放行。
    book.GET("/medicines", requirePermission(db, PermMedicineView), handleListMedicines(db))
    book.GET("/medicines-stats", requirePermission(db, PermMedicineView), handleMedicineStats(db))
    book.GET("/medicines/:id", requirePermission(db, PermMedicineView), handleGetMedicine(db))
    book.GET("/medicines/:id/batches", requirePermission(db, PermMedicineView), handleListBatches(db))
    book.GET("/medicines/:id/stock-movements", requirePermission(db, PermMedicineView), handleStockMovements(db))
    book.GET("/stock-movements/stats", requirePermission(db, PermMedicineView), handleStockMovementStats(db))
    book.GET("/stock-movements", requirePermission(db, PermMedicineView), handleListBookStockMovements(db))
    book.GET("/stock-alerts", requirePermission(db, PermMedicineView), handleStockAlerts(db))
    book.POST("/medicines", requirePermission(db, PermMedicineCreate), handleCreateMedicine(db))
    book.POST("/medicines/import", requirePermission(db, PermMedicineCreate), handleImportMedicines(db))
    book.POST("/medicines/:id/stock-in", requirePermission(db, PermMedicineCreate), handleStockIn(db))
    // 入库记录 7 天内可改：按批次定位，改数量/批号/有效期/供应商/备注/来源。
    book.GET("/batches/:batchId", requirePermission(db, PermMedicineView), handleGetBatch(db))
    book.PUT("/batches/:batchId", requirePermission(db, PermMedicineEdit), handleUpdateStockInBatch(db))
    book.POST("/medicines/:id/stock-out", requirePermission(db, PermMedicineCreate), handleStockOut(db))
    book.PUT("/medicines/:id/cover", requirePermission(db, PermMedicineEdit), handleSetMedicineCover(db))
    book.GET("/medicines/:id/relations", requirePermission(db, PermMedicineView), handleMedicineRelations(db))
    book.PUT("/medicines/:id/unit", requirePermission(db, PermMedicineEdit), handleConvertMedicineUnit(db))
    book.PUT("/medicines/:id", requirePermission(db, PermMedicineEdit), handleUpdateMedicine(db))
    book.POST("/medicines/:id/stock-adjust", requirePermission(db, PermMedicineEdit), handleStockAdjust(db))
    book.POST("/medicines/:id/stocktake", requirePermission(db, PermMedicineEdit), handleStocktake(db))
    book.POST("/medicines/:id/restore", requirePermission(db, PermMedicineEdit), handleArchiveMedicine(db, false))
    // 录错用药本时把药品整体转移到另一个用药本：转出改药、转入相当于新增，
    // 所以目标本那边要求「药品与库存 · 新增」（见 resolveTransferTarget）。
    book.POST("/medicines/:id/transfer", requirePermission(db, PermMedicineEdit), handleTransferMedicine(db))
    book.DELETE("/medicines/:id", requirePermission(db, PermMedicineDelete), handleArchiveMedicine(db, true))

    // 地点库属于「用药计划」模块：打针输液要在计划里选地点。
    book.GET("/locations", requirePermission(db, PermPlanView), handleListLocations(db))
    book.POST("/locations", requirePermission(db, PermPlanCreate), handleCreateLocation(db))
    book.PUT("/locations/:id", requirePermission(db, PermPlanEdit), handleUpdateLocation(db))
    book.POST("/locations/:id/restore", requirePermission(db, PermPlanEdit), handleArchiveLocation(db, false))
    book.DELETE("/locations/:id", requirePermission(db, PermPlanDelete), handleArchiveLocation(db, true))

    book.GET("/plans", requirePermission(db, PermPlanView), handleListPlans(db))
    book.GET("/plans/:id", requirePermission(db, PermPlanView), handleGetPlan(db))
    book.GET("/plans/:id/preview", requirePermission(db, PermPlanView), handlePreviewPlan(db))
    book.POST("/plans", requirePermission(db, PermPlanCreate), handleCreatePlan(db))
    book.PUT("/plans/:id", requirePermission(db, PermPlanEdit), handleUpdatePlan(db))
    book.POST("/plans/:id/pause", requirePermission(db, PermPlanEdit), handlePlanStatus(db, PlanPaused))
    book.POST("/plans/:id/resume", requirePermission(db, PermPlanEdit), handlePlanStatus(db, PlanActive))
    book.POST("/plans/:id/end", requirePermission(db, PermPlanEdit), handlePlanStatus(db, PlanEnded))
    book.POST("/plans/:id/transfer", requirePermission(db, PermPlanEdit), handleTransferPlan(db))
    book.DELETE("/plans/:id", requirePermission(db, PermPlanDelete), handleDeletePlan(db))

    // 今日与日历：查看 / 点提醒 / 点完成三格。
    book.GET("/dashboard", requirePermission(db, PermTodayView), handleDashboard(db))
    book.GET("/calendar", requirePermission(db, PermTodayView), handleCalendar(db))
    book.GET("/dose-events", requirePermission(db, PermTodayView), handleListEvents(db))
    book.GET("/dose-events/:id", requirePermission(db, PermTodayView), handleGetEvent(db))
    // 提醒用药：给用药人与能帮忙记录的家人发即时提醒，不动数据。
    book.POST("/dose-events/:id/remind", requirePermission(db, PermTodayRemind), handleRemindEvent(db))
    book.POST("/dose-events/:id/start", requirePermission(db, PermTodayRecord), handleStartEvent(db))
    book.POST("/dose-events/:id/complete", requirePermission(db, PermTodayRecord), handleCompleteEvent(db))
    book.POST("/dose-events/:id/undo", requirePermission(db, PermTodayRecord), handleUndoEvent(db))
    book.POST("/dose-events/:id/miss", requirePermission(db, PermTodayRecord), handleMarkMissed(db))
    book.POST("/dose-events/:id/snooze", requirePermission(db, PermTodayRecord), handleSnoozeEvent(db))
    book.POST("/dose-events/:id/location", requirePermission(db, PermTodayRecord), handleEventLocation(db))
    book.POST("/dose-events/:id/note", requirePermission(db, PermTodayRecord), handleEventNote(db))
    // 调整：单次调整只需「用药记录 · 点完成」，改往后/暂停/终止会动到计划，
    // handler 内再要求「用药计划 · 修改」。
    book.POST("/dose-events/:id/adjust", requirePermission(db, PermTodayRecord), handleAdjustEvent(db))

    book.GET("/stats/overview", requirePermission(db, PermStatsView), handleStatsOverview(db))
    book.GET("/stats/trend", requirePermission(db, PermStatsView), handleStatsTrend(db))
    book.GET("/stats/heatmap", requirePermission(db, PermStatsView), handleStatsHeatmap(db))
    book.GET("/stats/breakdown", requirePermission(db, PermStatsView), handleStatsBreakdown(db))
    book.GET("/stats/stock", requirePermission(db, PermStatsView), handleStatsStock(db))

    // 处方识别：查看 / 识别 / 确认录入 / 删除。确认入领会建档入库，
    // handler 内另要求「药品与库存 · 新增」，排计划再另要求「用药计划 · 新建」。
    book.GET("/prescriptions", requirePermission(db, PermPrescView), handleListPrescriptions(db))
    book.GET("/prescriptions/:id", requirePermission(db, PermPrescView), handleGetPrescription(db))
    book.GET("/prescriptions/:id/image", requirePermission(db, PermPrescView), handlePrescriptionImage(db))
    book.POST("/prescriptions/recognize", requirePermission(db, PermPrescCreate), handleRecognizePrescription(db))
    book.POST("/prescriptions/:id/retry", requirePermission(db, PermPrescEdit), handleRetryPrescription(db))
    book.POST("/prescriptions/:id/confirm", requirePermission(db, PermPrescEdit), handleConfirmPrescription(db))
    book.DELETE("/prescriptions/:id", requirePermission(db, PermPrescDelete), handleDeletePrescription(db))

    // 图片资料：查看 / 上传 / 编辑 / 删除。图片可能含病历隐私，单独一档。
    book.GET("/images", requirePermission(db, PermImageView), handleListImages(db))
    book.GET("/images/:id/file", requirePermission(db, PermImageView), handleImageFile(db))
    book.POST("/images", requirePermission(db, PermImageCreate), handleUploadImages(db))
    book.PUT("/images/:id", requirePermission(db, PermImageEdit), handleUpdateImage(db))
    book.DELETE("/images/:id", requirePermission(db, PermImageDelete), handleDeleteImage(db))

}

func setupAdminRoutes(api *gin.RouterGroup, db *gorm.DB) {
    // 本地上传备份：与框架 backup.RegisterRoutes 同层挂在 adminGroup 上
    // （POST /api/backups/upload）。上传只入库成一份备份，恢复仍走列表里
    // 既有的「恢复」按钮，复用确认弹窗与恢复前快照。
    api.POST("/backups/upload", handleUploadBackup(db))

    admin := api.Group("/medcare/admin")
    admin.GET("/smtp", handleGetSMTPConfig(db))
    admin.PUT("/smtp", handleSaveSMTPConfig(db))
    admin.POST("/smtp/test", handleTestSMTPConfig(db))
    admin.GET("/ai", handleGetAIConfig(db))
    admin.PUT("/ai", handleSaveAIConfig(db))
    admin.POST("/ai/test", handleTestAIConfig(db))
    admin.POST("/ai/models", handleListAIModels(db))
    admin.GET("/ocr", handleGetOCRConfig(db))
    admin.PUT("/ocr", handleSaveOCRConfig(db))
    admin.POST("/ocr/test", handleTestOCRConfig(db))
    admin.GET("/deliveries", handleAdminDeliveries(db))
    admin.POST("/deliveries/:id/retry", handleAdminRetryDelivery(db))
    admin.GET("/health", handleAdminHealth(db))
}
