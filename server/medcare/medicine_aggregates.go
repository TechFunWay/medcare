package medcare

import (
    "smallgo/server/database"

    "gorm.io/gorm"
)

// 药品的派生数据（在用计划数、最近效期、过期批次数、操作人姓名）原先逐行查询，
// 一页 100 条要发 500 次查询。这里改成「一页一次」批量预取，查询数与条数无关。

// medicineAggregates 是一页药品共享的派生数据快照，键为药品 ID 或用户 ID。
type medicineAggregates struct {
    activePlans    map[uint]int64
    nearestExpiry  map[uint]string
    expiredBatches map[uint]int64
    userNames      map[uint]string
}

// batchExpiryStats 一次查出一页药品的最近效期与过期批次数。
// 口径与逐行查询一致：只算还有剩余数量、且填了效期的批次。
func batchExpiryStats(db *gorm.DB, bookID uint, medicineIDs []uint, today string) (map[uint]string, map[uint]int64, error) {
    nearest := make(map[uint]string, len(medicineIDs))
    expired := make(map[uint]int64, len(medicineIDs))
    if len(medicineIDs) == 0 {
        return nearest, expired, nil
    }
    var rows []struct {
        MedicineID uint   `gorm:"column:medicine_id"`
        Nearest    string `gorm:"column:nearest"`
        Expired    int64  `gorm:"column:expired"`
    }
    err := db.Model(&MedicineBatch{}).
        Select("medicine_id, MIN(expiry_date) AS nearest, SUM(CASE WHEN expiry_date < ? THEN 1 ELSE 0 END) AS expired", today).
        Where("book_id = ? AND medicine_id IN ? AND quantity_remaining > 0 AND expiry_date <> ''", bookID, medicineIDs).
        Group("medicine_id").Scan(&rows).Error
    if err != nil {
        return nil, nil, err
    }
    for _, r := range rows {
        nearest[r.MedicineID] = r.Nearest
        expired[r.MedicineID] = r.Expired
    }
    return nearest, expired, nil
}

// activePlanCounts 一次数出一页药品各自的在用计划数（进行中 + 暂停）。
func activePlanCounts(db *gorm.DB, bookID uint, medicineIDs []uint) (map[uint]int64, error) {
    counts := make(map[uint]int64, len(medicineIDs))
    if len(medicineIDs) == 0 {
        return counts, nil
    }
    var rows []struct {
        MedicineID uint  `gorm:"column:medicine_id"`
        Total      int64 `gorm:"column:total"`
    }
    err := db.Model(&Plan{}).
        Select("medicine_id, COUNT(*) AS total").
        Where("book_id = ? AND medicine_id IN ? AND status IN ?", bookID, medicineIDs, []string{PlanActive, PlanPaused}).
        Group("medicine_id").Scan(&rows).Error
    if err != nil {
        return nil, err
    }
    for _, r := range rows {
        counts[r.MedicineID] = r.Total
    }
    return counts, nil
}

// userNamesByID 一次取回一页药品涉及的操作人姓名；查不到的留空，与原有展示一致。
func userNamesByID(db *gorm.DB, userIDs []uint) (map[uint]string, error) {
    names := make(map[uint]string, len(userIDs))
    unique := make([]uint, 0, len(userIDs))
    seen := make(map[uint]bool, len(userIDs))
    for _, id := range userIDs {
        if id == 0 || seen[id] {
            continue
        }
        seen[id] = true
        unique = append(unique, id)
    }
    if len(unique) == 0 {
        return names, nil
    }
    var rows []struct {
        ID       uint   `gorm:"column:id"`
        Username string `gorm:"column:username"`
    }
    if err := db.Model(&database.User{}).Select("id, username").Where("id IN ?", unique).Scan(&rows).Error; err != nil {
        return nil, err
    }
    for _, r := range rows {
        names[r.ID] = r.Username
    }
    return names, nil
}

// loadMedicineAggregates 用三次查询取回一页药品的全部派生数据。
func loadMedicineAggregates(db *gorm.DB, bookID uint, meds []Medicine, today string) (medicineAggregates, error) {
    agg := medicineAggregates{
        activePlans:    map[uint]int64{},
        nearestExpiry:  map[uint]string{},
        expiredBatches: map[uint]int64{},
        userNames:      map[uint]string{},
    }
    if len(meds) == 0 {
        return agg, nil
    }
    medicineIDs := make([]uint, 0, len(meds))
    userIDs := make([]uint, 0, len(meds)*2)
    for _, m := range meds {
        medicineIDs = append(medicineIDs, m.ID)
        userIDs = append(userIDs, m.CreatedBy, m.UpdatedBy)
    }
    nearest, expired, err := batchExpiryStats(db, bookID, medicineIDs, today)
    if err != nil {
        return agg, err
    }
    plans, err := activePlanCounts(db, bookID, medicineIDs)
    if err != nil {
        return agg, err
    }
    names, err := userNamesByID(db, userIDs)
    if err != nil {
        return agg, err
    }
    agg.nearestExpiry, agg.expiredBatches, agg.activePlans, agg.userNames = nearest, expired, plans, names
    return agg, nil
}

// stockStatusOf 是纯函数版的库存状态判定，便于批量与单条共用同一套口径。
func stockStatusOf(m Medicine, today string, expired int64, nearest string) string {
    if m.StockQuantity <= 0 {
        return "out"
    }
    if expired > 0 {
        return "expired"
    }
    if m.LowStockThreshold > 0 && m.StockQuantity <= m.LowStockThreshold {
        return "low"
    }
    if nearest != "" && nearest <= addDays(today, m.ExpiryWarnDays) {
        return "expiring"
    }
    return "ok"
}

// buildMedicineDTO 只做字段拼装，不再碰数据库。
func buildMedicineDTO(m Medicine, today string, agg medicineAggregates) MedicineDTO {
    nearest := agg.nearestExpiry[m.ID]
    return MedicineDTO{
        ID: m.ID, BookID: m.BookID, Name: m.Name, GenericName: m.GenericName, Aliases: parseAliases(m.Aliases),
        Category: m.Category, Form: m.Form, Spec: m.Spec, Unit: m.Unit, PackageSpec: m.PackageSpec,
        Manufacturer: m.Manufacturer, Color: m.Color, CoverImageID: m.CoverImageID, Notes: m.Notes, StockQuantity: m.StockQuantity,
        LowStockThreshold: m.LowStockThreshold, ExpiryWarnDays: m.ExpiryWarnDays, UnitPrice: m.UnitPrice,
        StockStatus: stockStatusOf(m, today, agg.expiredBatches[m.ID], nearest), NearestExpiry: nearest,
        ActivePlanCount: agg.activePlans[m.ID], ArchivedAt: m.ArchivedAt, Version: m.Version,
        CreatedByName: agg.userNames[m.CreatedBy], UpdatedByName: agg.userNames[m.UpdatedBy],
        CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt,
    }
}

// medicinesToDTOs 批量组装一页药品的返回结构。
func medicinesToDTOs(db *gorm.DB, bookID uint, meds []Medicine, today string) ([]MedicineDTO, error) {
    agg, err := loadMedicineAggregates(db, bookID, meds, today)
    if err != nil {
        return nil, err
    }
    items := make([]MedicineDTO, 0, len(meds))
    for _, m := range meds {
        items = append(items, buildMedicineDTO(m, today, agg))
    }
    return items, nil
}
