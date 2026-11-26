package medcare

import (
    "encoding/json"
    "errors"
    "net/mail"
    "net/url"
    "strings"
    "time"

    "smallgo/server/response"

    "github.com/gin-gonic/gin"
    "gorm.io/gorm"
)

type ChannelDTO struct {
    ID            uint       `json:"id"`
    Type          string     `json:"type"`
    Name          string     `json:"name"`
    Enabled       bool       `json:"enabled"`
    Status        string     `json:"status"`
    TargetMasked  string     `json:"target_masked"`
    LastTestedAt  *time.Time `json:"last_tested_at,omitempty"`
    LastErrorCode string     `json:"last_error_code,omitempty"`
    LastErrorAt   *time.Time `json:"last_error_at,omitempty"`
    // Keyword 是机器人「自定义关键词」校验用的词（不是机密，可回显）；
    // 应用会把它随消息发出以满足校验，但不会显示在通知正文里。
    Keyword    string    `json:"keyword,omitempty"`
    KeywordSet bool      `json:"keyword_set,omitempty"`
    CreatedAt  time.Time `json:"created_at"`
}

type dingTalkConfig struct {
    Webhook string `json:"webhook"`
    Secret  string `json:"secret"`
    // Keyword 对应钉钉机器人「自定义关键词」安全设置；填写后每条消息都会带上它。
    Keyword string `json:"keyword"`
}

type feishuConfig struct {
    Webhook string `json:"webhook"`
    Secret  string `json:"secret"`
    // Keyword 对应飞书机器人「自定义关键词」安全设置。
    Keyword string `json:"keyword"`
}

type emailChannelConfig struct {
    Address string `json:"address"`
}

// handleGetChannelConfig 返回渠道的可编辑配置（仅本人）。
// Webhook 与收件地址不是密钥，回填后用户才能在编辑时看到并修正；
// 加签密钥仍然不回传，编辑时留空表示不修改。
func handleGetChannelConfig(db *gorm.DB) gin.HandlerFunc {
    return func(c *gin.Context) {
        id, ok := parseUintParam(c, "id")
        if !ok {
            return
        }
        var ch NotificationChannel
        if err := db.Where("id = ? AND user_id = ?", id, currentUserID(c)).First(&ch).Error; err != nil {
            response.ErrorNotFound(c, "通知渠道不存在")
            return
        }
        out := gin.H{"type": ch.Type, "name": ch.Name, "enabled": ch.Enabled}
        plain, err := decryptString(db, ch.ConfigEncrypted)
        if err == nil {
            switch ch.Type {
            case ChannelDingTalk, ChannelFeishu:
                var cfg struct {
                    Webhook string `json:"webhook"`
                    Keyword string `json:"keyword"`
                }
                if json.Unmarshal([]byte(plain), &cfg) == nil {
                    out["webhook"] = cfg.Webhook
                    out["keyword"] = cfg.Keyword
                }
            case ChannelEmail:
                var cfg emailChannelConfig
                if json.Unmarshal([]byte(plain), &cfg) == nil {
                    out["address"] = cfg.Address
                }
            }
        }
        response.Success(c, out)
    }
}

func channelToDTO(ch NotificationChannel) ChannelDTO {
    return ChannelDTO{ID: ch.ID, Type: ch.Type, Name: ch.Name, Enabled: ch.Enabled, Status: ch.Status, TargetMasked: ch.TargetMasked, LastTestedAt: ch.LastTestedAt, LastErrorCode: ch.LastErrorCode, LastErrorAt: ch.LastErrorAt, CreatedAt: ch.CreatedAt}
}

// mergeChannelConfig 把「编辑时提交的部分字段」合并到已保存的配置上。
// 前端不回显 Webhook 与密钥（接口只给掩码），所以编辑时这两项留空表示保持原值；
// 邮箱地址同理：留空就沿用旧地址。
func mergeChannelConfig(db *gorm.DB, ch NotificationChannel, incoming json.RawMessage) (json.RawMessage, error) {
    plain, err := decryptString(db, ch.ConfigEncrypted)
    if err != nil {
        return incoming, nil
    }
    var oldMap, newMap map[string]interface{}
    if json.Unmarshal([]byte(plain), &oldMap) != nil {
        return incoming, nil
    }
    if json.Unmarshal(incoming, &newMap) != nil {
        return nil, errors.New("渠道配置无效")
    }
    for k, v := range newMap {
        s, isStr := v.(string)
        // 空字符串视为「不修改」，保留旧值；非字符串（如布尔）照常覆盖。
        if isStr && strings.TrimSpace(s) == "" {
            continue
        }
        oldMap[k] = v
    }
    merged, err := json.Marshal(oldMap)
    if err != nil {
        return nil, errors.New("渠道配置无效")
    }
    return merged, nil
}

func validateChannelConfig(channelType string, raw json.RawMessage) (interface{}, string, error) {
    switch channelType {
    case ChannelEmail:
        var cfg emailChannelConfig
        if err := json.Unmarshal(raw, &cfg); err != nil {
            return nil, "", errors.New("邮箱配置无效")
        }
        addr := strings.TrimSpace(cfg.Address)
        parsed, err := mail.ParseAddress(addr)
        if err != nil || parsed.Address != addr {
            return nil, "", errors.New("请输入有效的邮箱地址")
        }
        cfg.Address = addr
        return cfg, maskEmail(addr), nil
    case ChannelDingTalk:
        var cfg dingTalkConfig
        if err := json.Unmarshal(raw, &cfg); err != nil {
            return nil, "", errors.New("钉钉配置无效")
        }
        normalized, err := normalizeWebhook(cfg.Webhook, "oapi.dingtalk.com", "/robot/send", true)
        if err != nil {
            return nil, "", err
        }
        cfg.Webhook = normalized
        cfg.Keyword = strings.TrimSpace(cfg.Keyword)
        if len([]rune(cfg.Keyword)) > 20 {
            return nil, "", errors.New("关键词不能超过 20 个字符")
        }
        return cfg, maskWebhook(normalized), nil
    case ChannelFeishu:
        var cfg feishuConfig
        if err := json.Unmarshal(raw, &cfg); err != nil {
            return nil, "", errors.New("飞书配置无效")
        }
        normalized, err := normalizeWebhook(cfg.Webhook, "open.feishu.cn", "/open-apis/bot/v2/hook/", false)
        if err != nil {
            return nil, "", err
        }
        cfg.Webhook = normalized
        cfg.Keyword = strings.TrimSpace(cfg.Keyword)
        if len([]rune(cfg.Keyword)) > 20 {
            return nil, "", errors.New("关键词不能超过 20 个字符")
        }
        return cfg, maskWebhook(normalized), nil
    default:
        return nil, "", ErrChannelInvalid
    }
}

func normalizeWebhook(raw, host, pathPrefix string, onlyAccessToken bool) (string, error) {
    u, err := url.ParseRequestURI(strings.TrimSpace(raw))
    if err != nil || !strings.EqualFold(u.Scheme, "https") || !strings.EqualFold(u.Hostname(), host) || u.User != nil || u.Fragment != "" {
        return "", errors.New("Webhook 地址无效，请粘贴官方机器人地址")
    }
    if !strings.HasPrefix(u.Path, pathPrefix) {
        return "", errors.New("Webhook 路径不是官方机器人地址")
    }
    if onlyAccessToken {
        q := u.Query()
        if len(q) != 1 || len(q["access_token"]) != 1 || strings.TrimSpace(q["access_token"][0]) == "" {
            return "", errors.New("钉钉 Webhook 只能包含 access_token")
        }
        accessToken := q["access_token"][0]
        u.RawQuery = "access_token=" + url.QueryEscape(accessToken)
    }
    if len(u.String()) > 1000 {
        return "", errors.New("Webhook 地址过长")
    }
    return u.String(), nil
}

func handleListChannels(db *gorm.DB) gin.HandlerFunc {
    return func(c *gin.Context) {
        var rows []NotificationChannel
        if err := db.Where("user_id = ? AND archived_at IS NULL", currentUserID(c)).Order("created_at ASC").Find(&rows).Error; err != nil {
            response.ErrorInternal(c, "读取通知渠道失败")
            return
        }
        items := make([]ChannelDTO, 0, len(rows))
        for _, ch := range rows {
            dto := channelToDTO(ch)
            if ch.Type == ChannelDingTalk || ch.Type == ChannelFeishu {
                var cfg struct {
                    Keyword string `json:"keyword"`
                }
                if plain, err := decryptString(db, ch.ConfigEncrypted); err == nil {
                    if json.Unmarshal([]byte(plain), &cfg) == nil && strings.TrimSpace(cfg.Keyword) != "" {
                        dto.Keyword = strings.TrimSpace(cfg.Keyword)
                        dto.KeywordSet = true
                    }
                }
            }
            items = append(items, dto)
        }
        response.Success(c, items)
    }
}

func handleCreateChannel(db *gorm.DB) gin.HandlerFunc {
    return func(c *gin.Context) {
        var in struct {
            Type    string          `json:"type"`
            Name    string          `json:"name"`
            Enabled *bool           `json:"enabled"`
            Config  json.RawMessage `json:"config"`
        }
        if err := c.ShouldBindJSON(&in); err != nil {
            response.ErrorBadRequest(c, "参数无效")
            return
        }
        in.Type = strings.ToLower(strings.TrimSpace(in.Type))
        if in.Type != ChannelEmail && in.Type != ChannelDingTalk && in.Type != ChannelFeishu {
            response.ErrorBadRequest(c, "不支持的通知渠道")
            return
        }
        if strings.TrimSpace(in.Name) == "" {
            response.ErrorBadRequest(c, "请填写渠道名称")
            return
        }
        normalized, masked, err := validateChannelConfig(in.Type, in.Config)
        if err != nil {
            response.ErrorBadRequest(c, err.Error())
            return
        }
        raw, _ := json.Marshal(normalized)
        encrypted, err := encryptString(db, string(raw))
        if err != nil {
            response.ErrorInternal(c, "加密渠道配置失败")
            return
        }
        enabled := true
        if in.Enabled != nil {
            enabled = *in.Enabled
        }
        now := nowUTC()
        ch := NotificationChannel{UserID: currentUserID(c), Type: in.Type, Name: strings.TrimSpace(in.Name), Enabled: enabled, ConfigEncrypted: encrypted, TargetMasked: masked, Status: "active", Version: 1, CreatedAt: now, UpdatedAt: now}
        if err := db.Create(&ch).Error; err != nil {
            response.ErrorInternal(c, "保存通知渠道失败")
            return
        }
        response.Success(c, channelToDTO(ch))
    }
}

func handleUpdateChannel(db *gorm.DB) gin.HandlerFunc {
    return func(c *gin.Context) {
        id, ok := parseUintParam(c, "id")
        if !ok {
            return
        }
        var ch NotificationChannel
        if err := db.Where("id = ? AND user_id = ?", id, currentUserID(c)).First(&ch).Error; err != nil {
            response.ErrorNotFound(c, "通知渠道不存在")
            return
        }
        var in struct {
            Name    string          `json:"name"`
            Enabled *bool           `json:"enabled"`
            Config  json.RawMessage `json:"config"`
        }
        if err := c.ShouldBindJSON(&in); err != nil {
            response.ErrorBadRequest(c, "参数无效")
            return
        }
        if strings.TrimSpace(in.Name) != "" {
            ch.Name = strings.TrimSpace(in.Name)
        }
        if in.Enabled != nil {
            ch.Enabled = *in.Enabled
        }
        if len(in.Config) > 0 {
            // 编辑时允许只提交部分字段（例如只改加签密钥、不动 Webhook）：
            // 先解出旧配置，用新提交里的非空字段覆盖，再整体校验与加密。
            merged, err := mergeChannelConfig(db, ch, in.Config)
            if err != nil {
                response.ErrorBadRequest(c, err.Error())
                return
            }
            normalized, masked, err := validateChannelConfig(ch.Type, merged)
            if err != nil {
                response.ErrorBadRequest(c, err.Error())
                return
            }
            raw, _ := json.Marshal(normalized)
            encrypted, err := encryptString(db, string(raw))
            if err != nil {
                response.ErrorInternal(c, "加密渠道配置失败")
                return
            }
            ch.ConfigEncrypted = encrypted
            ch.TargetMasked = masked
        }
        ch.Status = "active"
        ch.UpdatedAt = nowUTC()
        ch.Version++
        if err := db.Save(&ch).Error; err != nil {
            response.ErrorInternal(c, "保存通知渠道失败")
            return
        }
        response.Success(c, channelToDTO(ch))
    }
}

func handleDeleteChannel(db *gorm.DB) gin.HandlerFunc {
    return func(c *gin.Context) {
        id, ok := parseUintParam(c, "id")
        if !ok {
            return
        }
        now := nowUTC()
        result := db.Model(&NotificationChannel{}).Where("id = ? AND user_id = ?", id, currentUserID(c)).Updates(map[string]interface{}{"archived_at": &now, "enabled": false, "status": "disabled"})
        if result.Error != nil {
            response.ErrorInternal(c, "删除通知渠道失败")
            return
        }
        if result.RowsAffected == 0 {
            response.ErrorNotFound(c, "通知渠道不存在")
            return
        }
        cancelChannelJobs(db, id, "渠道已删除")
        response.Success(c, gin.H{"deleted": true})
    }
}

func handleTestChannel(db *gorm.DB) gin.HandlerFunc {
    return func(c *gin.Context) {
        id, ok := parseUintParam(c, "id")
        if !ok {
            return
        }
        var ch NotificationChannel
        if err := db.Where("id = ? AND user_id = ?", id, currentUserID(c)).First(&ch).Error; err != nil {
            response.ErrorNotFound(c, "通知渠道不存在")
            return
        }
        bookName := "用药管理"
        if pref := getPreferences(db, currentUserID(c)); pref.LastBookID != nil {
            if b, err := getBook(db, *pref.LastBookID); err == nil {
                bookName = b.Name
            }
        }
        msg := notificationMessage{Title: "用药管理测试提醒", Lines: []string{"这是一条测试消息。", "如果你收到它，说明渠道配置成功。"}, BookName: bookName}
        _, err := sendChannelMessage(c.Request.Context(), db, ch, msg)
        now := nowUTC()
        if err != nil {
            code := "SEND_FAILED"
            var de *deliveryError
            if errors.As(err, &de) {
                code = de.Code
            }
            db.Model(&ch).Updates(map[string]interface{}{"status": "error", "last_error_code": code, "last_error_at": &now, "last_tested_at": &now})
            response.ErrorBadRequest(c, err.Error())
            return
        }
        db.Model(&ch).Updates(map[string]interface{}{"status": "active", "last_error_code": "", "last_error_at": nil, "last_tested_at": &now, "verified_at": &now})
        response.Success(c, gin.H{"sent": true})
    }
}
