package medcare

import (
    "net/http"
    "net/http/httptest"
    "path/filepath"
    "strings"
    "testing"

    "github.com/gin-gonic/gin"
    "gorm.io/driver/sqlite"
    "gorm.io/gorm"
    gormlogger "gorm.io/gorm/logger"

    "smallgo/server/database"
)

func setupPermissionDB(t *testing.T) *gorm.DB {
    t.Helper()
    db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "perm.db")), &gorm.Config{Logger: gormlogger.Default.LogMode(gormlogger.Silent)})
    if err != nil {
        t.Fatalf("open db: %v", err)
    }
    if err := db.AutoMigrate(&MedicationBook{}, &BookMember{}, &database.AuditLog{}); err != nil {
        t.Fatalf("migrate: %v", err)
    }
    return db
}

func newPermissionBook(t *testing.T, db *gorm.DB) MedicationBook {
    t.Helper()
    book := MedicationBook{ID: 1, Name: "李奶奶的用药本", CareRecipientName: "李奶奶", OwnerUserID: 1, Status: BookActive, Timezone: BeijingTimezone}
    if err := db.Create(&book).Error; err != nil {
        t.Fatalf("create book: %v", err)
    }
    return book
}

func TestPermissionMatrixShape(t *testing.T) {
    // 模块 × 动作是这套权限的唯一来源：矩阵与扁平键表必须一致，
    // 否则前端渲染的格子与后端认的键会对不上。
    keys := allPermissionKeys()
    if len(keys) != 22 {
        t.Fatalf("能力键应为 22 项，实际 %d：%v", len(keys), keys)
    }
    for _, module := range permissionModules() {
        for _, action := range module.Actions {
            if action.Key != permKey(module.Module, action.Act) {
                t.Fatalf("%s 的键拼错：%s", module.Label, action.Key)
            }
            if strings.TrimSpace(action.Label) == "" {
                t.Fatalf("%s 的 %s 没有文案", module.Label, action.Act)
            }
        }
    }
    if got := permissionLabel(PermMedicineDelete); got != "药品与库存 · 删除" {
        t.Fatalf("报错文案应带模块名，实际 %s", got)
    }
}

func TestDefaultPermissionsFollowRoles(t *testing.T) {
    if len(defaultPermissions(RoleOwner, false)) != len(allPermissionKeys()) {
        t.Fatalf("管理员应默认全勾")
    }
    if len(defaultPermissions(RoleManager, false)) != len(allPermissionKeys()) {
        t.Fatalf("监护人应默认全勾")
    }
    member := defaultPermissions(RoleMember, false)
    for _, key := range []string{PermTodayView, PermTodayRemind, PermMedicineView, PermPlanView, PermPrescView, PermImageView, PermStatsView, PermMemberView, PermMemberInvite} {
        if !containsString(member, key) {
            t.Fatalf("成员默认应能查看 %s", key)
        }
    }
    for _, key := range []string{PermTodayRecord, PermMedicineCreate, PermMedicineEdit, PermMedicineDelete, PermPlanDelete, PermImageCreate, PermPrescEdit} {
        if containsString(member, key) {
            t.Fatalf("成员默认不该有写权限 %s，实际 %v", key, member)
        }
    }
    recipient := defaultPermissions(RoleMember, true)
    if !containsString(recipient, PermTodayRecord) || len(recipient) != len(member)+1 {
        t.Fatalf("用药人本人应只多一格「点完成」，实际 %v", recipient)
    }
}

func TestEffectivePermissionsWithOverride(t *testing.T) {
    // 勾了才有：只勾药品查看+新增的成员，连提醒都点不动。
    partial := BookMember{Role: RoleMember, Permissions: encodePermissions([]string{PermMedicineView, PermMedicineCreate})}
    if !memberHasPermission(partial, PermMedicineCreate) || !memberCanView(partial, ModMedicine) {
        t.Fatalf("勾选的格子应生效，实际 %v", effectivePermissions(partial))
    }
    if memberHasPermission(partial, PermTodayRemind) || memberCanView(partial, ModToday) {
        t.Fatalf("没勾的格子必须没有（含查看），实际 %v", effectivePermissions(partial))
    }

    // 空数组＝一格都不给，与「跟随角色默认」不同。
    none := BookMember{Role: RoleManager, Permissions: encodePermissions(nil)}
    if len(effectivePermissions(none)) != 0 || memberCanView(none, ModToday) {
        t.Fatalf("清空后不应有任何权限，实际 %v", effectivePermissions(none))
    }

    // 监护人可以被削减成只看不改（比如只能替老人看着，不动数据）。
    viewOnly := BookMember{Role: RoleManager, Permissions: encodePermissions([]string{PermTodayView, PermMedicineView, PermPlanView})}
    if memberHasPermission(viewOnly, PermTodayRecord) || memberHasPermission(viewOnly, PermPlanCreate) {
        t.Fatalf("被削减的监护人不应还能记录或建计划")
    }

    // 管理员不可被削减：成员管理只有管理员能做。
    owner := BookMember{Role: RoleOwner, Permissions: encodePermissions([]string{PermTodayView})}
    if !memberHasPermission(owner, PermMedicineDelete) || !memberHasPermission(owner, PermPlanCreate) {
        t.Fatalf("管理员应恒为全部格子，实际 %v", effectivePermissions(owner))
    }

    // 用药人被收回「点完成」后，管理员勾的说了算。
    elder := BookMember{Role: RoleMember, IsCareRecipient: true, Permissions: encodePermissions([]string{PermTodayView, PermTodayRemind})}
    if memberHasPermission(elder, PermTodayRecord) {
        t.Fatalf("被收回后用药人不能自己打卡")
    }
}

func TestLegacyPermissionKeysExpanded(t *testing.T) {
    // 上一版只有五格粗粒度能力；库里残留的旧键要展开，不能升级后变成只读。
    keys, override := permissionsOverride(`["remind","record","medicine"]`)
    if !override {
        t.Fatalf("旧键也应视为已单独勾选")
    }
    for _, want := range []string{PermTodayRemind, PermTodayRecord, PermMedicineDelete, PermPrescEdit, PermMedicineView, PermPlanView} {
        if !containsString(keys, want) {
            t.Fatalf("旧键展开后应包含 %s，实际 %v", want, keys)
        }
    }
    if _, err := normalizePermissionKeys([]string{"remind"}); err != nil {
        t.Fatalf("旧键提交应被接受并展开，实际报错 %v", err)
    }
}

func TestPermissionsOverrideParsing(t *testing.T) {
    if _, override := permissionsOverride(""); override {
        t.Fatalf("空串应表示跟随角色默认")
    }
    keys, override := permissionsOverride("[]")
    if !override || len(keys) != 0 {
        t.Fatalf("空数组应表示一格都不给，实际 %v / %v", keys, override)
    }
    // 脏数据不应把人锁死在门外。
    if _, override := permissionsOverride("不是 JSON"); override {
        t.Fatalf("无法解析的值应退化为跟随角色默认")
    }
}

func TestNormalizePermissionKeys(t *testing.T) {
    keys, err := normalizePermissionKeys([]string{PermMedicineDelete, " " + PermTodayView + " ", PermMedicineDelete, ""})
    if err != nil {
        t.Fatalf("归一化失败: %v", err)
    }
    if strings.Join(keys, ",") != PermTodayView+","+PermMedicineDelete {
        t.Fatalf("应去重并按矩阵顺序排列，实际 %v", keys)
    }
    if _, err := normalizePermissionKeys([]string{"medicine"}); err != nil {
        t.Fatalf("合法旧键不该报错: %v", err)
    }
    if _, err := normalizePermissionKeys([]string{"teleport"}); err == nil {
        t.Fatalf("未知能力键应报错")
    }
}

// requirePermission 要与 requireBookMember 一致：非成员 404、缺格子 403、勾了放行。
// 读接口同样受控——这是这次改动的重点：查看也是一格权限。
func TestRequirePermissionMiddleware(t *testing.T) {
    db := setupPermissionDB(t)
    newPermissionBook(t, db)
    members := []BookMember{
        {BookID: 1, UserID: 1, Role: RoleOwner},
        {BookID: 1, UserID: 2, Role: RoleMember},
        {BookID: 1, UserID: 3, Role: RoleMember, Permissions: encodePermissions([]string{PermMedicineView, PermMedicineCreate})},
        {BookID: 1, UserID: 4, Role: RoleMember, Permissions: encodePermissions([]string{PermTodayView, PermTodayRemind})},
        {BookID: 1, UserID: 5, Role: RoleManager},
    }
    for i := range members {
        if err := db.Create(&members[i]).Error; err != nil {
            t.Fatalf("create member: %v", err)
        }
    }

    router := gin.New()
    router.Use(func(c *gin.Context) {
        var id uint
        for _, ch := range c.GetHeader("X-Test-User") {
            id = id*10 + uint(ch-'0')
        }
        c.Set("userID", id)
        c.Next()
    })
    router.GET("/books/:bookId/medicines", requirePermission(db, PermMedicineView), func(c *gin.Context) {
        c.JSON(http.StatusOK, gin.H{"ok": true})
    })
    router.POST("/books/:bookId/medicines", requirePermission(db, PermMedicineCreate), func(c *gin.Context) {
        c.JSON(http.StatusOK, gin.H{"ok": true})
    })

    check := func(method, user string, want int) {
        t.Helper()
        req := httptest.NewRequest(method, "/books/1/medicines", strings.NewReader("{}"))
        req.Header.Set("X-Test-User", user)
        w := httptest.NewRecorder()
        router.ServeHTTP(w, req)
        if w.Code != want {
            t.Fatalf("%s 用户 %s 得到 %d，期望 %d，响应 %s", method, user, w.Code, want, w.Body.String())
        }
    }
    // 非成员：不透露用药本是否存在
    check(http.MethodGet, "9", http.StatusNotFound)
    check(http.MethodPost, "9", http.StatusNotFound)
    // 普通成员：能看不能改（角色默认）
    check(http.MethodGet, "2", http.StatusOK)
    check(http.MethodPost, "2", http.StatusForbidden)
    // 只勾了药品两格：能建药，但看不到今日安排
    check(http.MethodGet, "3", http.StatusOK)
    check(http.MethodPost, "3", http.StatusOK)
    // 只勾了用药记录两格：药品列表直接看不见
    check(http.MethodGet, "4", http.StatusForbidden)
    check(http.MethodPost, "4", http.StatusForbidden)
    // 监护人默认全勾；管理员恒为全勾
    check(http.MethodGet, "5", http.StatusOK)
    check(http.MethodPost, "1", http.StatusOK)
}

func TestUpdateMemberPermissionsHandler(t *testing.T) {
    db := setupPermissionDB(t)
    newPermissionBook(t, db)
    if err := db.Create(&BookMember{BookID: 1, UserID: 1, Role: RoleOwner}).Error; err != nil {
        t.Fatalf("create owner: %v", err)
    }
    if err := db.Create(&BookMember{BookID: 1, UserID: 2, Role: RoleManager}).Error; err != nil {
        t.Fatalf("create member: %v", err)
    }

    call := func(body string) *httptest.ResponseRecorder {
        w := httptest.NewRecorder()
        c, _ := gin.CreateTestContext(w)
        c.Params = gin.Params{{Key: "userId", Value: "2"}}
        c.Set("bookID", uint(1))
        c.Set("bookRole", RoleOwner)
        c.Set("bookMember", BookMember{BookID: 1, UserID: 1, Role: RoleOwner})
        c.Request = httptest.NewRequest(http.MethodPut, "/books/1/members/2/permissions", strings.NewReader(body))
        c.Request.Header.Set("Content-Type", "application/json")
        handleUpdateMemberPermissions(db)(c)
        return w
    }

    w := call(`{"permissions":["medicine:view","medicine:create"]}`)
    if w.Code != http.StatusOK {
        t.Fatalf("逐格勾选应成功，得到 %d %s", w.Code, w.Body.String())
    }
    var saved BookMember
    if err := db.Where("book_id = ? AND user_id = ?", 1, 2).First(&saved).Error; err != nil {
        t.Fatalf("read member: %v", err)
    }
    if strings.Join(effectivePermissions(saved), ",") != PermMedicineView+","+PermMedicineCreate {
        t.Fatalf("落库的勾选不对，实际 %v", effectivePermissions(saved))
    }
    // 削减后的监护人看不到计划、也不能删药
    if memberCanView(saved, ModPlan) || memberHasPermission(saved, PermMedicineDelete) {
        t.Fatalf("没勾的格子必须失效")
    }

    if w = call(`{"teleport":true}`); w.Code != http.StatusBadRequest {
        t.Fatalf("非法 permissions 应 400，得到 %d", w.Code)
    }
    if w = call(`{"permissions":["medicine:fly"]}`); w.Code != http.StatusBadRequest {
        t.Fatalf("未知能力键应 400，得到 %d", w.Code)
    }
    if w = call(`{}`); w.Code != http.StatusBadRequest {
        t.Fatalf("缺少 permissions 应 400，得到 %d", w.Code)
    }

    w = call(`{"permissions":null}`)
    if w.Code != http.StatusOK {
        t.Fatalf("恢复默认应成功，得到 %d %s", w.Code, w.Body.String())
    }
    if err := db.Where("book_id = ? AND user_id = ?", 1, 2).First(&saved).Error; err != nil {
        t.Fatalf("read member: %v", err)
    }
    if _, override := permissionsOverride(saved.Permissions); override {
        t.Fatalf("传 null 后应清空单独勾选，实际 %q", saved.Permissions)
    }
    if !memberHasPermission(saved, PermImageDelete) {
        t.Fatalf("恢复默认后监护人应重新全勾")
    }
}

func TestBookDTOCarriesEffectivePermissions(t *testing.T) {
    db := setupPermissionDB(t)
    book := newPermissionBook(t, db)
    if err := db.Create(&BookMember{BookID: 1, UserID: 2, Role: RoleMember, IsCareRecipient: true, Permissions: encodePermissions([]string{PermTodayView})}).Error; err != nil {
        t.Fatalf("create member: %v", err)
    }
    dto := bookToDTO(db, book, RoleMember, 2)
    if len(dto.Permissions) != 1 || dto.Permissions[0] != PermTodayView {
        t.Fatalf("用药本应下发查看者的有效勾选，实际 %v", dto.Permissions)
    }
    if dto.Role != RoleMember || !dto.IsCareRecipient {
        t.Fatalf("DTO 的角色与用药人标记不对：%s / %v", dto.Role, dto.IsCareRecipient)
    }
}
