package medcare

import (
    "errors"
    "fmt"

    "smallgo/server/audit"
    "smallgo/server/response"

    "github.com/gin-gonic/gin"
    "gorm.io/gorm"
)

type transferInput struct {
    TargetBookID uint `json:"target_book_id"`
}

// resolveTransferTarget 校验目标用药本存在、且当前用户对目标本也有对应的管理能力。
// perm 决定目标本要查哪一格：转移药品要求对方本有「药品与库存 · 新增」，
// 转移计划要求「用药计划 · 新建」——转过去等于在目标本里建数据。
func resolveTransferTarget(db *gorm.DB, c *gin.Context, sourceBookID uint, perm string) (MedicationBook, uint, error) {
    var in transferInput
    if err := c.ShouldBindJSON(&in); err != nil || in.TargetBookID == 0 {
        return MedicationBook{}, 0, errors.New("请选择目标用药本")
    }
    if in.TargetBookID == sourceBookID {
        return MedicationBook{}, 0, errors.New("源用药本与目标用药本相同")
    }
    target, err := getBook(db, in.TargetBookID)
    if err != nil || target.Status != BookActive {
        return MedicationBook{}, 0, errors.New("目标用药本不存在或已归档")
    }
    userID := currentUserID(c)
    member, err := getMember(db, target.ID, userID)
    if err != nil || !memberHasPermission(member, perm) {
        return MedicationBook{}, 0, errors.New("你在目标用药本没有「" + permissionLabel(perm) + "」的权限")
    }
    return target, userID, nil
}

// handleTransferMedicine 把药品及其批次、库存流水、用药计划、用药记录整体转移到另一个用药本。
// 用于「录错用药本」的纠正场景。
func handleTransferMedicine(db *gorm.DB) gin.HandlerFunc {
    return func(c *gin.Context) {
        id, ok := parseUintParam(c, "id")
        if !ok {
            return
        }
        sourceBookID := contextBookID(c)
        var medicine Medicine
        if err := db.Where("id = ? AND book_id = ?", id, sourceBookID).First(&medicine).Error; err != nil {
            response.ErrorNotFound(c, "药品不存在")
            return
        }
        target, userID, err := resolveTransferTarget(db, c, sourceBookID, PermMedicineCreate)
        if err != nil {
            response.ErrorBadRequest(c, err.Error())
            return
        }
        var moved struct {
            Batches   int64 `json:"batches"`
            Movements int64 `json:"movements"`
            Plans     int64 `json:"plans"`
            Events    int64 `json:"events"`
        }
        err = db.Transaction(func(tx *gorm.DB) error {
            var planIDs, batchIDs, eventIDs []uint
            tx.Model(&Plan{}).Where("book_id = ? AND medicine_id = ?", sourceBookID, medicine.ID).Pluck("id", &planIDs)
            tx.Model(&MedicineBatch{}).Where("book_id = ? AND medicine_id = ?", sourceBookID, medicine.ID).Pluck("id", &batchIDs)
            tx.Model(&DoseEvent{}).Where("book_id = ? AND medicine_id = ?", sourceBookID, medicine.ID).Pluck("id", &eventIDs)

            res := tx.Model(&Medicine{}).Where("id = ? AND book_id = ?", medicine.ID, sourceBookID).Updates(map[string]interface{}{"book_id": target.ID, "updated_by": userID, "updated_at": nowUTC()})
            moved.Batches = tx.Model(&MedicineBatch{}).Where("book_id = ? AND medicine_id = ?", sourceBookID, medicine.ID).Update("book_id", target.ID).RowsAffected
            moved.Movements = tx.Model(&StockMovement{}).Where("book_id = ? AND medicine_id = ?", sourceBookID, medicine.ID).Update("book_id", target.ID).RowsAffected
            moved.Plans = tx.Model(&Plan{}).Where("book_id = ? AND medicine_id = ?", sourceBookID, medicine.ID).Updates(map[string]interface{}{"book_id": target.ID, "updated_by": userID, "updated_at": nowUTC()}).RowsAffected
            moved.Events = tx.Model(&DoseEvent{}).Where("book_id = ? AND medicine_id = ?", sourceBookID, medicine.ID).Update("book_id", target.ID).RowsAffected
            if res.RowsAffected == 0 {
                return errors.New("转移药品失败")
            }
            if len(planIDs) > 0 {
                tx.Model(&PlanAdjustment{}).Where("book_id = ? AND plan_id IN ?", sourceBookID, planIDs).Update("book_id", target.ID)
            }
            if len(batchIDs) > 0 {
                tx.Model(&StockAllocation{}).Where("book_id = ? AND batch_id IN ?", sourceBookID, batchIDs).Update("book_id", target.ID)
            }
            if len(eventIDs) > 0 {
                tx.Model(&StockAllocation{}).Where("book_id = ? AND event_id IN ?", sourceBookID, eventIDs).Update("book_id", target.ID)
                // 源本成员不一定在目标本，迁移后取消这些事件未发的提醒，避免错发给别人。
                tx.Model(&NotificationJob{}).Where("book_id = ? AND event_id IN ? AND status IN ?", sourceBookID, eventIDs, []string{JobPending, JobRetry}).Updates(map[string]interface{}{"status": JobCanceled, "last_error_message": "用药数据已转移到其它用药本"})
            }
            return nil
        })
        if err != nil {
            response.ErrorBadRequest(c, err.Error())
            return
        }
        audit.Log(db, c, "medcare_medicine_transfer", "medicine", medicine.ID, fmt.Sprintf("to book %d(%s)", target.ID, target.Name))
        response.Success(c, gin.H{"ok": true, "target_book_id": target.ID, "target_book_name": target.Name, "moved": moved})
    }
}

// handleTransferPlan 把用药计划及其用药记录转移到另一个用药本；
// 要求该计划对应的药品已在目标本（否则数据不完整）。
func handleTransferPlan(db *gorm.DB) gin.HandlerFunc {
    return func(c *gin.Context) {
        id, ok := parseUintParam(c, "id")
        if !ok {
            return
        }
        sourceBookID := contextBookID(c)
        var plan Plan
        if err := db.Where("id = ? AND book_id = ?", id, sourceBookID).First(&plan).Error; err != nil {
            response.ErrorNotFound(c, "用药计划不存在")
            return
        }
        target, userID, err := resolveTransferTarget(db, c, sourceBookID, PermPlanCreate)
        if err != nil {
            response.ErrorBadRequest(c, err.Error())
            return
        }
        var medicine Medicine
        if err := db.Where("id = ? AND book_id = ?", plan.MedicineID, target.ID).First(&medicine).Error; err != nil {
            response.ErrorBadRequest(c, "目标用药本里没有该计划用的药品，请先转移药品")
            return
        }
        var movedEvents int64
        err = db.Transaction(func(tx *gorm.DB) error {
            var eventIDs []uint
            tx.Model(&DoseEvent{}).Where("book_id = ? AND plan_id = ?", sourceBookID, plan.ID).Pluck("id", &eventIDs)
            res := tx.Model(&Plan{}).Where("id = ? AND book_id = ?", plan.ID, sourceBookID).Updates(map[string]interface{}{"book_id": target.ID, "updated_by": userID, "updated_at": nowUTC()})
            if res.RowsAffected == 0 {
                return errors.New("转移计划失败")
            }
            movedEvents = tx.Model(&DoseEvent{}).Where("book_id = ? AND plan_id = ?", sourceBookID, plan.ID).Update("book_id", target.ID).RowsAffected
            tx.Model(&PlanAdjustment{}).Where("book_id = ? AND plan_id = ?", sourceBookID, plan.ID).Update("book_id", target.ID)
            if len(eventIDs) > 0 {
                tx.Model(&StockAllocation{}).Where("book_id = ? AND event_id IN ?", sourceBookID, eventIDs).Update("book_id", target.ID)
                tx.Model(&NotificationJob{}).Where("book_id = ? AND event_id IN ? AND status IN ?", sourceBookID, eventIDs, []string{JobPending, JobRetry}).Updates(map[string]interface{}{"status": JobCanceled, "last_error_message": "用药数据已转移到其它用药本"})
            }
            return nil
        })
        if err != nil {
            response.ErrorBadRequest(c, err.Error())
            return
        }
        audit.Log(db, c, "medcare_plan_transfer", "plan", plan.ID, fmt.Sprintf("to book %d(%s)", target.ID, target.Name))
        response.Success(c, gin.H{"ok": true, "target_book_id": target.ID, "target_book_name": target.Name, "moved_events": movedEvents})
    }
}
