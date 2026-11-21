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
    // KeywordSet 表示这个机器人渠道还配着旧的「自定义关键词」——应用已不再在消息里补前缀，
    // 需要用户改用加签或 IP 白名单，否则机器人会拒收。
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

func channelToDTO(ch NotificationChannel) ChannelDTO {
    return ChannelDTO{ID: ch.ID, Type: ch.Type, Name: ch.Name, Enabled: ch.Enabled, Status: ch.Status, TargetMasked: ch.TargetMasked, LastTestedAt: ch.LastTestedAt, LastErrorCode: ch.LastErrorCode, LastErrorAt: ch.LastErrorAt, CreatedAt: ch.CreatedAt}
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
            normalized, masked, err := validateChannelConfig(ch.Type, in.Config)
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
