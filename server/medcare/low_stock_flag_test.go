package medcare

import (
    "testing"
)

// 药品关掉「库存不足提醒」后，低库存调度应当整条跳过该药。
func TestRunLowStockNotifyRespectsMedicineFlag(t *testing.T) {
    db := setupTestDB(t)
    realtimeDB = db
    defer func() { realtimeDB = nil }()
    now := nowUTC()
    book := MedicationBook{Name: "低库存本", CareRecipientName: "李奶奶", Timezone: "Asia/Shanghai", OwnerUserID: 1, CreatedBy: 1, Status: BookActive}
    if err := db.Create(&book).Error; err != nil {
        t.Fatal(err)
    }
    db.Create(&BookMember{BookID: book.ID, UserID: 1, Role: RoleOwner, NotificationsEnabled: true, JoinedAt: now})
    db.Create(&UserPreference{UserID: 1, Timezone: "Asia/Shanghai", LowStockNotifyEnabled: true})
    m := Medicine{BookID: book.ID, Name: "停用药", Category: CatOral, Unit: "片", StockQuantity: 0, Version: 1, CreatedBy: 1, ExpiryWarnDays: 30}
    if err := db.Create(&m).Error; err != nil {
        t.Fatal(err)
    }
    var count int64
    runLowStockNotify()
    db.Model(&NotificationJob{}).Where("kind = ?", JobLowStock).Count(&count)
    if count == 0 {
        t.Fatal("默认开启时应生成低库存提醒任务")
    }
    // 关闭该药品的提醒
    if err := db.Model(&Medicine{}).Where("id = ?", m.ID).Update("low_stock_notify_disabled", true).Error; err != nil {
        t.Fatal(err)
    }
    if err := db.Where("kind = ?", JobLowStock).Delete(&NotificationJob{}).Error; err != nil {
        t.Fatal(err)
    }
    count = 0
    runLowStockNotify()
    db.Model(&NotificationJob{}).Where("kind = ?", JobLowStock).Count(&count)
    if count != 0 {
        t.Fatalf("关闭后不应生成低库存提醒任务，got %d", count)
    }
}
