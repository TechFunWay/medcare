package medcare

import "errors"

var (
    ErrNotFound          = errors.New("记录不存在")
    ErrForbidden         = errors.New("没有权限")
    ErrVersionConflict   = errors.New("数据已被其他成员更新，请刷新后重试")
    ErrMedicineInUse     = errors.New("药品仍被活动计划使用")
    ErrStockShortage     = errors.New("库存不足")
    ErrStockExpired      = errors.New("仅剩过期批次")
    ErrInvalidTransition = errors.New("当前状态不允许此操作")
    ErrChannelInvalid    = errors.New("通知渠道配置无效")
    ErrInviteInvalid     = errors.New("邀请已失效")
    ErrAIUnavailable     = errors.New("AI 服务不可用")
    ErrAISchemaInvalid   = errors.New("AI 返回结果无法解析")
    ErrValidation        = errors.New("参数不合法")
)
