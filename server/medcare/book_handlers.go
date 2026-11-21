package medcare

import (
    "crypto/rand"
    "crypto/sha256"
    "encoding/base64"
    "encoding/hex"
    "fmt"
    "net/http"
    "strings"
    "time"

    "smallgo/server/audit"
    "smallgo/server/response"

    "github.com/gin-gonic/gin"
    "gorm.io/gorm"
)

type BookDTO struct {
    ID                uint   `json:"id"`
    Name              string `json:"name"`
    CareRecipientName string `json:"care_recipient_name"`
    Relation          string `json:"relation"`
    Color             string `json:"color"`
    Timezone          string `json:"timezone"`
    Notes             string `json:"notes"`
    Status            string `json:"status"`
    Role              string `json:"role"`
    IsCareRecipient   bool   `json:"is_care_recipient"`
    // Permissions 是「查看者」在这个用药本里最终生效的能力，前端据此决定按钮显隐。
    Permissions         []string  `json:"permissions"`
    CareRecipientUserID *uint     `json:"care_recipient_user_id,omitempty"`
    OwnerUserID         uint      `json:"owner_user_id"`
    OwnerName           string    `json:"owner_name"`
    MemberCount         int64     `json:"member_count"`
    CreatedByName       string    `json:"created_by_name"`
    UpdatedByName       string    `json:"updated_by_name"`
    CreatedAt           time.Time `json:"created_at"`
    UpdatedAt           time.Time `json:"updated_at"`
}

type MemberDTO struct {
    UserID          uint   `json:"user_id"`
    Username        string `json:"username"`
    Role            string `json:"role"`
    IsCareRecipient bool   `json:"is_care_recipient"`
    // Permissions 是该成员最终生效的能力；CustomPermissions 表示管理员单独勾选过
    // （true）还是跟随角色默认（false）。管理员在成员管理里改的就是这个。
    Permissions          []string  `json:"permissions"`
    CustomPermissions    bool      `json:"custom_permissions"`
    JoinedAt             time.Time `json:"joined_at"`
    IsSelf               bool      `json:"is_self"`
    NotificationsEnabled *bool     `json:"notifications_enabled,omitempty"`
}

type InviteDTO struct {
    ID              uint       `json:"id"`
    Role            string     `json:"role"`
    AsCareRecipient bool       `json:"as_care_recipient"`
    MaxUses         int        `json:"max_uses"`
    UsedCount       int        `json:"used_count"`
    ExpiresAt       time.Time  `json:"expires_at"`
    RevokedAt       *time.Time `json:"revoked_at,omitempty"`
    CreatedAt       time.Time  `json:"created_at"`
    Status          string     `json:"status"`
    // IsMine 表示这条邀请是当前用户生成的（成员只能撤销自己的邀请）。
    IsMine bool `json:"is_mine"`
}

type InviteCreatedDTO struct {
    InviteDTO
    Token string `json:"token"`
    Code  string `json:"code"`
    Link  string `json:"link"`
}

type InvitePreviewDTO struct {
    BookName          string    `json:"book_name"`
    CareRecipientName string    `json:"care_recipient_name"`
    Role              string    `json:"role"`
    AsCareRecipient   bool      `json:"as_care_recipient"`
    ExpiresAt         time.Time `json:"expires_at"`
    InviterName       string    `json:"inviter_name"`
    AlreadyMember     bool      `json:"already_member"`
}

func bookToDTO(db *gorm.DB, book MedicationBook, role string, viewerUserID uint) BookDTO {
    var count int64
    db.Model(&BookMember{}).Where("book_id = ?", book.ID).Count(&count)
    var recipient BookMember
    var recipientID *uint
    if db.Where("book_id = ? AND is_care_recipient = ?", book.ID, true).First(&recipient).Error == nil {
        id := recipient.UserID
        recipientID = &id
    }
    isRecipient := viewerUserID != 0 && recipientID != nil && *recipientID == viewerUserID
    // 查看者最终生效的能力：能查到成员行就按「单独勾选优先、否则角色默认」算，
    // 查不到（例如刚创建、或调用方显式传角色）就退化成角色默认。
    permissions := defaultPermissions(role, isRecipient)
    var viewer BookMember
    if db.Where("book_id = ? AND user_id = ?", book.ID, viewerUserID).First(&viewer).Error == nil {
        viewer.Role = role
        permissions = effectivePermissions(viewer)
    }
    return BookDTO{
        ID: book.ID, Name: book.Name, CareRecipientName: book.CareRecipientName,
        Relation: book.Relation, Color: book.Color, Timezone: book.Timezone,
        Notes: book.Notes, Status: book.Status, Role: role,
        IsCareRecipient: isRecipient, Permissions: permissions,
        CareRecipientUserID: recipientID, OwnerUserID: book.OwnerUserID,
        OwnerName: getUserName(db, book.OwnerUserID), MemberCount: count,
        CreatedByName: userNameOrEmpty(db, book.CreatedBy), UpdatedByName: userNameOrEmpty(db, book.UpdatedBy),
        CreatedAt: book.CreatedAt, UpdatedAt: book.UpdatedAt,
    }
}

func handleListBooks(db *gorm.DB) gin.HandlerFunc {
    return func(c *gin.Context) {
        userID := currentUserID(c)
        var members []BookMember
        if err := db.Where("user_id = ?", userID).Order("joined_at ASC").Find(&members).Error; err != nil {
            response.ErrorInternal(c, "读取用药本失败")
            return
        }
        result := make([]BookDTO, 0, len(members))
        for _, m := range members {
            var book MedicationBook
            if err := db.Where("id = ? AND deleted_at IS NULL", m.BookID).First(&book).Error; err != nil {
                continue
            }
            result = append(result, bookToDTO(db, book, m.Role, m.UserID))
        }
        response.Success(c, result)
    }
}

func handleCreateBook(db *gorm.DB) gin.HandlerFunc {
    return func(c *gin.Context) {
        var in struct {
            Name              string `json:"name"`
            CareRecipientName string `json:"care_recipient_name"`
            Relation          string `json:"relation"`
            Color             string `json:"color"`
            Timezone          string `json:"timezone"`
            Notes             string `json:"notes"`
            // CareRecipientIsSelf 表示用药人就是创建者自己；
            // 为 false 时创建者只是监护人，用药人可稍后通过邀请加入或被指定。
            CareRecipientIsSelf *bool `json:"care_recipient_is_self"`
        }
        if err := c.ShouldBindJSON(&in); err != nil {
            response.ErrorBadRequest(c, "参数无效")
            return
        }
        in.Name = strings.TrimSpace(in.Name)
        in.CareRecipientName = strings.TrimSpace(in.CareRecipientName)
        if in.Name == "" || len([]rune(in.Name)) > 60 {
            response.ErrorBadRequest(c, "用药本名称应为 1–60 个字符")
            return
        }
        if in.CareRecipientName == "" || len([]rune(in.CareRecipientName)) > 40 {
            response.ErrorBadRequest(c, "用药人称呼应为 1–40 个字符")
            return
        }
        if in.Color == "" {
            in.Color = "sage"
        }
        userID := currentUserID(c)
        now := nowUTC()
        book := MedicationBook{
            Name: in.Name, CareRecipientName: in.CareRecipientName, Relation: in.Relation,
            Color: in.Color, Timezone: BeijingTimezone, Notes: in.Notes,
            OwnerUserID: userID, Status: BookActive, CreatedBy: userID, CreatedAt: now, UpdatedAt: now,
        }
        isRecipient := true
        if in.CareRecipientIsSelf != nil {
            isRecipient = *in.CareRecipientIsSelf
        }
        err := db.Transaction(func(tx *gorm.DB) error {
            if err := tx.Create(&book).Error; err != nil {
                return err
            }
            member := BookMember{BookID: book.ID, UserID: userID, Role: RoleOwner, IsCareRecipient: isRecipient, NotificationsEnabled: true, JoinedAt: now}
            if err := tx.Create(&member).Error; err != nil {
                return err
            }
            pref := getPreferences(tx, userID)
            pref.LastBookID = &book.ID
            tx.Save(&pref)
            return nil
        })
        if err != nil {
            response.ErrorInternal(c, "创建用药本失败")
            return
        }
        audit.Log(db, c, "medcare_book_create", "medication_book", book.ID, book.Name)
        response.Success(c, bookToDTO(db, book, RoleOwner, userID))
    }
}

func handleGetBook(db *gorm.DB) gin.HandlerFunc {
    return func(c *gin.Context) {
        book, err := getBook(db, contextBookID(c))
        if err != nil {
            response.ErrorNotFound(c, "用药本不存在")
            return
        }
        response.Success(c, bookToDTO(db, book, contextBookRole(c), currentUserID(c)))
    }
}

func handleUpdateBook(db *gorm.DB) gin.HandlerFunc {
    return func(c *gin.Context) {
        var in struct {
            Name              string `json:"name"`
            CareRecipientName string `json:"care_recipient_name"`
            Relation          string `json:"relation"`
            Color             string `json:"color"`
            Timezone          string `json:"timezone"`
            Notes             string `json:"notes"`
        }
        if err := c.ShouldBindJSON(&in); err != nil {
            response.ErrorBadRequest(c, "参数无效")
            return
        }
        book, err := getBook(db, contextBookID(c))
        if err != nil {
            response.ErrorNotFound(c, "用药本不存在")
            return
        }
        if name := strings.TrimSpace(in.Name); name != "" {
            if len([]rune(name)) > 60 {
                response.ErrorBadRequest(c, "名称不能超过 60 个字符")
                return
            }
            book.Name = name
        }
        if name := strings.TrimSpace(in.CareRecipientName); name != "" {
            if len([]rune(name)) > 40 {
                response.ErrorBadRequest(c, "用药人称呼不能超过 40 个字符")
                return
            }
            book.CareRecipientName = name
        }
        if in.Relation != "" {
            book.Relation = in.Relation
        }
        if in.Color != "" {
            book.Color = in.Color
        }
        book.Timezone = BeijingTimezone
        book.Notes = in.Notes
        book.UpdatedBy = currentUserID(c)
        book.UpdatedAt = nowUTC()
        if err := db.Save(&book).Error; err != nil {
            response.ErrorInternal(c, "保存用药本失败")
            return
        }
        audit.Log(db, c, "medcare_book_update", "medication_book", book.ID, book.Name)
        response.Success(c, bookToDTO(db, book, contextBookRole(c), currentUserID(c)))
    }
}

func handleArchiveBook(db *gorm.DB, archive bool) gin.HandlerFunc {
    return func(c *gin.Context) {
        book, err := getBook(db, contextBookID(c))
        if err != nil {
            response.ErrorNotFound(c, "用药本不存在")
            return
        }
        now := nowUTC()
        if archive {
            book.Status = BookArchived
            book.ArchivedAt = &now
            cancelBookJobs(db, book.ID, "用药本已归档")
        } else {
            book.Status = BookActive
            book.ArchivedAt = nil
        }
        book.UpdatedAt = now
        if err := db.Save(&book).Error; err != nil {
            response.ErrorInternal(c, "更新用药本失败")
            return
        }
        audit.Log(db, c, "medcare_book_archive", "medication_book", book.ID, book.Status)
        response.Success(c, bookToDTO(db, book, contextBookRole(c), currentUserID(c)))
    }
}

func handleDeleteBook(db *gorm.DB) gin.HandlerFunc {
    return func(c *gin.Context) {
        book, err := getBook(db, contextBookID(c))
        if err != nil {
            response.ErrorNotFound(c, "用药本不存在")
            return
        }
        now := nowUTC()
        book.Status = BookArchived
        book.ArchivedAt = &now
        book.DeletedAt = &now
        book.UpdatedAt = now
        if err := db.Save(&book).Error; err != nil {
            response.ErrorInternal(c, "删除用药本失败")
            return
        }
        cancelBookJobs(db, book.ID, "用药本已删除")
        audit.Log(db, c, "medcare_book_delete", "medication_book", book.ID, book.Name)
        response.Success(c, gin.H{"deleted": true})
    }
}

func handleTransferBook(db *gorm.DB) gin.HandlerFunc {
    return func(c *gin.Context) {
        var in struct {
            UserID uint `json:"user_id"`
        }
        if err := c.ShouldBindJSON(&in); err != nil || in.UserID == 0 {
            response.ErrorBadRequest(c, "请选择新的管理员")
            return
        }
        book, err := getBook(db, contextBookID(c))
        if err != nil {
            response.ErrorNotFound(c, "用药本不存在")
            return
        }
        if in.UserID == book.OwnerUserID {
            response.ErrorBadRequest(c, "该成员已经是管理员")
            return
        }
        var target BookMember
        if err := db.Where("book_id = ? AND user_id = ?", book.ID, in.UserID).First(&target).Error; err != nil {
            response.ErrorBadRequest(c, "新管理员必须是该用药本成员")
            return
        }
        err = db.Transaction(func(tx *gorm.DB) error {
            if err := tx.Model(&BookMember{}).Where("book_id = ? AND user_id = ?", book.ID, book.OwnerUserID).Update("role", RoleManager).Error; err != nil {
                return err
            }
            if err := tx.Model(&BookMember{}).Where("book_id = ? AND user_id = ?", book.ID, in.UserID).Update("role", RoleOwner).Error; err != nil {
                return err
            }
            return tx.Model(&book).Updates(map[string]interface{}{"owner_user_id": in.UserID, "updated_at": nowUTC()}).Error
        })
        if err != nil {
            response.ErrorInternal(c, "转让失败")
            return
        }
        audit.Log(db, c, "medcare_book_transfer", "medication_book", book.ID, fmt.Sprintf("to user %d", in.UserID))
        book.OwnerUserID = in.UserID
        response.Success(c, bookToDTO(db, book, RoleManager, currentUserID(c)))
    }
}

func handleListMembers(db *gorm.DB) gin.HandlerFunc {
    return func(c *gin.Context) {
        bookID := contextBookID(c)
        userID := currentUserID(c)
        var members []BookMember
        if err := db.Where("book_id = ?", bookID).Order("CASE WHEN is_care_recipient THEN 0 ELSE 1 END, CASE role WHEN 'owner' THEN 0 WHEN 'manager' THEN 1 ELSE 2 END, joined_at ASC").Find(&members).Error; err != nil {
            response.ErrorInternal(c, "读取成员失败")
            return
        }
        result := make([]MemberDTO, 0, len(members))
        for _, m := range members {
            _, custom := permissionsOverride(m.Permissions)
            item := MemberDTO{UserID: m.UserID, Username: getUserName(db, m.UserID), Role: m.Role, IsCareRecipient: m.IsCareRecipient,
                Permissions: effectivePermissions(m), CustomPermissions: custom,
                JoinedAt: m.JoinedAt, IsSelf: m.UserID == userID}
            if m.UserID == userID {
                enabled := m.NotificationsEnabled
                item.NotificationsEnabled = &enabled
            }
            result = append(result, item)
        }
        response.Success(c, result)
    }
}

// handleSetCareRecipient 指定用药人。用药本内用药人有且只有一位，
// 所有者与监护人都可以指定；被指定者必须是该用药本成员。
func handleSetCareRecipient(db *gorm.DB) gin.HandlerFunc {
    return func(c *gin.Context) {
        var in struct {
            UserID uint `json:"user_id"`
        }
        if err := c.ShouldBindJSON(&in); err != nil || in.UserID == 0 {
            response.ErrorBadRequest(c, "请选择用药人")
            return
        }
        bookID := contextBookID(c)
        var target BookMember
        if err := db.Where("book_id = ? AND user_id = ?", bookID, in.UserID).First(&target).Error; err != nil {
            response.ErrorBadRequest(c, "用药人必须是该用药本的成员")
            return
        }
        err := db.Transaction(func(tx *gorm.DB) error {
            if err := tx.Model(&BookMember{}).Where("book_id = ? AND is_care_recipient = ?", bookID, true).Update("is_care_recipient", false).Error; err != nil {
                return err
            }
            if err := tx.Model(&BookMember{}).Where("book_id = ? AND user_id = ?", bookID, in.UserID).Update("is_care_recipient", true).Error; err != nil {
                return err
            }
            return nil
        })
        if err != nil {
            response.ErrorInternal(c, "指定用药人失败")
            return
        }
        audit.Log(db, c, "medcare_care_recipient_set", "book_member", in.UserID, bookName(db, bookID))
        response.Success(c, gin.H{"ok": true, "care_recipient_user_id": in.UserID})
    }
}

// handleUpdateMemberRole 修改成员权限层级（所有者 / 监护人 / 成员）。
func handleUpdateMemberRole(db *gorm.DB) gin.HandlerFunc {
    return func(c *gin.Context) {
        targetID, ok := parseUintParam(c, "userId")
        if !ok {
            return
        }
        var in struct {
            Role string `json:"role"`
        }
        if err := c.ShouldBindJSON(&in); err != nil {
            response.ErrorBadRequest(c, "参数无效")
            return
        }
        if in.Role != RoleOwner && in.Role != RoleManager && in.Role != RoleMember {
            response.ErrorBadRequest(c, "角色只能是所有者、监护人或成员")
            return
        }
        book, err := getBook(db, contextBookID(c))
        if err != nil {
            response.ErrorNotFound(c, "用药本不存在")
            return
        }
        if targetID == book.OwnerUserID {
            response.ErrorBadRequest(c, "不能修改管理员的角色，请先转交管理权限")
            return
        }
        if in.Role == RoleOwner {
            response.ErrorBadRequest(c, "请使用转交管理权限把管理员交给他人")
            return
        }
        result := db.Model(&BookMember{}).Where("book_id = ? AND user_id = ?", book.ID, targetID).Updates(map[string]interface{}{
            // 换角色时清掉单独勾选的能力，让新角色的默认能力生效，
            // 避免「降成成员了还留着药品管理」这种看不见的残留。
            "role":        in.Role,
            "permissions": "",
        })
        if result.Error != nil {
            response.ErrorInternal(c, "修改角色失败")
            return
        }
        if result.RowsAffected == 0 {
            response.ErrorNotFound(c, "成员不存在")
            return
        }
        audit.Log(db, c, "medcare_member_role", "book_member", targetID, in.Role)
        response.Success(c, gin.H{"ok": true})
    }
}

// handlePermissionCatalog 返回能力矩阵与各角色的默认勾选。
// 前端「成员与权限」的面板照这份渲染，避免同一套文案在前后端各维护一份。
func handlePermissionCatalog(c *gin.Context) {
    member := contextBookMember(c)
    _, custom := permissionsOverride(member.Permissions)
    response.Success(c, gin.H{
        "modules": permissionModules(),
        "keys":    allPermissionKeys(),
        "defaults": gin.H{
            // 管理员恒为全部能力，不可削减（成员管理只有管理员能做）。
            "owner":          allPermissionKeys(),
            "manager":        defaultPermissions(RoleManager, false),
            "member":         defaultPermissions(RoleMember, false),
            "care_recipient": defaultPermissions(RoleMember, true),
        },
        "mine":               effectivePermissions(member),
        "mine_custom":        member.Role == RoleOwner || custom,
        "can_manage_members": contextBookRole(c) == RoleOwner,
    })
}

// handleUpdateMemberPermissions 管理员逐人指定权限：
// permissions 传数组＝逐格勾选（空数组＝一格都不给，连列表都不能看）；传 null＝恢复跟随角色默认。
// 接口幂等：同样的集合重复提交，结果一致。
func handleUpdateMemberPermissions(db *gorm.DB) gin.HandlerFunc {
    return func(c *gin.Context) {
        targetID, ok := parseUintParam(c, "userId")
        if !ok {
            return
        }
        var in struct {
            Permissions permissionInput `json:"permissions"`
        }
        if err := c.ShouldBindJSON(&in); err != nil || !in.Permissions.submitted() {
            response.ErrorBadRequest(c, "请提交 permissions：数组表示单独指定能力，null 表示跟随角色默认")
            return
        }
        bookID := contextBookID(c)
        book, err := getBook(db, bookID)
        if err != nil {
            response.ErrorNotFound(c, "用药本不存在")
            return
        }
        var target BookMember
        if err := db.Where("book_id = ? AND user_id = ?", bookID, targetID).First(&target).Error; err != nil {
            response.ErrorNotFound(c, "成员不存在")
            return
        }
        if target.UserID == book.OwnerUserID {
            response.ErrorBadRequest(c, "管理员始终拥有全部权限，需要限制请先转交管理权限")
            return
        }
        custom := !in.Permissions.isNull()
        updates := map[string]interface{}{}
        if custom {
            normalized, err := in.Permissions.keys()
            if err != nil {
                response.ErrorBadRequest(c, err.Error())
                return
            }
            updates["permissions"] = encodePermissions(normalized)
        } else {
            updates["permissions"] = ""
        }
        if err := db.Model(&BookMember{}).Where("book_id = ? AND user_id = ?", bookID, targetID).Updates(updates).Error; err != nil {
            response.ErrorInternal(c, "保存权限失败")
            return
        }
        var saved BookMember
        if err := db.Where("book_id = ? AND user_id = ?", bookID, targetID).First(&saved).Error; err != nil {
            response.ErrorInternal(c, "读取权限失败")
            return
        }
        effective := effectivePermissions(saved)
        audit.Log(db, c, "medcare_member_permissions", "book_member", targetID, bookName(db, bookID)+"："+strings.Join(effective, ","))
        response.Success(c, gin.H{
            "ok":                 true,
            "user_id":            targetID,
            "permissions":        effective,
            "custom_permissions": custom,
        })
    }
}

func handleRemoveMember(db *gorm.DB) gin.HandlerFunc {
    return func(c *gin.Context) {
        targetID, ok := parseUintParam(c, "userId")
        if !ok {
            return
        }
        book, err := getBook(db, contextBookID(c))
        if err != nil {
            response.ErrorNotFound(c, "用药本不存在")
            return
        }
        if targetID == book.OwnerUserID {
            response.ErrorBadRequest(c, "不能移除管理员，请先转交管理权限")
            return
        }
        result := db.Where("book_id = ? AND user_id = ?", book.ID, targetID).Delete(&BookMember{})
        if result.Error != nil {
            response.ErrorInternal(c, "移除成员失败")
            return
        }
        if result.RowsAffected == 0 {
            response.ErrorNotFound(c, "成员不存在")
            return
        }
        cancelMemberJobs(db, book.ID, targetID, "成员已被移除")
        audit.Log(db, c, "medcare_member_remove", "book_member", targetID, book.Name)
        response.Success(c, gin.H{"ok": true})
    }
}

func handleLeaveBook(db *gorm.DB) gin.HandlerFunc {
    return func(c *gin.Context) {
        book, err := getBook(db, contextBookID(c))
        if err != nil {
            response.ErrorNotFound(c, "用药本不存在")
            return
        }
        userID := currentUserID(c)
        if userID == book.OwnerUserID {
            response.ErrorBadRequest(c, "管理员不能直接退出，请先转交管理权限")
            return
        }
        if err := db.Where("book_id = ? AND user_id = ?", book.ID, userID).Delete(&BookMember{}).Error; err != nil {
            response.ErrorInternal(c, "退出失败")
            return
        }
        cancelMemberJobs(db, book.ID, userID, "成员已退出")
        audit.Log(db, c, "medcare_member_leave", "book_member", userID, book.Name)
        response.Success(c, gin.H{"ok": true})
    }
}

func handleMyNotification(db *gorm.DB) gin.HandlerFunc {
    return func(c *gin.Context) {
        var in struct {
            Enabled *bool `json:"enabled"`
        }
        if err := c.ShouldBindJSON(&in); err != nil || in.Enabled == nil {
            response.ErrorBadRequest(c, "参数无效")
            return
        }
        bookID := contextBookID(c)
        userID := currentUserID(c)
        if err := db.Model(&BookMember{}).Where("book_id = ? AND user_id = ?", bookID, userID).Update("notifications_enabled", *in.Enabled).Error; err != nil {
            response.ErrorInternal(c, "保存通知设置失败")
            return
        }
        if !*in.Enabled {
            cancelMemberJobs(db, bookID, userID, "成员关闭了本用药本提醒")
        } else {
            go backfillMemberJobs(realtimeDB, bookID, userID)
        }
        response.Success(c, gin.H{"enabled": *in.Enabled})
    }
}

func randomToken(n int) (string, error) {
    b := make([]byte, n)
    if _, err := rand.Read(b); err != nil {
        return "", err
    }
    return base64.RawURLEncoding.EncodeToString(b), nil
}

func randomCode(n int) (string, error) {
    const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
    b := make([]byte, n)
    if _, err := rand.Read(b); err != nil {
        return "", err
    }
    out := make([]byte, n)
    for i, v := range b {
        out[i] = alphabet[int(v)%len(alphabet)]
    }
    return string(out), nil
}

func sha256Hex(s string) string {
    sum := sha256.Sum256([]byte(s))
    return hex.EncodeToString(sum[:])
}

func inviteStatus(inv BookInvite) string {
    if inv.RevokedAt != nil {
        return "revoked"
    }
    if nowUTC().After(inv.ExpiresAt) {
        return "expired"
    }
    if inv.UsedCount >= inv.MaxUses {
        return "exhausted"
    }
    return "active"
}

func handleCreateInvite(db *gorm.DB) gin.HandlerFunc {
    return func(c *gin.Context) {
        var in struct {
            Role         string `json:"role"`
            ExpiresHours int    `json:"expires_hours"`
            MaxUses      int    `json:"max_uses"`
            // AsCareRecipient 表示这位受邀人加入后就是用药人。
            AsCareRecipient bool `json:"as_care_recipient"`
        }
        if err := c.ShouldBindJSON(&in); err != nil {
            response.ErrorBadRequest(c, "参数无效")
            return
        }
        if in.Role == "" {
            in.Role = RoleManager
        }
        if in.Role != RoleManager && in.Role != RoleMember {
            response.ErrorBadRequest(c, "邀请角色只能是监护人或成员")
            return
        }
        // 成员只能邀请成员（「非监护人，可以增加成员」）；监护人可邀请监护人。
        if contextBookRole(c) == RoleMember && in.Role != RoleMember {
            response.ErrorForbidden(c, "成员只能邀请成员，邀请监护人需要管理员或监护人")
            return
        }
        // 只有管理员能指定「对方加入后就是用药人」。
        if in.AsCareRecipient && contextBookRole(c) != RoleOwner {
            response.ErrorForbidden(c, "只有管理员可以指定用药人")
            return
        }
        if in.ExpiresHours <= 0 {
            in.ExpiresHours = 168
        }
        if in.ExpiresHours > 24*30 {
            in.ExpiresHours = 24 * 30
        }
        if in.MaxUses <= 0 {
            in.MaxUses = 10
        }
        if in.MaxUses > 50 {
            in.MaxUses = 50
        }
        token, err := randomToken(24)
        if err != nil {
            response.ErrorInternal(c, "生成邀请失败")
            return
        }
        code, err := randomCode(6)
        if err != nil {
            response.ErrorInternal(c, "生成邀请失败")
            return
        }
        bookID := contextBookID(c)
        userID := currentUserID(c)
        inv := BookInvite{
            BookID: bookID, TokenHash: sha256Hex(token), CodeHash: sha256Hex(code),
            Role: in.Role, AsCareRecipient: in.AsCareRecipient, MaxUses: in.MaxUses, ExpiresAt: nowUTC().Add(time.Duration(in.ExpiresHours) * time.Hour),
            CreatedBy: userID, CreatedAt: nowUTC(),
        }
        if err := db.Create(&inv).Error; err != nil {
            response.ErrorInternal(c, "保存邀请失败")
            return
        }
        base := publicBaseURL(db, c)
        link := strings.TrimRight(base, "/") + "/join?token=" + token
        audit.Log(db, c, "medcare_invite_create", "book_invite", inv.ID, inv.Role)
        response.Success(c, InviteCreatedDTO{
            InviteDTO: InviteDTO{ID: inv.ID, Role: inv.Role, AsCareRecipient: inv.AsCareRecipient, MaxUses: inv.MaxUses, UsedCount: inv.UsedCount, ExpiresAt: inv.ExpiresAt, CreatedAt: inv.CreatedAt, Status: inviteStatus(inv)},
            Token:     token, Code: code, Link: link,
        })
    }
}

func handleListInvites(db *gorm.DB) gin.HandlerFunc {
    return func(c *gin.Context) {
        var rows []BookInvite
        if err := db.Where("book_id = ?", contextBookID(c)).Order("created_at DESC").Find(&rows).Error; err != nil {
            response.ErrorInternal(c, "读取邀请失败")
            return
        }
        result := make([]InviteDTO, 0, len(rows))
        for _, inv := range rows {
            result = append(result, InviteDTO{ID: inv.ID, Role: inv.Role, AsCareRecipient: inv.AsCareRecipient, MaxUses: inv.MaxUses, UsedCount: inv.UsedCount, ExpiresAt: inv.ExpiresAt, RevokedAt: inv.RevokedAt, CreatedAt: inv.CreatedAt, Status: inviteStatus(inv), IsMine: inv.CreatedBy == currentUserID(c)})
        }
        response.Success(c, result)
    }
}

func handleRevokeInvite(db *gorm.DB) gin.HandlerFunc {
    return func(c *gin.Context) {
        inviteID, ok := parseUintParam(c, "inviteId")
        if !ok {
            return
        }
        var inv BookInvite
        if err := db.Where("id = ? AND book_id = ?", inviteID, contextBookID(c)).First(&inv).Error; err != nil {
            response.ErrorNotFound(c, "邀请不存在")
            return
        }
        // 成员可以撤销自己生成的邀请；他人的邀请需要管理员或监护人。
        if inv.CreatedBy != currentUserID(c) && !roleAtLeast(contextBookRole(c), RoleManager) {
            response.ErrorForbidden(c, "只能撤销自己生成的邀请")
            return
        }
        now := nowUTC()
        result := db.Model(&BookInvite{}).Where("id = ? AND book_id = ?", inviteID, contextBookID(c)).Update("revoked_at", &now)
        if result.Error != nil {
            response.ErrorInternal(c, "撤销邀请失败")
            return
        }
        if result.RowsAffected == 0 {
            response.ErrorNotFound(c, "邀请不存在")
            return
        }
        audit.Log(db, c, "medcare_invite_revoke", "book_invite", inviteID, "")
        response.Success(c, gin.H{"ok": true})
    }
}

func publicBaseURL(db *gorm.DB, c *gin.Context) string {
    var cfg SystemNotificationConfig
    if err := db.First(&cfg, 1).Error; err == nil && cfg.PublicBaseURL != "" {
        return cfg.PublicBaseURL
    }
    if c == nil {
        return ""
    }
    scheme := "http"
    if c.Request.TLS != nil || strings.EqualFold(c.GetHeader("X-Forwarded-Proto"), "https") {
        scheme = "https"
    }
    return scheme + "://" + c.Request.Host
}

func loadInviteByToken(db *gorm.DB, token string) (BookInvite, error) {
    var inv BookInvite
    err := db.Where("token_hash = ?", sha256Hex(token)).First(&inv).Error
    return inv, err
}

func loadInviteByCode(db *gorm.DB, code string) (BookInvite, error) {
    var inv BookInvite
    err := db.Where("code_hash = ?", sha256Hex(strings.ToUpper(strings.TrimSpace(code)))).First(&inv).Error
    return inv, err
}

func inviteUsable(inv BookInvite) bool {
    return inv.RevokedAt == nil && nowUTC().Before(inv.ExpiresAt) && inv.UsedCount < inv.MaxUses
}

func handlePreviewInvite(db *gorm.DB) gin.HandlerFunc {
    return func(c *gin.Context) {
        inv, err := loadInviteByToken(db, c.Param("token"))
        if err != nil {
            response.Error(c, http.StatusGone, 2010, "邀请已失效")
            return
        }
        if !inviteUsable(inv) {
            response.Error(c, http.StatusGone, 2010, "邀请已过期或被撤销")
            return
        }
        var book MedicationBook
        if err := db.Where("id = ? AND deleted_at IS NULL", inv.BookID).First(&book).Error; err != nil {
            response.Error(c, http.StatusGone, 2010, "用药本不存在")
            return
        }
        _, memberErr := getMember(db, inv.BookID, currentUserID(c))
        response.Success(c, InvitePreviewDTO{
            BookName: book.Name, CareRecipientName: book.CareRecipientName, Role: inv.Role,
            AsCareRecipient: inv.AsCareRecipient,
            ExpiresAt:       inv.ExpiresAt, InviterName: getUserName(db, inv.CreatedBy), AlreadyMember: memberErr == nil,
        })
    }
}

func joinInvite(db *gorm.DB, c *gin.Context, inv BookInvite) {
    if !inviteUsable(inv) {
        response.Error(c, http.StatusGone, 2010, "邀请已过期或被撤销")
        return
    }
    book, err := getBook(db, inv.BookID)
    if err != nil {
        response.Error(c, http.StatusGone, 2010, "用药本不存在")
        return
    }
    userID := currentUserID(c)
    var result BookDTO
    err = db.Transaction(func(tx *gorm.DB) error {
        var existing BookMember
        if tx.Where("book_id = ? AND user_id = ?", inv.BookID, userID).First(&existing).Error == nil {
            result = bookToDTO(tx, book, existing.Role, userID)
            return nil
        }
        // 若邀请标记为「用药人」，加入时设为用药人（用药本内唯一）。
        if inv.AsCareRecipient {
            if err := tx.Model(&BookMember{}).Where("book_id = ? AND is_care_recipient = ?", inv.BookID, true).Update("is_care_recipient", false).Error; err != nil {
                return err
            }
        }
        member := BookMember{BookID: inv.BookID, UserID: userID, Role: inv.Role, IsCareRecipient: inv.AsCareRecipient, NotificationsEnabled: true, JoinedAt: nowUTC(), InvitedBy: uintPtr(inv.CreatedBy)}
        if err := tx.Create(&member).Error; err != nil {
            return err
        }
        if err := tx.Model(&inv).Update("used_count", gorm.Expr("used_count + 1")).Error; err != nil {
            return err
        }
        result = bookToDTO(tx, book, inv.Role, userID)
        return nil
    })
    if err != nil {
        response.ErrorInternal(c, "加入用药本失败")
        return
    }
    audit.Log(db, c, "medcare_invite_join", "medication_book", book.ID, book.Name)
    go backfillMemberJobs(realtimeDB, book.ID, userID)
    response.Success(c, result)
}

func handleJoinInvite(db *gorm.DB) gin.HandlerFunc {
    return func(c *gin.Context) {
        inv, err := loadInviteByToken(db, c.Param("token"))
        if err != nil {
            response.Error(c, http.StatusGone, 2010, "邀请已失效")
            return
        }
        joinInvite(db, c, inv)
    }
}

func handleJoinByCode(db *gorm.DB) gin.HandlerFunc {
    return func(c *gin.Context) {
        var in struct {
            Code string `json:"code"`
        }
        if err := c.ShouldBindJSON(&in); err != nil {
            response.ErrorBadRequest(c, "请输入邀请码")
            return
        }
        inv, err := loadInviteByCode(db, in.Code)
        if err != nil {
            response.Error(c, http.StatusGone, 2010, "邀请码无效")
            return
        }
        joinInvite(db, c, inv)
    }
}

func handleGetPreferences(db *gorm.DB) gin.HandlerFunc {
    return func(c *gin.Context) {
        response.Success(c, getPreferences(db, currentUserID(c)))
    }
}

func handleUpdatePreferences(db *gorm.DB) gin.HandlerFunc {
    return func(c *gin.Context) {
        var in UserPreference
        if err := c.ShouldBindJSON(&in); err != nil {
            response.ErrorBadRequest(c, "参数无效")
            return
        }
        pref := getPreferences(db, currentUserID(c))
        pref.Timezone = BeijingTimezone
        if in.LeadMinutes >= 0 && in.LeadMinutes <= 120 {
            pref.LeadMinutes = in.LeadMinutes
        }
        if in.OverdueFirstMinutes >= 5 && in.OverdueFirstMinutes <= 240 {
            pref.OverdueFirstMinutes = in.OverdueFirstMinutes
        }
        // 重复提醒间隔支持 1 分钟起，配合「直到点完成」的反复提醒。
        if in.OverdueRepeatMinutes >= 1 && in.OverdueRepeatMinutes <= 1440 {
            pref.OverdueRepeatMinutes = in.OverdueRepeatMinutes
        }
        if in.OverdueMaxRepeats >= 0 && in.OverdueMaxRepeats <= 5 {
            pref.OverdueMaxRepeats = in.OverdueMaxRepeats
        }
        if in.DailySummaryTime != "" {
            if _, err := time.Parse("15:04", in.DailySummaryTime); err != nil {
                response.ErrorBadRequest(c, "总结时间格式应为 HH:MM")
                return
            }
            pref.DailySummaryTime = in.DailySummaryTime
        }
        pref.DailySummaryEnabled = in.DailySummaryEnabled
        pref.LowStockNotifyEnabled = in.LowStockNotifyEnabled
        switch in.ThemeMode {
        case "system", "light", "dark":
            pref.ThemeMode = in.ThemeMode
        case "":
            if pref.ThemeMode == "" {
                pref.ThemeMode = "system"
            }
        default:
            response.ErrorBadRequest(c, "主题模式无效")
            return
        }
        switch in.ThemeBrand {
        case "a", "b", "c", "d", "e", "f", "g", "h", "i", "j":
            pref.ThemeBrand = in.ThemeBrand
        case "":
            if pref.ThemeBrand == "" {
                pref.ThemeBrand = "a"
            }
        default:
            response.ErrorBadRequest(c, "主题配色无效")
            return
        }
        pref.IncludeLocationInNotifications = in.IncludeLocationInNotifications
        pref.IncludeNotesInNotifications = in.IncludeNotesInNotifications
        pref.OnboardingCompleted = in.OnboardingCompleted
        if in.LastBookID != nil {
            pref.LastBookID = in.LastBookID
        }
        pref.UpdatedAt = nowUTC()
        if err := db.Save(&pref).Error; err != nil {
            response.ErrorInternal(c, "保存偏好失败")
            return
        }
        response.Success(c, pref)
    }
}
