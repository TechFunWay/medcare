package user

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"smallgo/server/audit"
	"smallgo/server/database"
	"smallgo/server/response"
	"smallgo/server/sysconfig"
	"smallgo/server/utils"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type fnOSGatewayContextKey struct{}

// MarkFnOSGateway marks a request connection that arrived through the fnOS
// Unix-socket gateway. TCP clients can send the same header names, so headers
// are only trusted when this marker was applied by server.Start's ConnContext.
func MarkFnOSGateway(ctx context.Context) context.Context {
	return context.WithValue(ctx, fnOSGatewayContextKey{}, true)
}

// IsFnOSGatewayRequest reports whether the request arrived on the fnOS gateway
// Unix socket (as opposed to the public TCP listener).
func IsFnOSGatewayRequest(r *http.Request) bool {
	return r.Context().Value(fnOSGatewayContextKey{}) == true
}

func parseUserID(c *gin.Context) (uint, bool) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil || id == 0 {
		response.ErrorBadRequest(c, "无效的用户 ID")
		return 0, false
	}
	return uint(id), true
}

func handleSetupRequired(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var count int64
		if err := db.Model(&database.User{}).Count(&count).Error; err != nil {
			response.ErrorInternal(c, "检查初始化状态失败")
			return
		}
		response.Success(c, map[string]interface{}{
			"setup_required": count == 0,
		})
	}
}

func handleRegister(db *gorm.DB, gatewayPrefix string) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			Username string `json:"username" binding:"required"`
			Password string `json:"password" binding:"required"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			response.ErrorBadRequest(c, "请输入用户名和密码")
			return
		}

		secret := getJWTSecret(db)
		result, err := Register(db, req.Username, req.Password, secret)
		if err != nil {
			switch {
			case errors.Is(err, ErrRegisterDisabled):
				response.Error(c, http.StatusForbidden, response.CodeRegisterDisabled, err.Error())
			case errors.Is(err, ErrUserExists):
				response.Error(c, http.StatusConflict, response.CodeUserExists, err.Error())
			case errors.Is(err, ErrPasswordTooShort), errors.Is(err, ErrPasswordTooLong):
				response.ErrorBadRequest(c, err.Error())
			default:
				response.ErrorInternal(c, "注册失败")
			}
			return
		}

		// 网关域上 Authorization 头会被飞牛接入层拦截，种 HttpOnly 会话
		// Cookie 维持登录态（直连端口仍走 JWT，不种）。
		issueSessionCookie(c, db, gatewayPrefix, result)
		response.Success(c, result)
	}
}

func handleLogin(db *gorm.DB, gatewayPrefix string) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			Username    string `json:"username" binding:"required"`
			Password    string `json:"password" binding:"required"`
			PasswordMd5 string `json:"password_md5"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			response.ErrorBadRequest(c, "请输入用户名和密码")
			return
		}

		secret := getJWTSecret(db)
		result, err := Login(db, req.Username, req.Password, req.PasswordMd5, secret)
		if err != nil {
			if errors.Is(err, ErrPendingApproval) {
				response.Error(c, http.StatusForbidden, response.CodeForbidden, err.Error())
				return
			}
			response.Error(c, http.StatusUnauthorized, response.CodeInvalidCredentials, err.Error())
			return
		}

		// 网关域上 Authorization 头会被飞牛接入层拦截，种 HttpOnly 会话
		// Cookie 维持登录态（直连端口仍走 JWT，不种）。
		issueSessionCookie(c, db, gatewayPrefix, result)

		// Populate context so the audit entry is attributed to the logged-in user.
		if u, ok := result["user"].(map[string]interface{}); ok {
			if uid, ok := u["id"].(uint); ok {
				c.Set("userID", uid)
				c.Set("username", req.Username)
			}
		}
		audit.Log(db, c, "login", "user", c.GetUint("userID"), "用户登录")

		response.Success(c, result)
	}
}

// fnOSIdentity resolves the fnOS identity for this request. Requests arriving on
// the gateway's Unix socket carry the identity headers injected by the gateway
// and cannot be forged there. Requests on the public TCP listener must instead
// present a one-time ticket minted by the gateway; the SPA is handed over to
// that listener because the gateway intercepts the application's own
// Authorization credentials.
func fnOSIdentity(c *gin.Context) (FnOSIdentity, bool) {
	if c.Request.Context().Value(fnOSGatewayContextKey{}) == true {
		uid, err := strconv.ParseUint(c.GetHeader("X-Trim-Userid"), 10, 32)
		username := c.GetHeader("X-Trim-Username")
		if err != nil || uid == 0 || username == "" {
			response.ErrorUnauthorized(c, "未获取到飞牛 NAS 登录信息")
			return FnOSIdentity{}, false
		}
		return FnOSIdentity{
			UserID:   uint(uid),
			Username: username,
			IsAdmin:  c.GetHeader("X-Trim-Isadmin") == "true",
		}, true
	}
	ticket := c.GetHeader("X-FnOS-Ticket")
	if ticket == "" {
		response.ErrorUnauthorized(c, "请从飞牛桌面中的应用入口使用一键登录")
		return FnOSIdentity{}, false
	}
	identity, ok := lookupFnOSTicket(ticket)
	if !ok {
		response.ErrorUnauthorized(c, "飞牛登录凭证无效或已过期，请重新点击飞牛一键登录")
		return FnOSIdentity{}, false
	}
	return identity, true
}

func setLoginAuditContext(c *gin.Context, result map[string]interface{}) {
	user, ok := result["user"].(map[string]interface{})
	if !ok {
		return
	}
	if uid, ok := user["id"].(uint); ok {
		c.Set("userID", uid)
	}
	if username, ok := user["username"].(string); ok {
		c.Set("username", username)
	}
}

func handleFnOSIdentity(gatewayPrefix string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 飞牛授权流程的会话归飞牛登录所有：先清残留的账密会话 Cookie。
		clearSessionCookie(c, gatewayPrefix)
		identity, ok := fnOSIdentity(c)
		if !ok {
			return
		}
		response.Success(c, map[string]interface{}{
			"fnos_username": identity.Username,
		})
	}
}

func handleFnOSLogin(db *gorm.DB, gatewayPrefix string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 飞牛授权流程的会话归本次登录所有：先清掉可能残留的账密会话
		// Cookie，成功后按新令牌重种。
		clearSessionCookie(c, gatewayPrefix)
		identity, ok := fnOSIdentity(c)
		if !ok {
			return
		}
		result, err := LoginWithFnOS(db, identity, getJWTSecret(db))
		if errors.Is(err, ErrFnOSNotBound) {
			var accountCount int64
			if countErr := db.Model(&database.User{}).Count(&accountCount).Error; countErr != nil {
				response.ErrorInternal(c, "检查应用账号状态失败")
				return
			}
			var matchingAccounts int64
			if countErr := db.Model(&database.User{}).Where("username = ?", identity.Username).Count(&matchingAccounts).Error; countErr != nil {
				response.ErrorInternal(c, "检查飞牛账号绑定状态失败")
				return
			}
			suggestedMode := "register"
			if accountCount > 0 {
				suggestedMode = "bind"
			}
			suggestedUsername := ""
			if matchingAccounts > 0 {
				suggestedUsername = identity.Username
			}
			// 同名冲突（飞牛用户名已被应用账号占用）才会走到这里：给出一个
			// 空闲用户名，供「跳过绑定、创建纯飞牛账号直接登录」使用。
			autoUsername := DeriveFreeFnOSUsername(db, identity.Username)
			response.Success(c, map[string]interface{}{
				"binding_required":   true,
				"fnos_username":      identity.Username,
				"has_accounts":       accountCount > 0,
				"suggested_mode":     suggestedMode,
				"suggested_username": suggestedUsername,
				"auto_username":      autoUsername,
			})
			return
		}
		if err != nil {
			response.Error(c, http.StatusUnauthorized, response.CodeInvalidCredentials, "飞牛一键登录失败")
			return
		}
		setLoginAuditContext(c, result)
		audit.Log(db, c, "fnos_login", "user", c.GetUint("userID"), "飞牛 NAS 一键登录")
		consumeFnOSTicket(c.GetHeader("X-FnOS-Ticket"))
		issueSessionCookie(c, db, gatewayPrefix, result)
		response.Success(c, result)
	}
}

func handleFnOSBind(db *gorm.DB, gatewayPrefix string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 飞牛授权流程的会话归本次绑定所有：先清残留 Cookie，成功后按新令牌重种。
		clearSessionCookie(c, gatewayPrefix)
		identity, ok := fnOSIdentity(c)
		if !ok {
			return
		}
		var req struct {
			Mode string `json:"mode" binding:"required"`
			// Password 允许为空：空密码 = 创建纯飞牛账号（不设密码，只能飞牛授权登录）。
			Username string `json:"username" binding:"required"`
			Password string `json:"password"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			response.ErrorBadRequest(c, "请输入应用账号和密码")
			return
		}
		result, err := BindFnOSAccount(db, identity, req.Username, req.Password, req.Mode, getJWTSecret(db))
		if err != nil {
			switch {
			case errors.Is(err, ErrRegisterDisabled):
				response.Error(c, http.StatusForbidden, response.CodeRegisterDisabled, err.Error())
			case errors.Is(err, ErrUserExists), errors.Is(err, ErrFnOSAlreadyBound):
				response.Error(c, http.StatusConflict, response.CodeUserExists, err.Error())
			case errors.Is(err, ErrPasswordTooShort), errors.Is(err, ErrPasswordTooLong):
				response.ErrorBadRequest(c, err.Error())
			default:
				response.ErrorBadRequest(c, err.Error())
			}
			return
		}
		setLoginAuditContext(c, result)
		audit.Log(db, c, "fnos_bind", "user", c.GetUint("userID"), "绑定飞牛 NAS 账号")
		consumeFnOSTicket(c.GetHeader("X-FnOS-Ticket"))
		issueSessionCookie(c, db, gatewayPrefix, result)
		response.Success(c, result)
	}
}

// handleFnOSBindSelf 「后期绑定」：把当前登录的应用账号绑定到当前飞牛 NAS
// 账号（个人资料里的绑定入口）。身份仍走显式正门（网关 socket 或一次性票据），
// 会话归当前应用账号所有，不动会话 Cookie。
func handleFnOSBindSelf(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		identity, ok := fnOSIdentity(c)
		if !ok {
			return
		}
		userID := c.GetUint("userID")
		result, err := BindFnOSAccountSelf(db, identity, userID)
		if err != nil {
			switch {
			case errors.Is(err, ErrFnOSAlreadyBound):
				response.Error(c, http.StatusConflict, response.CodeUserExists, err.Error())
			case errors.Is(err, ErrUserNotFound):
				response.Error(c, http.StatusNotFound, response.CodeNotFound, err.Error())
			default:
				response.ErrorBadRequest(c, err.Error())
			}
			return
		}
		audit.Log(db, c, "fnos_bind_self", "user", userID, "绑定飞牛 NAS 账号")
		consumeFnOSTicket(c.GetHeader("X-FnOS-Ticket"))
		response.Success(c, result)
	}
}

func handleCheckAuth(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		requireLogin, _ := sysconfig.GetConfig(db, "require_login", 0)

		userID, _ := c.Get("userID")
		uid, _ := userID.(uint)

		result := CheckAuth(db, uid, requireLogin)
		response.Success(c, result)
	}
}

func handleGetCurrentUser(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, _ := c.Get("userID")
		uid, _ := userID.(uint)

		result, err := GetCurrentUser(db, uid)
		if err != nil {
			response.ErrorInternal(c, "获取用户信息失败")
			return
		}

		response.Success(c, result)
	}
}

func handleChangePassword(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			// OldPassword 允许为空：纯飞牛账号（未设密码）首次设置密码无需旧密码，
			// 是否校验旧密码由 service 按账号是否有密码决定。
			OldPassword    string `json:"old_password"`
			OldPasswordMd5 string `json:"old_password_md5"`
			NewPassword    string `json:"new_password" binding:"required"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			response.ErrorBadRequest(c, "请输入新密码")
			return
		}

		userID, _ := c.Get("userID")
		uid, _ := userID.(uint)

		if err := ChangePassword(db, uid, req.OldPassword, req.OldPasswordMd5, req.NewPassword); err != nil {
			response.ErrorBadRequest(c, err.Error())
			return
		}

		response.Success(c, nil)
	}
}

func handleRegenerateAPIKey(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, _ := c.Get("userID")
		uid, _ := userID.(uint)

		newKey, err := RegenerateAPIKey(db, uid)
		if err != nil {
			response.ErrorInternal(c, "重新生成 API Key 失败")
			return
		}

		response.Success(c, map[string]string{"api_key": newKey})
	}
}

func handleGetAllUsers(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		page := utils.Atoi(c.Query("page"), 1)
		pageSize := utils.Atoi(c.Query("pageSize"), utils.DefaultPageSize)
		search := c.Query("search")

		users, total, err := GetAllUsers(db, page, pageSize, search)
		if err != nil {
			response.ErrorInternal(c, "获取用户列表失败")
			return
		}

		page, pageSize = utils.NormalizePage(page, pageSize)
		response.SuccessPage(c, users, total, page, pageSize)
	}
}

func handleRenameUser(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, ok := parseUserID(c)
		if !ok {
			return
		}
		var req struct {
			Username string `json:"username" binding:"required"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			response.ErrorBadRequest(c, "请输入新用户名")
			return
		}
		if err := RenameUser(db, userID, req.Username); err != nil {
			switch {
			case errors.Is(err, ErrSuperAdminProtected):
				response.Error(c, http.StatusForbidden, response.CodeForbidden, err.Error())
			case errors.Is(err, ErrUserNotFound):
				response.Error(c, http.StatusNotFound, response.CodeNotFound, err.Error())
			case errors.Is(err, ErrUserExists):
				response.Error(c, http.StatusConflict, response.CodeUserExists, err.Error())
			default:
				response.ErrorBadRequest(c, err.Error())
			}
			return
		}
		audit.Log(db, c, "user_rename", "user", userID, "管理员改用户名："+req.Username)
		response.Success(c, nil)
	}
}

func handleCreateUser(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			Username string `json:"username" binding:"required"`
			Password string `json:"password" binding:"required"`
			Role     string `json:"role"`
			Status   *int   `json:"status"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			response.ErrorBadRequest(c, "请输入用户名与密码")
			return
		}
		role := req.Role
		if role == "" {
			role = "user"
		}
		// 管理员新建的账号默认直接可用；传 status=2 表示先放进待审核队列。
		status := 1
		if req.Status != nil {
			status = *req.Status
		}
		u, err := CreateUser(db, req.Username, req.Password, role, status)
		if err != nil {
			switch {
			case errors.Is(err, ErrUserExists):
				response.Error(c, http.StatusConflict, response.CodeUserExists, err.Error())
			case errors.Is(err, ErrPasswordTooShort), errors.Is(err, ErrPasswordTooLong):
				response.ErrorBadRequest(c, err.Error())
			default:
				response.ErrorBadRequest(c, err.Error())
			}
			return
		}
		audit.Log(db, c, "user_create", "user", u.ID, "管理员创建用户："+u.Username)
		response.Success(c, map[string]interface{}{"id": u.ID, "username": u.Username, "role": u.Role, "status": u.Status})
	}
}

func handleUpdateUser(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			Role   *string `json:"role"`
			Status *int    `json:"status"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			response.ErrorBadRequest(c, "请求无效")
			return
		}

		userID, ok := parseUserID(c)
		if !ok {
			return
		}

		if err := UpdateUser(db, userID, req.Role, req.Status); err != nil {
			if errors.Is(err, ErrSuperAdminProtected) {
				response.Error(c, http.StatusForbidden, response.CodeForbidden, err.Error())
				return
			}
			if errors.Is(err, ErrUserNotFound) {
				response.Error(c, http.StatusNotFound, response.CodeNotFound, err.Error())
			} else {
				response.ErrorBadRequest(c, err.Error())
			}
			return
		}

		audit.Log(db, c, "user_update", "user", userID, "管理员修改用户信息")
		response.Success(c, nil)
	}
}

func handleDeleteUser(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, ok := parseUserID(c)
		if !ok {
			return
		}
		if userID == c.GetUint("userID") {
			response.ErrorBadRequest(c, "不能删除自己的账号")
			return
		}
		if err := DeleteUser(db, userID); err != nil {
			switch {
			case errors.Is(err, ErrSuperAdminProtected):
				response.Error(c, http.StatusForbidden, response.CodeForbidden, err.Error())
			case errors.Is(err, ErrUserNotFound):
				response.Error(c, http.StatusNotFound, response.CodeNotFound, err.Error())
			case errors.Is(err, ErrLastAdmin), errors.Is(err, ErrUserDeleteBlocked):
				response.ErrorBadRequest(c, err.Error())
			default:
				response.ErrorInternal(c, "删除用户失败")
			}
			return
		}

		audit.Log(db, c, "user_delete", "user", userID, "管理员删除用户")
		response.Success(c, nil)
	}
}

func handleToggleStatus(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, ok := parseUserID(c)
		if !ok {
			return
		}

		if err := ToggleUserStatus(db, userID); err != nil {
			switch {
			case errors.Is(err, ErrSuperAdminProtected):
				response.Error(c, http.StatusForbidden, response.CodeForbidden, err.Error())
			case errors.Is(err, ErrUserNotFound):
				response.Error(c, http.StatusNotFound, response.CodeNotFound, err.Error())
			case errors.Is(err, ErrLastAdmin):
				response.ErrorBadRequest(c, err.Error())
			default:
				response.ErrorInternal(c, "切换用户状态失败")
			}
			return
		}

		audit.Log(db, c, "user_status", "user", userID, "管理员切换用户状态")
		response.Success(c, nil)
	}
}

func handleResetPassword(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			NewPassword string `json:"new_password" binding:"required"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			response.ErrorBadRequest(c, "请输入新密码")
			return
		}

		userID, ok := parseUserID(c)
		if !ok {
			return
		}
		if err := ResetPassword(db, userID, req.NewPassword); err != nil {
			if errors.Is(err, ErrSuperAdminProtected) {
				response.Error(c, http.StatusForbidden, response.CodeForbidden, err.Error())
			} else if errors.Is(err, ErrPasswordTooShort) || errors.Is(err, ErrPasswordTooLong) {
				response.ErrorBadRequest(c, err.Error())
			} else if errors.Is(err, ErrUserNotFound) {
				response.Error(c, http.StatusNotFound, response.CodeNotFound, err.Error())
			} else {
				response.ErrorInternal(c, "重置密码失败")
			}
			return
		}

		audit.Log(db, c, "password_reset", "user", userID, "管理员重置用户密码")
		response.Success(c, nil)
	}
}

// handleLogout 退出登录：飞牛登录只是身份来源，应用会话必须能独立结束。
// 会话 Cookie 不分端口（同主机名共享），所以无论请求来自网关还是直连端口
// 都要清，防止网关域上的会话残留把人认回去。
func handleLogout(db *gorm.DB, gatewayPrefix string) gin.HandlerFunc {
	return func(c *gin.Context) {
		clearSessionCookie(c, gatewayPrefix)
		audit.Log(db, c, "logout", "user", c.GetUint("userID"), "退出登录")
		response.Success(c, map[string]interface{}{"ok": true})
	}
}

// RegisterRoutes wires user and authentication routes.
func RegisterRoutes(publicGroup *gin.RouterGroup, optionalAuthGroup *gin.RouterGroup, authGroup *gin.RouterGroup, adminGroup *gin.RouterGroup, db *gorm.DB, fnOSApp bool, gatewayPrefix string) {
	publicGroup.GET("/auth/setup-required", handleSetupRequired(db))
	publicGroup.POST("/auth/register", handleRegister(db, gatewayPrefix))
	publicGroup.POST("/auth/login", handleLogin(db, gatewayPrefix))
	if fnOSApp {
		publicGroup.POST("/auth/fnos/ticket", handleFnOSTicket())
		// identity 同时挂 GET 与 POST：只读当前网关身份，两个方法都幂等，
		// 防止前端方法不匹配落进 404 兜底返回 SPA 网页。
		publicGroup.GET("/auth/fnos/identity", handleFnOSIdentity(gatewayPrefix))
		publicGroup.POST("/auth/fnos/identity", handleFnOSIdentity(gatewayPrefix))
		publicGroup.POST("/auth/fnos/login", handleFnOSLogin(db, gatewayPrefix))
		publicGroup.POST("/auth/fnos/bind", handleFnOSBind(db, gatewayPrefix))
		// 后期绑定：把当前登录的应用账号绑到当前飞牛身份，需要应用会话
		// （网关域走会话 Cookie，直连端口走 JWT），所以挂在 auth 组。
		authGroup.POST("/auth/fnos/bind-self", handleFnOSBindSelf(db))
	}

	optionalAuthGroup.GET("/auth/check", handleCheckAuth(db))

	authGroup.GET("/auth/me", handleGetCurrentUser(db))
	authGroup.PUT("/auth/password", handleChangePassword(db))
	authGroup.POST("/auth/apikey", handleRegenerateAPIKey(db))
	authGroup.POST("/auth/logout", handleLogout(db, gatewayPrefix))

	adminGroup.GET("/users", handleGetAllUsers(db))
	adminGroup.POST("/users", handleCreateUser(db))
	adminGroup.PUT("/users/:id/username", handleRenameUser(db))
	adminGroup.PUT("/users/:id", handleUpdateUser(db))
	adminGroup.DELETE("/users/:id", handleDeleteUser(db))
	adminGroup.PUT("/users/:id/status", handleToggleStatus(db))
	adminGroup.PUT("/users/:id/password", handleResetPassword(db))
}
