package medcare

import (
    "smallgo/server/response"
    "smallgo/server/stats"
    "smallgo/server/version"

    "github.com/gin-gonic/gin"
    "gorm.io/gorm"
)

// handleDonateSupport 记录账号级「已支持」状态（2026-11-16 由前端 localStorage 改到后端）：
// 写进 user_preferences.donate_supported_version，同账号在手机 / 电脑 / 飞牛入口共享，
// 换设备登录不会再被重复提示。匿名赞赏计数只是尽力上报：统计被退出开关或运行时配置
// 关闭、门户暂时连不上，都不影响「已支持」落库——用户明确点过的支持不该被网络问题抹掉。
// 计数放后台 goroutine 带重试跑，不阻塞这次点击的响应。
func handleDonateSupport(db *gorm.DB) gin.HandlerFunc {
    return func(c *gin.Context) {
        // 支持金额（元）：可选字段，缺省为 0（不校验支付，仅作金额标注），钳位交给 stats。
        var payload struct {
            Amount float64 `json:"amount"`
        }
        _ = c.ShouldBindJSON(&payload)
        uid := currentUserID(c)
        // 先确保偏好行存在（新用户首次点击时还没有行），再更新支持版本。
        getPreferences(db, uid)
        ver := version.Version
        if err := db.Model(&UserPreference{}).Where("user_id = ?", uid).
            Update("donate_supported_version", ver).Error; err != nil {
            response.ErrorInternal(c, "记录支持状态失败，请稍后再试")
            return
        }
        if stats.Enabled(db) {
            amount := payload.Amount
            go func() { _ = stats.SendDonate(amount) }()
        }
        response.Success(c, gin.H{"ok": true, "supported_version": ver})
    }
}
