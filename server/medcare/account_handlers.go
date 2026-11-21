package medcare

import (
    "smallgo/server/audit"
    "smallgo/server/database"
    "smallgo/server/response"

    "github.com/gin-gonic/gin"
    "gorm.io/gorm"
)

// handleUnbindFnOS 解除当前应用账号与飞牛 NAS 账号的绑定。
// 必须已设置登录密码，否则解绑后将无法再登录。
func handleUnbindFnOS(db *gorm.DB) gin.HandlerFunc {
    return func(c *gin.Context) {
        userID := currentUserID(c)
        var user database.User
        if err := db.First(&user, userID).Error; err != nil {
            response.ErrorNotFound(c, "账号不存在")
            return
        }
        if user.FnOSUserID == nil {
            response.Success(c, gin.H{"ok": true, "fnos_bound": false})
            return
        }
        if user.Password == "" {
            response.ErrorBadRequest(c, "请先设置登录密码再解除飞牛绑定，否则将无法登录")
            return
        }
        if err := db.Model(&user).Updates(map[string]interface{}{"fn_os_user_id": nil, "fn_os_username": ""}).Error; err != nil {
            response.ErrorInternal(c, "解除绑定失败")
            return
        }
        audit.Log(db, c, "fnos_unbind", "user", userID, "解除飞牛 NAS 绑定")
        response.Success(c, gin.H{"ok": true, "fnos_bound": false})
    }
}
