package medcare

import (
    "fmt"
    "testing"
    "time"

    "gorm.io/gorm"
)

// seedStatusMedicine 造一条处于指定库存状态的药品，口径与 stockStatusOf 对应。
func seedStatusMedicine(t *testing.T, db *gorm.DB, name string, stock, threshold float64, expiry string) Medicine {
    t.Helper()
    now := nowUTC()
    m := Medicine{BookID: 1, Name: name, Category: CatOral, Unit: "片", StockQuantity: stock,
        LowStockThreshold: threshold, ExpiryWarnDays: 30, Version: 1, CreatedBy: 1, UpdatedBy: 1,
        CreatedAt: now, UpdatedAt: now}
    if err := db.Create(&m).Error; err != nil {
        t.Fatalf("create medicine: %v", err)
    }
    batch := MedicineBatch{BookID: 1, MedicineID: m.ID, QuantityInitial: 50, QuantityRemaining: stock, ExpiryDate: expiry, ReceivedAt: now}
    if err := db.Create(&batch).Error; err != nil {
        t.Fatalf("create batch: %v", err)
    }
    return m
}

// 每个库存状态筛选都必须与该药品自身的 stock_status 一致，且 total 与实际条数相等。
func TestStockStatusFilterMatchesStatus(t *testing.T) {
    db, _ := setupMedicineListDB(t)
    today := todayIn(BeijingTimezone)
    seedStatusMedicine(t, db, "缺货药", 0, 10, addDays(today, 200))
    seedStatusMedicine(t, db, "过期药", 50, 10, addDays(today, -1))
    seedStatusMedicine(t, db, "低库存药", 5, 10, addDays(today, 200))
    seedStatusMedicine(t, db, "临期药", 50, 10, addDays(today, 10))
    seedStatusMedicine(t, db, "正常药", 50, 10, addDays(today, 200))

    for _, status := range []string{"out", "expired", "low", "expiring", "ok"} {
        items, total := decodeMedicinePage(t, listMedicinesRequest(t, db, "pageSize=100&stock_status="+status))
        if int64(len(items)) != total {
            t.Fatalf("%s: total=%d 与实际条数 %d 不一致", status, total, len(items))
        }
        for _, it := range items {
            if it.StockStatus != status {
                t.Fatalf("%s 筛选返回了状态为 %s 的药品 %s", status, it.StockStatus, it.Name)
            }
        }
    }

    // 逐个状态确认命中的正是预期药品，防止 SQL 条件放宽或收紧。
    want := map[string]string{"out": "缺货药", "expired": "过期药", "low": "低库存药", "expiring": "临期药", "ok": "正常药"}
    for status, name := range want {
        items, _ := decodeMedicinePage(t, listMedicinesRequest(t, db, "pageSize=100&stock_status="+status))
        if len(items) != 1 || items[0].Name != name {
            t.Fatalf("%s 命中的药品应为 %s，实际 %+v", status, name, items)
        }
    }
}

// 翻页必须不漏不重：原先 expiring / expired 在分页之后过滤，会漏掉后面的条目。
func TestStockStatusFilterPaginatesWithoutGaps(t *testing.T) {
    db, _ := setupMedicineListDB(t)
    today := todayIn(BeijingTimezone)
    const want = 7
    for i := 0; i < want; i++ {
        seedStatusMedicine(t, db, fmt.Sprintf("临期药%d", i+1), 50, 10, addDays(today, 10))
    }
    // 混入其他状态，确认不会被带出来。
    seedStatusMedicine(t, db, "正常药", 50, 10, addDays(today, 200))
    seedStatusMedicine(t, db, "过期药", 50, 10, addDays(today, -1))

    seen := map[uint]bool{}
    var firstTotal int64
    for page := 1; page <= 4; page++ {
        items, total := decodeMedicinePage(t, listMedicinesRequest(t, db, fmt.Sprintf("pageSize=2&page=%d&stock_status=expiring", page)))
        if page == 1 {
            firstTotal = total
            if total != want {
                t.Fatalf("total = %d, want %d", total, want)
            }
        }
        if total != firstTotal {
            t.Fatalf("第 %d 页 total 变了: %d", page, total)
        }
        for _, it := range items {
            if it.StockStatus != "expiring" {
                t.Fatalf("第 %d 页混入状态 %s", page, it.StockStatus)
            }
            if seen[it.ID] {
                t.Fatalf("药品 %d 跨页重复出现", it.ID)
            }
            seen[it.ID] = true
        }
    }
    if len(seen) != want {
        t.Fatalf("翻页共取到 %d 条，want %d（存在漏数据）", len(seen), want)
    }
}

// 带状态筛选时归档过滤仍然生效，且不把归档药品算进 total。
func TestStockStatusFilterRespectsArchive(t *testing.T) {
    db, _ := setupMedicineListDB(t)
    today := todayIn(BeijingTimezone)
    keep := seedStatusMedicine(t, db, "在用临期药", 50, 10, addDays(today, 10))
    archived := seedStatusMedicine(t, db, "归档临期药", 50, 10, addDays(today, 10))
    at := nowUTC()
    if err := db.Model(&Medicine{}).Where("id = ?", archived.ID).Update("archived_at", at).Error; err != nil {
        t.Fatalf("archive: %v", err)
    }

    items, total := decodeMedicinePage(t, listMedicinesRequest(t, db, "stock_status=expiring"))
    if total != 1 || len(items) != 1 || items[0].ID != keep.ID {
        t.Fatalf("归档药品未被排除: total=%d items=%+v", total, items)
    }

    items, total = decodeMedicinePage(t, listMedicinesRequest(t, db, "stock_status=expiring&include_archived=1"))
    if total != 2 || len(items) != 2 {
        t.Fatalf("include_archived=1 应含归档: total=%d len=%d", total, len(items))
    }
}

// 时区数据库缺失时也不能返回 nil，否则 time.In(nil) 会 panic。
func TestLoadLocationNeverNil(t *testing.T) {
    for _, tz := range []string{"Asia/Shanghai", "UTC", "Invalid/Zone", ""} {
        loc := loadLocation(tz)
        if loc == nil {
            t.Fatalf("%q 返回了 nil 时区", tz)
        }
        if _, offset := time.Now().In(loc).Zone(); offset == 0 && loc.String() != "UTC" {
            t.Fatalf("%q 的偏移量异常: %d", tz, offset)
        }
    }
    // 业务时区必须稳定是东八区。
    if _, offset := time.Now().In(loadLocation(BeijingTimezone)).Zone(); offset != 8*60*60 {
        t.Fatalf("北京时间偏移 = %d, want %d", offset, 8*60*60)
    }
}
