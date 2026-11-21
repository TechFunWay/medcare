package medcare

import (
    "encoding/json"
    "errors"
    "net/http"
    "strconv"
    "strings"
    "time"

    "smallgo/server/database"
    "smallgo/server/logger"
    "smallgo/server/response"
    "smallgo/server/sysconfig"

    "github.com/gin-gonic/gin"
    "gorm.io/gorm"
)

func currentUserID(c *gin.Context) uint {
    v, _ := c.Get("userID")
    switch id := v.(type) {
    case uint:
        return id
    case int:
        return uint(id)
    case float64:
        return uint(id)
    default:
        return 0
    }
}

func parseUintParam(c *gin.Context, name string) (uint, bool) {
    n, err := strconv.ParseUint(c.Param(name), 10, 64)
    if err != nil || n == 0 {
        response.ErrorBadRequest(c, "无效的资源 ID")
        return 0, false
    }
    return uint(n), true
}

func parseBodyID(raw string) (uint, bool) {
    n, err := strconv.ParseUint(strings.TrimSpace(raw), 10, 64)
    return uint(n), err == nil && n > 0
}

func roleRank(role string) int {
    switch role {
    case RoleOwner:
        return 3
    case RoleManager:
        return 2
    case RoleMember:
        return 1
    default:
        return 0
    }
}

func roleAtLeast(role, min string) bool {
    return roleRank(role) >= roleRank(min)
}

// requireBookMember 解析 /books/:bookId，校验当前用户是成员（可要求最低角色层级），
// 并把 bookID/bookRole/bookMember 放入 context。
func requireBookMember(db *gorm.DB, minRole string) gin.HandlerFunc {
    return func(c *gin.Context) {
        if _, ok := resolveBookMember(db, c, minRole, ""); !ok {
            return
        }
        c.Next()
    }
}

// requirePermission 按「能力」放行：先看管理员为该成员单独勾选的能力，
// 没有单独勾选就用角色默认能力（见 permissions.go）。
// 用于「谁可以点完成」「谁可以管药品」这类逐人配置的门禁。
func requirePermission(db *gorm.DB, perm string) gin.HandlerFunc {
    return func(c *gin.Context) {
        if _, ok := resolveBookMember(db, c, RoleMember, perm); !ok {
            return
        }
        c.Next()
    }
}

// resolveBookMember 是两种门禁的共同实现：解析路径、查成员、按角色层级与能力校验，
// 通过后把 bookID/bookRole/bookMember 写进 context。返回 false 表示已经写好响应并 Abort。
func resolveBookMember(db *gorm.DB, c *gin.Context, minRole, perm string) (BookMember, bool) {
    bookID, ok := parseUintParam(c, "bookId")
    if !ok {
        c.Abort()
        return BookMember{}, false
    }
    userID := currentUserID(c)
    var member BookMember
    err := db.Where("book_id = ? AND user_id = ?", bookID, userID).First(&member).Error
    if err != nil {
        // 非成员一律 404，不透露用药本是否存在。
        response.ErrorNotFound(c, "用药本不存在")
        c.Abort()
        return BookMember{}, false
    }
    if !roleAtLeast(member.Role, minRole) {
        response.ErrorForbidden(c, "权限不足")
        c.Abort()
        return member, false
    }
    if perm != "" && !memberHasPermission(member, perm) {
        response.ErrorForbidden(c, "管理员没有把「"+permissionLabel(perm)+"」的权限给你，需要的话请联系用药本管理员")
        c.Abort()
        return member, false
    }
    c.Set("bookID", bookID)
    c.Set("bookRole", member.Role)
    c.Set("bookMember", member)
    return member, true
}

// contextBookMember 返回 requireBookMember 放进 context 的成员对象。
func contextBookMember(c *gin.Context) BookMember {
    v, _ := c.Get("bookMember")
    if member, ok := v.(BookMember); ok {
        return member
    }
    return BookMember{}
}

// requireCanRecord 判断当前用户能否记录用药执行（完成/漏服/撤销/备注/稍后/调整）。
// 走「用药记录 · 点完成」这一格：管理员没单独勾选时，监护人与用药人本人默认有，
// 普通成员默认没有——与本次改动之前的行为一致。
func requireCanRecord(c *gin.Context) bool {
    return memberHasPermission(contextBookMember(c), PermTodayRecord)
}

// requirePermissionKey 在 handler 内部按能力复查（路由已用 requirePermission 时是双保险）。
// 通过返回 true；不通过已经写好 403 响应，返回 false。
func requirePermissionKey(c *gin.Context, perm string) bool {
    if memberHasPermission(contextBookMember(c), perm) {
        return true
    }
    response.ErrorForbidden(c, "管理员没有把「"+permissionLabel(perm)+"」的权限给你，需要的话请联系用药本管理员")
    return false
}

func contextBookID(c *gin.Context) uint {
    v, _ := c.Get("bookID")
    if id, ok := v.(uint); ok {
        return id
    }
    return 0
}

func contextBookRole(c *gin.Context) string {
    v, _ := c.Get("bookRole")
    if r, ok := v.(string); ok {
        return r
    }
    return ""
}

func writeError(c *gin.Context, err error) {
    switch {
    case errors.Is(err, gorm.ErrRecordNotFound), errors.Is(err, ErrNotFound):
        response.ErrorNotFound(c, "记录不存在")
    case errors.Is(err, ErrForbidden):
        response.ErrorForbidden(c, err.Error())
    case errors.Is(err, ErrVersionConflict):
        response.Error(c, http.StatusConflict, 2007, err.Error())
    case errors.Is(err, ErrMedicineInUse):
        response.Error(c, http.StatusConflict, 2001, err.Error())
    case errors.Is(err, ErrStockShortage):
        response.Error(c, http.StatusConflict, 2002, err.Error())
    case errors.Is(err, ErrStockExpired):
        response.Error(c, http.StatusConflict, 2003, err.Error())
    case errors.Is(err, ErrInvalidTransition):
        response.Error(c, http.StatusConflict, 2004, err.Error())
    case errors.Is(err, ErrChannelInvalid):
        response.ErrorBadRequest(c, err.Error())
    case errors.Is(err, ErrInviteInvalid):
        response.Error(c, http.StatusGone, 2010, err.Error())
    case errors.Is(err, ErrAIUnavailable):
        response.Error(c, http.StatusServiceUnavailable, 2011, err.Error())
    case errors.Is(err, ErrAISchemaInvalid):
        response.Error(c, http.StatusUnprocessableEntity, 2012, err.Error())
    default:
        response.ErrorInternal(c, "操作失败，请稍后重试")
    }
}

func decodeJSON(raw string, target interface{}) error {
    if strings.TrimSpace(raw) == "" {
        return nil
    }
    return json.Unmarshal([]byte(raw), target)
}

func encodeJSON(v interface{}) string {
    b, _ := json.Marshal(v)
    return string(b)
}

// BeijingTimezone 是本应用统一的业务时区。用户明确要求「时间的时区统一用
// 北京时间」，因此不再接受任意 IANA 时区，统一收敛到 Asia/Shanghai。
const BeijingTimezone = "Asia/Shanghai"

// normalizeTimezone 始终返回北京时间，忽略传入值；保留参数是为了兼容既有调用点。
func normalizeTimezone(_ string) string {
    return BeijingTimezone
}

func loadLocation(tz string) *time.Location {
    loc, err := time.LoadLocation(normalizeTimezone(tz))
    if err == nil {
        return loc
    }
    // 目标系统缺少时区数据库时 LoadLocation 会失败，此时退回固定东八区，
    // 绝不让 loc 为 nil——time.In(nil) 会直接 panic，把整个进程带下去。
    fallback, err := time.LoadLocation("Asia/Shanghai")
    if err == nil {
        return fallback
    }
    logger.Error("medcare: 时区数据库不可用，退回固定东八区: %v", err)
    return time.FixedZone("Asia/Shanghai", 8*60*60)
}

func todayIn(tz string) string {
    return nowUTC().In(loadLocation(tz)).Format("2006-01-02")
}

func parseLocalDate(loc *time.Location, date string) (time.Time, error) {
    return time.ParseInLocation("2006-01-02", strings.TrimSpace(date), loc)
}

func parseClock(times []string) ([]string, error) {
    seen := map[string]bool{}
    out := make([]string, 0, len(times))
    for _, raw := range times {
        t := strings.TrimSpace(raw)
        if t == "" {
            continue
        }
        if _, err := time.Parse("15:04", t); err != nil {
            return nil, err
        }
        if !seen[t] {
            seen[t] = true
            out = append(out, t)
        }
    }
    sortStrings(out)
    if len(out) == 0 {
        return nil, errors.New("至少需要选择一个时间")
    }
    if len(out) > 12 {
        return nil, errors.New("每天最多 12 个时间点")
    }
    return out, nil
}

func sortStrings(items []string) {
    for i := 1; i < len(items); i++ {
        for j := i; j > 0 && items[j] < items[j-1]; j-- {
            items[j], items[j-1] = items[j-1], items[j]
        }
    }
}

func getBook(db *gorm.DB, bookID uint) (MedicationBook, error) {
    var book MedicationBook
    err := db.Where("id = ? AND deleted_at IS NULL", bookID).First(&book).Error
    return book, err
}

func getMember(db *gorm.DB, bookID, userID uint) (BookMember, error) {
    var member BookMember
    err := db.Where("book_id = ? AND user_id = ?", bookID, userID).First(&member).Error
    return member, err
}

func getUserName(db *gorm.DB, userID uint) string {
    var user database.User
    if err := db.Select("username").First(&user, userID).Error; err != nil {
        return "未知用户"
    }
    return user.Username
}

// userNameOrEmpty 用于「新增人 / 最后更新人」展示：没有就不显示，避免出现「未知用户」。
func userNameOrEmpty(db *gorm.DB, userID uint) string {
    if userID == 0 {
        return ""
    }
    name := getUserName(db, userID)
    if name == "未知用户" {
        return ""
    }
    return name
}

func getPreferences(db *gorm.DB, userID uint) UserPreference {
    var pref UserPreference
    err := db.Where("user_id = ?", userID).First(&pref).Error
    if err == nil {
        return pref
    }
    modeDefault := "system"
    brandDefault := "a"
    if v, err := sysconfig.GetConfig(db, "medcare_default_theme_mode", 0); err == nil && v != "" {
        modeDefault = v
    }
    if v, err := sysconfig.GetConfig(db, "medcare_default_theme_brand", 0); err == nil && v != "" {
        brandDefault = v
    }
    pref = UserPreference{UserID: userID, Timezone: BeijingTimezone, ThemeMode: modeDefault, ThemeBrand: brandDefault, LeadMinutes: 10, OverdueFirstMinutes: 15, OverdueRepeatMinutes: 60, OverdueMaxRepeats: 2, DailySummaryEnabled: true, DailySummaryTime: "21:00", IncludeLocationInNotifications: true, LowStockNotifyEnabled: true}
    db.Create(&pref)
    return pref
}

func bookName(db *gorm.DB, bookID uint) string {
    b, err := getBook(db, bookID)
    if err != nil {
        return "用药本"
    }
    return b.Name
}

func isoTime(t *time.Time) interface{} {
    if t == nil {
        return nil
    }
    return t.UTC().Format(time.RFC3339)
}

func maskEmail(addr string) string {
    parts := strings.Split(addr, "@")
    if len(parts) != 2 {
        return "邮箱"
    }
    name := parts[0]
    if len(name) > 2 {
        name = name[:2] + "***"
    } else {
        name += "***"
    }
    return name + "@" + parts[1]
}

func maskWebhook(raw string) string {
    if raw == "" {
        return "Webhook（已加密）"
    }
    return "Webhook（已加密）"
}

func trimRunes(s string, max int) string {
    r := []rune(s)
    if len(r) <= max {
        return s
    }
    return string(r[:max])
}

// nowUTC 统一使用 UTC 写库与比较：SQLite 里时间按文本比较，
// 混用本地时区与 UTC 会让「未来/过去」判断出错。
func nowUTC() time.Time { return time.Now().UTC() }

func intPtr(v int) *int    { return &v }
func uintPtr(v uint) *uint { return &v }
