package user

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"smallgo/server/auth"
	"smallgo/server/database"
	"smallgo/server/sysconfig"
	"smallgo/server/utils"

	"gorm.io/gorm"
)

// Sentinel errors — handlers map these to the documented API error codes
// (1001 / 1003 in AGENTS.md). The error message IS the user-facing text.
var (
	ErrRegisterDisabled = errors.New("注册功能已关闭")
	ErrUserExists       = errors.New("用户名已存在")
	// ErrPendingApproval 注册需审核时，账号已建但未通过，登录要给出明确原因。
	ErrPendingApproval  = errors.New("账号正在等待管理员审核，请联系管理员通过审核")
	// ErrSuperAdminProtected 首次注册的管理员是超级管理员，任何账号都不能改它的设置。
	ErrSuperAdminProtected = errors.New("超级管理员是首次注册的账号，任何账号都不能修改其设置；忘记密码请用服务器命令行重置")
	ErrPasswordTooShort = errors.New("密码长度不能少于6位")
	ErrPasswordTooLong  = errors.New("密码长度不能超过72字节")
	ErrLastAdmin        = errors.New("不能降级、禁用或删除最后一个管理员")
	ErrUserNotFound     = errors.New("用户不存在")
	// ErrUserDeleteBlocked is returned when a registered app guard wants to
	// keep the user (its business data still references the account).
	ErrUserDeleteBlocked = errors.New("该用户在应用中已有数据，不能删除；可改为禁用账号")
	ErrFnOSNotBound      = errors.New("此飞牛 NAS 账号尚未绑定应用账号")
	ErrFnOSAlreadyBound = errors.New("此飞牛 NAS 账号已绑定其他应用账号")
)

// FnOSIdentity is the trusted identity context added by the fnOS unified
// gateway. It deliberately has no token field: gateway API tokens must never
// be persisted or passed to the browser.
type FnOSIdentity struct {
	UserID   uint
	Username string
	IsAdmin  bool
}

func validatePassword(password string) error {
	if utf8.RuneCountInString(password) < 6 {
		return ErrPasswordTooShort
	}
	if len(password) > 72 {
		return ErrPasswordTooLong
	}
	return nil
}

func sha256Hex(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

// verifyStoredPassword accepts both the current plaintext-over-TLS protocol
// and the previous browser-side SHA-256 protocol. A successful legacy match is
// migrated to bcrypt(raw) by callers that can safely update the row.
func verifyStoredPassword(stored, raw, legacyMD5 string) (bool, bool) {
	if auth.VerifyPassword(stored, raw) {
		return true, auth.IsLegacyHash(stored)
	}
	for _, candidate := range []string{sha256Hex(raw), legacyMD5} {
		if candidate != "" && candidate != raw && auth.VerifyPassword(stored, candidate) {
			return true, true
		}
	}
	return false, false
}

// loginTokenTTL resolves the configured login lifetime. The login_expiry_days
// sysconfig (1-365 days, default 7) lets the admin control how often users
// must re-authenticate.
func loginTokenTTL(db *gorm.DB) time.Duration {
	days := 7
	if value, err := sysconfig.GetConfig(db, "login_expiry_days", 0); err == nil {
		if parsed, parseErr := strconv.Atoi(value); parseErr == nil && parsed >= 1 && parsed <= 365 {
			days = parsed
		}
	}
	return time.Duration(days) * 24 * time.Hour
}

func Register(db *gorm.DB, username string, password string, jwtSecret string) (map[string]interface{}, error) {
	var result map[string]interface{}
	err := db.Transaction(func(tx *gorm.DB) error {
		var userCount int64
		if err := tx.Model(&database.User{}).Count(&userCount).Error; err != nil {
			return err
		}

		if userCount > 0 {
			allowRegister, err := sysconfig.GetConfig(tx, "allow_register", 0)
			if err != nil {
				return err
			}
			if allowRegister != "true" {
				return ErrRegisterDisabled
			}
		}

		var existing database.User
		if err := tx.Where("username = ?", username).First(&existing).Error; err == nil {
			return ErrUserExists
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		if err := validatePassword(password); err != nil {
			return err
		}
		hashedPassword, err := auth.HashPassword(password)
		if err != nil {
			return err
		}

		role := "user"
		if userCount == 0 {
			role = "admin"
		}

		// 注册审核开关：开启后普通账号先落成「待审核」状态（2），首个管理员始终直接可用。
		status := 1
		pendingApproval := false
		if role != "admin" {
			if v, err := sysconfig.GetConfig(tx, registerApprovalKey, 0); err == nil && v == "true" {
				status = 2
				pendingApproval = true
			}
		}

		user := database.User{
			Username:    username,
			Password:    hashedPassword,
			Role:        role,
			Status:      status,
			APIKey:      auth.GenerateAPIKey(),
			AuthVersion: 1,
		}
		if err := tx.Create(&user).Error; err != nil {
			return err
		}

		token, err := auth.GenerateTokenWithTTL(user.ID, user.Username, user.Role, user.AuthVersion, jwtSecret, loginTokenTTL(tx))
		if err != nil {
			return err
		}

		result = map[string]interface{}{
			"token": token,
			"user": map[string]interface{}{
				"id":       user.ID,
				"username": user.Username,
				"role":     user.Role,
			},
			"pending_approval": pendingApproval,
		}
		return nil
	})
	return result, err
}

func Login(db *gorm.DB, username string, password string, passwordMd5 string, jwtSecret string) (map[string]interface{}, error) {
	var user database.User
	if err := db.Where("username = ?", username).First(&user).Error; err != nil {
		return nil, fmt.Errorf("用户名或密码错误")
	}

	if user.Status == 2 {
		// 待审核是注册流程的正常中间态，明确告知比"密码错误"更省事，也不泄露他人账号信息。
		return nil, ErrPendingApproval
	}
	if user.Status != 1 {
		// Generic error — masking disabled accounts as "wrong credentials"
		// prevents username enumeration via response inspection.
		return nil, fmt.Errorf("用户名或密码错误")
	}

	if user.Password == "" {
		// 纯飞牛账号（未设置密码）：只能走飞牛授权登录。
		return nil, fmt.Errorf("该账号为纯飞牛登录账号，请使用飞牛 NAS 登录")
	}

	verified, needsRehash := verifyStoredPassword(user.Password, password, passwordMd5)
	if !verified {
		return nil, fmt.Errorf("用户名或密码错误")
	}
	if user.AuthVersion == 0 {
		user.AuthVersion = 1
		if err := db.Model(&user).Update("auth_version", user.AuthVersion).Error; err != nil {
			return nil, err
		}
	}
	if needsRehash {
		hashedPassword, err := auth.HashPassword(password)
		if err != nil {
			return nil, err
		}
		if err := db.Model(&user).Update("password", hashedPassword).Error; err != nil {
			return nil, err
		}
	}

	token, err := auth.GenerateTokenWithTTL(user.ID, user.Username, user.Role, user.AuthVersion, jwtSecret, loginTokenTTL(db))
	if err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"token": token,
		"user": map[string]interface{}{
			"id":       user.ID,
			"username": user.Username,
			"role":     user.Role,
		},
	}, nil
}

// LoginWithFnOS signs in the application account bound to the gateway's
// NAS-local UID. The identity must only come from the fnOS Unix-socket gateway.
//
// 未绑定时自动开通同名的「纯飞牛账号」（无密码）并直接登录：显式的飞牛授权
// 登录本身就是用户的选择，绑定或创建带密码的应用账号降级为可选项
// （2026-10-11 用户要求：不强求、不必须）。同名应用账号已存在时不自动开通，
// 交由 handler 返回绑定/改名建议，由用户决定。
func LoginWithFnOS(db *gorm.DB, identity FnOSIdentity, jwtSecret string) (map[string]interface{}, error) {
	var user database.User
	if err := db.Where("fn_os_user_id = ?", identity.UserID).First(&user).Error; err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
		return provisionFnOSAccount(db, identity, jwtSecret)
	}
	if user.Status != 1 {
		return nil, fmt.Errorf("账号已停用")
	}
	if user.AuthVersion == 0 {
		user.AuthVersion = 1
		if err := db.Model(&user).Update("auth_version", user.AuthVersion).Error; err != nil {
			return nil, err
		}
	}
	if user.FnOSUsername != identity.Username {
		if err := db.Model(&user).Update("fn_os_username", identity.Username).Error; err != nil {
			return nil, err
		}
	}
	return loginResult(db, user, jwtSecret)
}

// provisionFnOSAccount 为未绑定的飞牛身份自动开通同名纯飞牛账号（无密码，
// 密码登录不可用，走飞牛授权即用）并登录。用户名被占用时返回 ErrFnOSNotBound，
// 由登录页给出可选的绑定/改名路径。
func provisionFnOSAccount(db *gorm.DB, identity FnOSIdentity, jwtSecret string) (map[string]interface{}, error) {
	var sameName int64
	if err := db.Model(&database.User{}).Where("username = ?", identity.Username).Count(&sameName).Error; err != nil {
		return nil, err
	}
	if sameName > 0 {
		return nil, ErrFnOSNotBound
	}

	var (
		result map[string]interface{}
		rerr   error
	)
	rerr = db.Transaction(func(tx *gorm.DB) error {
		var userCount int64
		if err := tx.Model(&database.User{}).Count(&userCount).Error; err != nil {
			return err
		}
		role := "user"
		if userCount == 0 {
			role = "admin"
		}
		user := database.User{
			Username:    identity.Username,
			Password:    "",
			Role:        role,
			Status:      1,
			APIKey:      auth.GenerateAPIKey(),
			AuthVersion: 1,
			FnOSUserID:  &identity.UserID,
			FnOSUsername: identity.Username,
		}
		if err := tx.Create(&user).Error; err != nil {
			return err
		}
		result, rerr = loginResult(tx, user, jwtSecret)
		if rerr != nil {
			return rerr
		}
		result["provisioned"] = true
		return nil
	})
	if rerr != nil {
		return nil, rerr
	}
	return result, nil
}

// DeriveFreeFnOSUsername 为同名冲突时的「跳过绑定」路径派生一个空闲用户名：
// 原名 → 原名_fnos → 原名_fnos2 …
func DeriveFreeFnOSUsername(db *gorm.DB, base string) string {
	candidate := base
	for i := 1; ; i++ {
		var count int64
		if err := db.Model(&database.User{}).Where("username = ?", candidate).Count(&count).Error; err != nil || count == 0 {
			return candidate
		}
		if i == 1 {
			candidate = base + "_fnos"
		} else {
			candidate = base + "_fnos" + strconv.Itoa(i)
		}
	}
}

// BindFnOSAccount either creates a new application account or verifies an
// existing account, then atomically binds it to the current fnOS UID.
func BindFnOSAccount(db *gorm.DB, identity FnOSIdentity, username, password, mode, jwtSecret string) (map[string]interface{}, error) {
	if identity.UserID == 0 || identity.Username == "" {
		return nil, fmt.Errorf("飞牛登录信息无效")
	}

	var result map[string]interface{}
	err := db.Transaction(func(tx *gorm.DB) error {
		var linked database.User
		if err := tx.Where("fn_os_user_id = ?", identity.UserID).First(&linked).Error; err == nil {
			return ErrFnOSAlreadyBound
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		var user database.User
		switch mode {
		case "register":
			var userCount int64
			if err := tx.Model(&database.User{}).Count(&userCount).Error; err != nil {
				return err
			}
			if userCount > 0 {
				allowRegister, err := sysconfig.GetConfig(tx, "allow_register", 0)
				if err != nil {
					return err
				}
				if allowRegister != "true" {
					return ErrRegisterDisabled
				}
			}
			// password 为空 = 创建纯飞牛账号（密码登录不可用，走飞牛授权即用），
			// 供「跳过绑定、不设密码直接登录」路径使用。
			hashedPassword := ""
			if password != "" {
				if err := validatePassword(password); err != nil {
					return err
				}
				hash, err := auth.HashPassword(password)
				if err != nil {
					return err
				}
				hashedPassword = hash
			}
			if err := tx.Where("username = ?", username).First(&user).Error; err == nil {
				return ErrUserExists
			} else if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			role := "user"
			if userCount == 0 {
				role = "admin"
			}
			user = database.User{Username: username, Password: hashedPassword, Role: role, Status: 1, APIKey: auth.GenerateAPIKey(), AuthVersion: 1}
			if err := tx.Create(&user).Error; err != nil {
				return err
			}
		case "bind":
			if err := tx.Where("username = ?", username).First(&user).Error; err != nil || user.Status != 1 {
				return fmt.Errorf("用户名或密码错误")
			}
			verified, needsRehash := verifyStoredPassword(user.Password, password, "")
			if !verified {
				return fmt.Errorf("用户名或密码错误")
			}
			if needsRehash {
				hashedPassword, err := auth.HashPassword(password)
				if err != nil {
					return err
				}
				if err := tx.Model(&user).Update("password", hashedPassword).Error; err != nil {
					return err
				}
			}
		default:
			return fmt.Errorf("无效的绑定方式")
		}

		if user.FnOSUserID != nil && *user.FnOSUserID != identity.UserID {
			return fmt.Errorf("该应用账号已绑定其他飞牛 NAS 账号")
		}
		updates := map[string]interface{}{
			"fn_os_user_id":  identity.UserID,
			"fn_os_username": identity.Username,
		}
		if user.AuthVersion == 0 {
			user.AuthVersion = 1
			updates["auth_version"] = user.AuthVersion
		}
		if err := tx.Model(&user).Updates(updates).Error; err != nil {
			return err
		}
		user.FnOSUserID = &identity.UserID
		user.FnOSUsername = identity.Username
		var err error
		result, err = loginResult(tx, user, jwtSecret)
		return err
	})
	return result, err
}

func loginResult(db *gorm.DB, user database.User, jwtSecret string) (map[string]interface{}, error) {
	token, err := auth.GenerateTokenWithTTL(user.ID, user.Username, user.Role, user.AuthVersion, jwtSecret, loginTokenTTL(db))
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"token": token,
		"user":  map[string]interface{}{"id": user.ID, "username": user.Username, "role": user.Role},
	}, nil
}

func ChangePassword(db *gorm.DB, userID uint, oldPassword string, oldPasswordMd5 string, newPassword string) error {
	if err := validatePassword(newPassword); err != nil {
		return err
	}

	var user database.User
	if err := db.First(&user, userID).Error; err != nil {
		return fmt.Errorf("用户不存在")
	}

	// 本接口只对已登录会话开放（authGroup），会话本身就是身份凭证，
	// 因此改自己的密码不再强制填写旧密码；传了旧密码仍然校验（兼容其它应用的调用方）。
	// 纯飞牛账号（Password 为空）首次设置密码同样无需旧密码。
	if user.Password != "" && (strings.TrimSpace(oldPassword) != "" || strings.TrimSpace(oldPasswordMd5) != "") {
		verified, _ := verifyStoredPassword(user.Password, oldPassword, oldPasswordMd5)
		if !verified {
			return fmt.Errorf("原密码不正确")
		}
	}
	hashedPassword, err := auth.HashPassword(newPassword)
	if err != nil {
		return err
	}
	nextVersion := user.AuthVersion + 1
	return db.Model(&user).Updates(map[string]interface{}{
		"password":     hashedPassword,
		"auth_version": nextVersion,
	}).Error
}

// UserResponse is the admin user-list payload. APIKey is intentionally
// omitted — bearer tokens must not be enumerable via the list endpoint.
// Owners read their own key via GET /api/auth/me.
type UserResponse struct {
	ID           uint   `json:"id"`
	Username     string `json:"username"`
	Role         string `json:"role"`
	Status       int    `json:"status"`
	IsSuperAdmin bool   `json:"is_super_admin"`
	CreatedAt    string `json:"created_at"`
}

// superAdminID 返回超级管理员的用户 ID：所有管理员里 id 最小的那个（即首次注册的账号）。
// 没有管理员时返回 0。超级管理员的设置任何账号都不能改，只能靠命令行重置密码兜底。
func superAdminID(db *gorm.DB) uint {
	var u database.User
	if err := db.Where("role = ?", "admin").Order("id ASC").First(&u).Error; err != nil {
		return 0
	}
	return u.ID
}

// guardSuperAdmin 拦下对超级管理员的任何修改。
func guardSuperAdmin(db *gorm.DB, userID uint) error {
	if superAdminID(db) == userID {
		return ErrSuperAdminProtected
	}
	return nil
}

func GetAllUsers(db *gorm.DB, page int, pageSize int, search string) ([]UserResponse, int64, error) {
	page, pageSize = utils.NormalizePage(page, pageSize)

	query := db.Model(&database.User{})
	if search != "" {
		query = query.Where("username LIKE ?", "%"+search+"%")
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var users []database.User
	if err := query.Order("id ASC").Offset(utils.Offset(page, pageSize)).Limit(pageSize).Find(&users).Error; err != nil {
		return nil, 0, err
	}

	superID := superAdminID(db)
	userResponses := make([]UserResponse, 0, len(users))
	for _, u := range users {
		userResponses = append(userResponses, UserResponse{
			ID:           u.ID,
			Username:     u.Username,
			Role:         u.Role,
			Status:       u.Status,
			IsSuperAdmin: u.ID == superID,
			CreatedAt:    u.CreatedAt.Format("2006-01-02 15:04:05"),
		})
	}

	return userResponses, total, nil
}

// UpdateUser updates only the fields an admin is allowed to change (role and
// status). Sensitive columns (password, api_key, username) are intentionally
// not updatable here to avoid mass-assignment; use the dedicated endpoints.
// registerApprovalKey 是「普通用户注册需审核」的配置键，与 medcare 包注册的定义保持一致。
const registerApprovalKey = "medcare_register_approval"

// CreateUser 由管理员直接创建账号：可指定角色与初始状态（1 正常 / 2 待审核 / 0 禁用）。
func CreateUser(db *gorm.DB, username, password, role string, status int) (database.User, error) {
	var zero database.User
	name := strings.TrimSpace(username)
	if name == "" || utf8.RuneCountInString(name) > 32 {
		return zero, fmt.Errorf("用户名应为 1–32 个字符")
	}
	if role != "admin" && role != "user" {
		return zero, fmt.Errorf("无效的角色")
	}
	if status != 0 && status != 1 && status != 2 {
		return zero, fmt.Errorf("无效的状态")
	}
	if err := validatePassword(password); err != nil {
		return zero, err
	}
	var dup int64
	if err := db.Model(&database.User{}).Where("username = ?", name).Count(&dup).Error; err != nil {
		return zero, err
	}
	if dup > 0 {
		return zero, ErrUserExists
	}
	hashed, err := auth.HashPassword(password)
	if err != nil {
		return zero, err
	}
	u := database.User{Username: name, Password: hashed, Role: role, Status: status, APIKey: auth.GenerateAPIKey(), AuthVersion: 1}
	if err := db.Create(&u).Error; err != nil {
		return zero, err
	}
	return u, nil
}

// RenameUser 单独提供改名能力：不复用 UpdateUser 的批量字段更新，
// 避免把「管理员接口可批量赋值」的口子重新打开（见 MassAssignment 防护用例）。
func RenameUser(db *gorm.DB, userID uint, username string) error {
	if err := guardSuperAdmin(db, userID); err != nil {
		return err
	}
	name := strings.TrimSpace(username)
	if name == "" || utf8.RuneCountInString(name) > 32 {
		return fmt.Errorf("用户名应为 1–32 个字符")
	}
	var dup int64
	if err := db.Model(&database.User{}).Where("username = ? AND id <> ?", name, userID).Count(&dup).Error; err != nil {
		return err
	}
	if dup > 0 {
		return ErrUserExists
	}
	var target database.User
	if err := db.First(&target, userID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrUserNotFound
		}
		return err
	}
	if err := db.Model(&target).Update("username", name).Error; err != nil {
		return err
	}
	// 用户名变了要让已签发的令牌失效，避免旧令牌里带着旧名继续用。
	return db.Model(&target).UpdateColumn("auth_version", gorm.Expr("auth_version + 1")).Error
}

func UpdateUser(db *gorm.DB, userID uint, role *string, status *int) error {
	updates := map[string]interface{}{}
	if role != nil {
		if *role != "admin" && *role != "user" {
			return fmt.Errorf("无效的角色")
		}
		updates["role"] = *role
	}
	if status != nil {
		if *status != 0 && *status != 1 && *status != 2 {
			return fmt.Errorf("无效的状态")
		}
		updates["status"] = *status
	}
	if len(updates) == 0 {
		return fmt.Errorf("请提供要更新的角色或状态")
	}
	return db.Transaction(func(tx *gorm.DB) error {
		var target database.User
		if err := tx.First(&target, userID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrUserNotFound
			}
			return err
		}
		if err := ensureAdminRemains(tx, target, role, status); err != nil {
			return err
		}
		// 唯一的最后一个管理员已在上面按 400 挡住；这里拦「还有其他管理员时去改超级管理员」。
		if err := guardSuperAdmin(tx, userID); err != nil {
			return err
		}
		return tx.Model(&target).Updates(updates).Error
	})
}

func ensureAdminRemains(db *gorm.DB, target database.User, newRole *string, newStatus *int) error {
	wasActiveAdmin := target.Role == "admin" && target.Status == 1
	if !wasActiveAdmin {
		return nil
	}
	stillAdmin := newRole == nil || *newRole == "admin"
	stillActive := newStatus == nil || *newStatus == 1
	if stillAdmin && stillActive {
		return nil
	}
	var count int64
	if err := db.Model(&database.User{}).Where("role = ? AND status = ?", "admin", 1).Count(&count).Error; err != nil {
		return err
	}
	if count <= 1 {
		return ErrLastAdmin
	}
	return nil
}

func ToggleUserStatus(db *gorm.DB, userID uint) error {
	return db.Transaction(func(tx *gorm.DB) error {
		var target database.User
		if err := tx.First(&target, userID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrUserNotFound
			}
			return err
		}
		newStatus := 1
		if target.Status == 1 {
			newStatus = 0
		}
		if err := ensureAdminRemains(tx, target, nil, &newStatus); err != nil {
			return err
		}
		if err := guardSuperAdmin(tx, userID); err != nil {
			return err
		}
		return tx.Model(&target).Update("status", newStatus).Error
	})
}

func DeleteUser(db *gorm.DB, userID uint) error {
	return db.Transaction(func(tx *gorm.DB) error {
		var target database.User
		if err := tx.First(&target, userID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrUserNotFound
			}
			return err
		}
		disabled := 0
		if err := ensureAdminRemains(tx, target, nil, &disabled); err != nil {
			return err
		}
		if err := guardSuperAdmin(tx, userID); err != nil {
			return err
		}
		// App guards may retain users their own business data still references.
		if blocked, err := database.UserDeletionBlocked(tx, userID); err != nil {
			return err
		} else if blocked {
			return ErrUserDeleteBlocked
		}
		if err := tx.Where("user_id = ?", userID).Delete(&database.SecurityQuestion{}).Error; err != nil {
			return err
		}
		if err := tx.Where("user_id = ?", userID).Delete(&database.SystemConfig{}).Error; err != nil {
			return err
		}
		// Let apps drop their per-user metadata before the user row goes away.
		if err := database.CleanupUserAppData(tx, userID); err != nil {
			return err
		}
		return tx.Delete(&target).Error
	})
}

func ResetPassword(db *gorm.DB, userID uint, newPassword string) error {
	// 超级管理员忘记密码只能走服务器命令行（-reset-admin-password），管理员界面不提供重置。
	if err := guardSuperAdmin(db, userID); err != nil {
		return err
	}
	if err := validatePassword(newPassword); err != nil {
		return err
	}
	hashedPassword, err := auth.HashPassword(newPassword)
	if err != nil {
		return err
	}
	result := db.Model(&database.User{}).Where("id = ?", userID).Updates(map[string]interface{}{
		"password":     hashedPassword,
		"auth_version": gorm.Expr("auth_version + 1"),
	})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrUserNotFound
	}
	return nil
}

func ResetAdminPassword(db *gorm.DB, newPassword string) error {
	var user database.User
	if err := db.Where("role = ?", "admin").Order("id ASC").First(&user).Error; err != nil {
		return fmt.Errorf("未找到管理员账号")
	}

	// 没指定就随机生成，避免所有人共用 admin123 这类弱口令。
	if newPassword == "" {
		buf := make([]byte, 9)
		if _, err := rand.Read(buf); err != nil {
			return err
		}
		newPassword = "mc-" + hex.EncodeToString(buf)
	}
	if err := validatePassword(newPassword); err != nil {
		return err
	}
	hashedPassword, err := auth.HashPassword(newPassword)
	if err != nil {
		return err
	}
	if err := db.Model(&user).Updates(map[string]interface{}{
		"password":     hashedPassword,
		"auth_version": gorm.Expr("auth_version + 1"),
	}).Error; err != nil {
		return err
	}

	fmt.Printf("管理员 %s 的密码已重置为：%s\n", user.Username, newPassword)
	return nil
}

func CheckAuth(db *gorm.DB, userID uint, requireLogin string) map[string]interface{} {
	result := map[string]interface{}{
		"authenticated": false,
		"require_login": requireLogin,
		"user":          nil,
	}

	if userID > 0 {
		var user database.User
		if err := db.First(&user, userID).Error; err == nil && user.Status == 1 {
			result["authenticated"] = true
			result["user"] = map[string]interface{}{
				"id":       user.ID,
				"username": user.Username,
				"role":     user.Role,
			}
		}
	}

	return result
}

func GetCurrentUser(db *gorm.DB, userID uint) (map[string]interface{}, error) {
	var user database.User
	if err := db.First(&user, userID).Error; err != nil {
		return nil, err
	}

	var sq database.SecurityQuestion
	hasQuestions := db.Where("user_id = ?", userID).First(&sq).Error == nil

	return map[string]interface{}{
		"id":                     user.ID,
		"username":               user.Username,
		"role":                   user.Role,
		"api_key":                user.APIKey,
		"has_password":           user.Password != "",
		"has_security_questions": hasQuestions,
		"fnos_bound":             user.FnOSUserID != nil,
		"fnos_username":          user.FnOSUsername,
	}, nil
}

// BindFnOSAccountSelf 把当前登录的应用账号绑定到网关身份所在的飞牛 NAS
// 账号——即「先用账号密码使用，后期再绑定」的路径。绑定关系仍只经飞牛授权
// 正门建立：身份必须来自网关 socket 或一次性票据，绝不静默读取 X-Trim-*。
// 已绑定同一飞牛账号时幂等成功（只刷新飞牛用户名），绑定冲突沿用
// ErrFnOSAlreadyBound 让 handler 映射 409。
func BindFnOSAccountSelf(db *gorm.DB, identity FnOSIdentity, userID uint) (map[string]interface{}, error) {
	if identity.UserID == 0 || identity.Username == "" {
		return nil, fmt.Errorf("飞牛登录信息无效")
	}
	var result map[string]interface{}
	err := db.Transaction(func(tx *gorm.DB) error {
		var target database.User
		if err := tx.First(&target, userID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrUserNotFound
			}
			return err
		}
		var linked database.User
		if err := tx.Where("fn_os_user_id = ?", identity.UserID).First(&linked).Error; err == nil {
			if linked.ID != target.ID {
				return ErrFnOSAlreadyBound
			}
			// 幂等：同一飞牛账号重复绑定只刷新用户名。
			if target.FnOSUsername != identity.Username {
				if err := tx.Model(&target).Update("fn_os_username", identity.Username).Error; err != nil {
					return err
				}
				target.FnOSUsername = identity.Username
			}
			result = selfBindResult(target)
			return nil
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if target.FnOSUserID != nil && *target.FnOSUserID != identity.UserID {
			return fmt.Errorf("当前账号已绑定其他飞牛 NAS 账号")
		}
		updates := map[string]interface{}{
			"fn_os_user_id":  identity.UserID,
			"fn_os_username": identity.Username,
		}
		if target.AuthVersion == 0 {
			target.AuthVersion = 1
			updates["auth_version"] = target.AuthVersion
		}
		if err := tx.Model(&target).Updates(updates).Error; err != nil {
			return err
		}
		target.FnOSUserID = &identity.UserID
		target.FnOSUsername = identity.Username
		result = selfBindResult(target)
		return nil
	})
	return result, err
}

func selfBindResult(user database.User) map[string]interface{} {
	return map[string]interface{}{
		"id":            user.ID,
		"username":      user.Username,
		"fnos_bound":    true,
		"fnos_username": user.FnOSUsername,
	}
}

func RegenerateAPIKey(db *gorm.DB, userID uint) (string, error) {
	newKey := auth.GenerateAPIKey()
	if err := db.Model(&database.User{}).Where("id = ?", userID).Update("api_key", newKey).Error; err != nil {
		return "", err
	}
	return newKey, nil
}

func getJWTSecret(db *gorm.DB) string {
	var config database.SystemConfig
	if err := db.Where("user_id = ? AND key = ?", 0, "jwt_secret").First(&config).Error; err != nil {
		return ""
	}
	return config.Value
}
