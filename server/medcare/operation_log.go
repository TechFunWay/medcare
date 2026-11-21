package medcare

import (
    "bytes"
    "encoding/json"
    "io"
    "net/http"
    "strings"
    "time"

    "smallgo/server/logger"
    "smallgo/server/response"
    "smallgo/server/utils"

    "github.com/gin-gonic/gin"
    "gorm.io/gorm"
)

// 写操作对应的中文动作名，按「方法 + 规范化路径」匹配。
// 路径里的 :bookId 之后的部分已去掉，参数段统一写成 :p。
var operationActionLabels = map[string]string{
    "PUT /":                           "修改用药本",
    "POST /archive":                   "归档用药本",
    "POST /restore":                   "恢复用药本",
    "DELETE /":                        "删除用药本",
    "POST /transfer":                  "转交管理权限",
    "PUT /members/me/notifications":   "修改自己的提醒设置",
    "PUT /members/:p":                 "修改成员角色",
    "DELETE /members/:p":              "移除成员",
    "PUT /members/care-recipient":     "指定用药人",
    "POST /leave":                     "退出用药本",
    "POST /invites":                   "生成邀请",
    "DELETE /invites/:p":              "撤销邀请",
    "POST /medicines":                 "新增药品",
    "POST /medicines/import":          "批量导入药品",
    "PUT /medicines/:p":               "修改药品",
    "DELETE /medicines/:p":            "归档药品",
    "POST /medicines/:p/restore":      "恢复药品",
    "POST /medicines/:p/stock-in":     "药品入库",
    "POST /medicines/:p/stock-adjust": "库存调整",
    "POST /medicines/:p/stocktake":    "库存盘点",
    "POST /medicines/:p/transfer":     "转移药品到其它用药本",
    "POST /locations":                 "新增地点",
    "PUT /locations/:p":               "修改地点",
    "DELETE /locations/:p":            "归档地点",
    "POST /locations/:p/restore":      "恢复地点",
    "POST /plans":                     "新增用药计划",
    "PUT /plans/:p":                   "修改用药计划",
    "DELETE /plans/:p":                "删除用药计划",
    "POST /plans/:p/pause":            "暂停用药计划",
    "POST /plans/:p/resume":           "恢复用药计划",
    "POST /plans/:p/end":              "结束用药计划",
    "POST /plans/:p/transfer":         "转移用药计划到其它用药本",
    "POST /dose-events/:p/complete":   "完成用药",
    "POST /dose-events/:p/undo":       "撤销完成",
    "POST /dose-events/:p/miss":       "标记漏服",
    "POST /dose-events/:p/adjust":     "用药调整",
    "POST /dose-events/:p/snooze":     "稍后提醒",
    "POST /dose-events/:p/remind":     "提醒用药",
    "POST /dose-events/:p/location":   "修改用药地点",
    "POST /dose-events/:p/note":       "修改用药备注",
    "POST /images":                    "上传图片",
    "PUT /images/:p":                  "修改图片信息",
    "DELETE /images/:p":               "删除图片",
    "POST /prescriptions/recognize":   "识别处方单",
    "POST /prescriptions/:p/confirm":  "确认录入处方",
    "POST /prescriptions/:p/retry":    "重新识别处方",
    "DELETE /prescriptions/:p":        "删除处方记录",
}

func operationTargetType(rel string) string {
    seg := strings.Split(strings.TrimPrefix(rel, "/"), "/")
    if len(seg) == 0 || seg[0] == "" {
        return "book"
    }
    switch seg[0] {
    case "medicines":
        return "medicine"
    case "plans":
        return "plan"
    case "dose-events":
        return "dose_event"
    case "locations":
        return "location"
    case "members":
        return "member"
    case "invites":
        return "invite"
    case "prescriptions":
        return "prescription"
    }
    return seg[0]
}

func normalizeRoutePattern(fullPath string) string {
    idx := strings.Index(fullPath, "/books/:bookId")
    if idx < 0 {
        return fullPath
    }
    rel := fullPath[idx+len("/books/:bookId"):]
    if rel == "" {
        return "/"
    }
    parts := strings.Split(rel, "/")
    for i, p := range parts {
        if strings.HasPrefix(p, ":") {
            parts[i] = ":p"
        }
    }
    return strings.Join(parts, "/")
}

func operationActionLabel(method, rel string) string {
    if label, ok := operationActionLabels[method+" "+rel]; ok {
        return label
    }
    switch method {
    case http.MethodPost:
        return "新增"
    case http.MethodPut, http.MethodPatch:
        return "修改"
    case http.MethodDelete:
        return "删除"
    }
    return "操作"
}

// sanitizeBody 截取请求体作为操作摘要，密钥类字段一律打码。
func sanitizeBody(raw []byte) string {
    if len(raw) == 0 {
        return ""
    }
    lower := strings.ToLower(string(raw))
    for _, key := range []string{"password", "api_key", "apikey", "secret", "token", "credential"} {
        if strings.Contains(lower, key) {
            return "[含敏感字段，已省略]"
        }
    }
    var compact bytes.Buffer
    if err := json.Compact(&compact, raw); err == nil {
        raw = compact.Bytes()
    }
    s := string(raw)
    return trimRunes(s, 300)
}

// bookOperationLogger 记录所有成功的写操作到用药本操作日志，作为审计跟踪。
func bookOperationLogger(db *gorm.DB) gin.HandlerFunc {
    return func(c *gin.Context) {
        method := c.Request.Method
        mutating := method == http.MethodPost || method == http.MethodPut || method == http.MethodPatch || method == http.MethodDelete
        var detail string
        if mutating && c.Request.Body != nil && c.Request.ContentLength > 0 && c.Request.ContentLength <= 8192 && !strings.HasPrefix(c.ContentType(), "multipart/") {
            if raw, err := c.GetRawData(); err == nil {
                c.Request.Body = io.NopCloser(bytes.NewBuffer(raw))
                detail = sanitizeBody(raw)
            }
        }
        c.Next()
        if !mutating {
            return
        }
        if c.Writer.Status() < http.StatusOK || c.Writer.Status() >= http.StatusMultipleChoices {
            return
        }
        userID := c.GetUint("userID")
        if userID == 0 {
            return
        }
        bookID := contextBookID(c)
        if bookID == 0 {
            return
        }
        rel := normalizeRoutePattern(c.FullPath())
        targetID := bookID
        for _, name := range []string{"id", "userId", "inviteId"} {
            if v := c.Param(name); v != "" {
                if n := utils.Atoi(v, 0); n > 0 {
                    targetID = uint(n)
                    break
                }
            }
        }
        entry := OperationLog{
            BookID:      bookID,
            ActorUserID: userID,
            ActorName:   c.GetString("username"),
            Action:      operationActionLabel(method, rel),
            Method:      method,
            TargetType:  operationTargetType(rel),
            TargetID:    targetID,
            Detail:      detail,
            IP:          c.ClientIP(),
            CreatedAt:   nowUTC(),
        }
        if err := db.Create(&entry).Error; err != nil {
            logger.Error("medcare: write operation log: %v", err)
        }
    }
}

type operationLogDTO struct {
    ID          uint      `json:"id"`
    ActorUserID uint      `json:"actor_user_id"`
    ActorName   string    `json:"actor_name"`
    Action      string    `json:"action"`
    Method      string    `json:"method"`
    TargetType  string    `json:"target_type"`
    TargetID    uint      `json:"target_id"`
    Detail      string    `json:"detail"`
    IP          string    `json:"ip"`
    CreatedAt   time.Time `json:"created_at"`
}

// handleListOperationLogs 用药本级操作日志，支持按动作类别、操作人、目标类型与时间筛选。
func handleListOperationLogs(db *gorm.DB) gin.HandlerFunc {
    return func(c *gin.Context) {
        bookID := contextBookID(c)
        page := utils.Atoi(c.Query("page"), 1)
        pageSize := utils.Atoi(c.Query("pageSize"), 20)
        if page < 1 {
            page = 1
        }
        if pageSize <= 0 || pageSize > 100 {
            pageSize = 20
        }
        q := db.Model(&OperationLog{}).Where("book_id = ?", bookID)
        if m := strings.TrimSpace(c.Query("method")); m != "" {
            q = q.Where("method = ?", m)
        }
        if a := utils.Atoi(c.Query("actor_user_id"), 0); a > 0 {
            q = q.Where("actor_user_id = ?", a)
        }
        if t := strings.TrimSpace(c.Query("target_type")); t != "" {
            q = q.Where("target_type = ?", t)
        }
        if from := strings.TrimSpace(c.Query("date_from")); from != "" {
            if t, err := time.Parse("2006-01-02", from); err == nil {
                q = q.Where("created_at >= ?", t)
            }
        }
        if to := strings.TrimSpace(c.Query("date_to")); to != "" {
            if t, err := time.Parse("2006-01-02", to); err == nil {
                q = q.Where("created_at < ?", t.AddDate(0, 0, 1))
            }
        }
        var total int64
        if err := q.Count(&total).Error; err != nil {
            response.ErrorInternal(c, "读取操作日志失败")
            return
        }
        var rows []OperationLog
        if err := q.Order("created_at DESC, id DESC").Offset(utils.Offset(page, pageSize)).Limit(pageSize).Find(&rows).Error; err != nil {
            response.ErrorInternal(c, "读取操作日志失败")
            return
        }
        items := make([]operationLogDTO, 0, len(rows))
        for _, r := range rows {
            actor := r.ActorName
            if actor == "" {
                actor = getUserName(db, r.ActorUserID)
            }
            items = append(items, operationLogDTO{
                ID: r.ID, ActorUserID: r.ActorUserID, ActorName: actor, Action: r.Action,
                Method: r.Method, TargetType: r.TargetType, TargetID: r.TargetID,
                Detail: r.Detail, IP: r.IP, CreatedAt: r.CreatedAt,
            })
        }
        response.SuccessPage(c, items, total, page, pageSize)
    }
}
