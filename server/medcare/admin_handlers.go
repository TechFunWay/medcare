package medcare

import (
    "context"
    "encoding/json"
    "errors"
    "net"
    "net/url"
    "strings"
    "time"

    "smallgo/server/response"
    "smallgo/server/utils"

    "github.com/gin-gonic/gin"
    "gorm.io/gorm"
)

func ensureSystemConfigs(db *gorm.DB) {
    var nc SystemNotificationConfig
    if err := db.First(&nc, 1).Error; err != nil {
        db.Create(&SystemNotificationConfig{ID: 1, SMTPPort: 587, SMTPSecurity: "starttls", FromName: "用药管理", Enabled: true})
    }
    var ai SystemAIConfig
    if err := db.First(&ai, 1).Error; err != nil {
        db.Create(&SystemAIConfig{ID: 1, ProviderType: "openai_compatible", TimeoutSeconds: 120, MaxImageMB: 10, PromptVersion: "v1"})
    }
    var oc SystemOCRConfig
    if err := db.First(&oc, 1).Error; err != nil {
        db.Create(&SystemOCRConfig{ID: 1, Provider: "baidu"})
    }
}

func handleGetSMTPConfig(db *gorm.DB) gin.HandlerFunc {
    return func(c *gin.Context) {
        ensureSystemConfigs(db)
        var cfg SystemNotificationConfig
        db.First(&cfg, 1)
        response.Success(c, gin.H{
            "smtp_host": cfg.SMTPHost, "smtp_port": cfg.SMTPPort, "smtp_security": cfg.SMTPSecurity,
            "smtp_username": cfg.SMTPUsername, "from_address": cfg.FromAddress, "from_name": cfg.FromName,
            "public_base_url": cfg.PublicBaseURL, "enabled": cfg.Enabled, "has_password": cfg.SMTPPasswordEncrypted != "",
        })
    }
}

func handleSaveSMTPConfig(db *gorm.DB) gin.HandlerFunc {
    return func(c *gin.Context) {
        var in struct {
            SMTPHost      string `json:"smtp_host"`
            SMTPPort      int    `json:"smtp_port"`
            SMTPSecurity  string `json:"smtp_security"`
            SMTPUsername  string `json:"smtp_username"`
            SMTPPassword  string `json:"smtp_password"`
            FromAddress   string `json:"from_address"`
            FromName      string `json:"from_name"`
            PublicBaseURL string `json:"public_base_url"`
            Enabled       *bool  `json:"enabled"`
        }
        if err := c.ShouldBindJSON(&in); err != nil {
            response.ErrorBadRequest(c, "参数无效")
            return
        }
        ensureSystemConfigs(db)
        var cfg SystemNotificationConfig
        db.First(&cfg, 1)
        cfg.SMTPHost = strings.TrimSpace(in.SMTPHost)
        cfg.SMTPPort = in.SMTPPort
        if cfg.SMTPPort == 0 {
            cfg.SMTPPort = 587
        }
        cfg.SMTPSecurity = in.SMTPSecurity
        if cfg.SMTPSecurity == "" {
            cfg.SMTPSecurity = "starttls"
        }
        cfg.SMTPUsername = strings.TrimSpace(in.SMTPUsername)
        cfg.FromAddress = strings.TrimSpace(in.FromAddress)
        cfg.FromName = strings.TrimSpace(in.FromName)
        if cfg.FromName == "" {
            cfg.FromName = "用药管理"
        }
        cfg.PublicBaseURL = strings.TrimRight(strings.TrimSpace(in.PublicBaseURL), "/")
        if in.Enabled != nil {
            cfg.Enabled = *in.Enabled
        }
        if in.SMTPPassword != "" {
            encrypted, err := encryptString(db, in.SMTPPassword)
            if err != nil {
                response.ErrorInternal(c, "加密 SMTP 密码失败")
                return
            }
            cfg.SMTPPasswordEncrypted = encrypted
        }
        cfg.UpdatedAt = nowUTC()
        if err := db.Save(&cfg).Error; err != nil {
            response.ErrorInternal(c, "保存 SMTP 配置失败")
            return
        }
        response.Success(c, gin.H{"ok": true})
    }
}

func handleTestSMTPConfig(db *gorm.DB) gin.HandlerFunc {
    return func(c *gin.Context) {
        var in struct {
            To string `json:"to"`
        }
        _ = c.ShouldBindJSON(&in)
        ensureSystemConfigs(db)
        var cfg SystemNotificationConfig
        db.First(&cfg, 1)
        to := strings.TrimSpace(in.To)
        if to == "" {
            to = cfg.FromAddress
        }
        if to == "" {
            response.ErrorBadRequest(c, "请填写收件地址")
            return
        }
        raw, _ := json.Marshal(emailChannelConfig{Address: to})
        encrypted, err := encryptString(db, string(raw))
        if err != nil {
            response.ErrorInternal(c, "加密测试配置失败")
            return
        }
        temp := NotificationChannel{Type: ChannelEmail, ConfigEncrypted: encrypted, UserID: currentUserID(c)}
        msg := notificationMessage{Title: "用药管理 SMTP 测试", Lines: []string{"如果你收到这封邮件，SMTP 配置成功。"}, BookName: "用药管理"}
        if _, err := sendChannelMessage(c.Request.Context(), db, temp, msg); err != nil {
            response.ErrorBadRequest(c, err.Error())
            return
        }
        response.Success(c, gin.H{"sent": true})
    }
}

func handleGetAIConfig(db *gorm.DB) gin.HandlerFunc {
    return func(c *gin.Context) {
        ensureSystemConfigs(db)
        var cfg SystemAIConfig
        db.First(&cfg, 1)
        response.Success(c, gin.H{
            "enabled": cfg.Enabled, "provider_type": cfg.ProviderType, "provider_name": cfg.ProviderName, "base_url": cfg.BaseURL,
            "model": cfg.Model, "timeout_seconds": cfg.TimeoutSeconds, "max_image_mb": cfg.MaxImageMB,
            "has_key": cfg.APIKeyEncrypted != "", "prompt_version": cfg.PromptVersion,
        })
    }
}

func handleSaveAIConfig(db *gorm.DB) gin.HandlerFunc {
    return func(c *gin.Context) {
        var in struct {
            Enabled        *bool  `json:"enabled"`
            ProviderType   string `json:"provider_type"`
            ProviderName   *string `json:"provider_name"`
            BaseURL        string `json:"base_url"`
            APIKey         string `json:"api_key"`
            Model          string `json:"model"`
            TimeoutSeconds int    `json:"timeout_seconds"`
            MaxImageMB     int    `json:"max_image_mb"`
            PromptVersion  string `json:"prompt_version"`
        }
        if err := c.ShouldBindJSON(&in); err != nil {
            response.ErrorBadRequest(c, "参数无效")
            return
        }
        ensureSystemConfigs(db)
        var cfg SystemAIConfig
        db.First(&cfg, 1)
        if in.Enabled != nil {
            cfg.Enabled = *in.Enabled
        }
        if in.ProviderType != "" {
            cfg.ProviderType = in.ProviderType
        }
        if in.ProviderName != nil {
            name := strings.TrimSpace(*in.ProviderName)
            if len([]rune(name)) > 60 {
                name = string([]rune(name)[:60])
            }
            cfg.ProviderName = name
        }
        if err := validateAIBaseURL(in.BaseURL); err != nil {
            response.ErrorBadRequest(c, err.Error())
            return
        }
        cfg.BaseURL = strings.TrimRight(strings.TrimSpace(in.BaseURL), "/")
        if in.Model != "" {
            cfg.Model = strings.TrimSpace(in.Model)
        }
        if in.TimeoutSeconds > 0 {
            if in.TimeoutSeconds < 30 || in.TimeoutSeconds > 300 {
                response.ErrorBadRequest(c, "超时时间应为 30–300 秒")
                return
            }
            cfg.TimeoutSeconds = in.TimeoutSeconds
        }
        if in.MaxImageMB > 0 {
            cfg.MaxImageMB = in.MaxImageMB
        }
        if in.PromptVersion != "" {
            cfg.PromptVersion = in.PromptVersion
        }
        if in.APIKey != "" {
            encrypted, err := encryptString(db, in.APIKey)
            if err != nil {
                response.ErrorInternal(c, "加密 AI Key 失败")
                return
            }
            cfg.APIKeyEncrypted = encrypted
        }
        cfg.UpdatedAt = nowUTC()
        if err := db.Save(&cfg).Error; err != nil {
            response.ErrorInternal(c, "保存 AI 配置失败")
            return
        }
        response.Success(c, gin.H{"ok": true})
    }
}

// aiConfigOverride 是「测试连接」「获取模型列表」带的临时参数：
// 用户还没保存也能直接拿页面上填的值来测，不必先保存再回来点一次。
type aiConfigOverride struct {
    BaseURL        string `json:"base_url"`
    APIKey         string `json:"api_key"`
    Model          string `json:"model"`
    TimeoutSeconds int    `json:"timeout_seconds"`
}

// validateAIBaseURL 校验 AI 服务地址。这里不封内网地址——本机 Ollama、局域网
// LM Studio 都是合法用法；只挡住不该出现的协议、URL 内嵌凭据，以及云元数据地址
// （那类地址一旦被填进来，服务端就会把请求连到云主机的凭据端点）。
func validateAIBaseURL(raw string) error {
    v := strings.TrimSpace(raw)
    if v == "" {
        return nil
    }
    if len(v) > 300 {
        return errors.New("AI 服务地址过长")
    }
    u, err := url.Parse(v)
    if err != nil {
        return errors.New("AI 服务地址格式不正确")
    }
    if u.Scheme != "http" && u.Scheme != "https" {
        return errors.New("AI 服务地址只支持 http 或 https")
    }
    if u.Host == "" {
        return errors.New("AI 服务地址缺少主机名")
    }
    if u.User != nil {
        return errors.New("AI 服务地址不要内嵌账号密码，请填在 API Key 里")
    }
    if ip := net.ParseIP(u.Hostname()); ip != nil && ip.IsLinkLocalUnicast() {
        return errors.New("AI 服务地址不能指向链路本地地址")
    }
    if u.Hostname() == "metadata.google.internal" {
        return errors.New("AI 服务地址不能指向云元数据服务")
    }
    return nil
}

// effectiveAIConfig 把请求里的临时值叠加到已保存配置上：
// 临时值为空就回落到库里的值，API Key 没填则沿用已加密保存的 Key。
func effectiveAIConfig(db *gorm.DB, cfg SystemAIConfig, in aiConfigOverride) (SystemAIConfig, error) {
    if v := strings.TrimSpace(in.BaseURL); v != "" {
        if err := validateAIBaseURL(v); err != nil {
            return cfg, err
        }
        cfg.BaseURL = strings.TrimRight(v, "/")
    }
    if v := strings.TrimSpace(in.Model); v != "" {
        cfg.Model = v
    }
    if in.TimeoutSeconds >= 30 && in.TimeoutSeconds <= 300 {
        cfg.TimeoutSeconds = in.TimeoutSeconds
    }
    if v := strings.TrimSpace(in.APIKey); v != "" {
        encrypted, err := encryptString(db, v)
        if err != nil {
            return cfg, errors.New("加密 AI Key 失败")
        }
        cfg.APIKeyEncrypted = encrypted
    }
    return cfg, nil
}

func handleTestAIConfig(db *gorm.DB) gin.HandlerFunc {
    return func(c *gin.Context) {
        ensureSystemConfigs(db)
        var in aiConfigOverride
        // 允许空 body：不带参数时按已保存配置测试。
        _ = c.ShouldBindJSON(&in)
        var stored SystemAIConfig
        db.First(&stored, 1)
        cfg, err := effectiveAIConfig(db, stored, in)
        if err != nil {
            response.ErrorInternal(c, err.Error())
            return
        }
        // 测试只校验「填没填」，不看启用开关：没启用也能先验证连通性。
        if cfg.BaseURL == "" {
            response.ErrorBadRequest(c, "请先填写 Base URL")
            return
        }
        if cfg.Model == "" {
            response.ErrorBadRequest(c, "请先填写模型名称")
            return
        }
        if cfg.APIKeyEncrypted == "" {
            response.ErrorBadRequest(c, "请先填写 API Key")
            return
        }
        if err := testAIConnection(c.Request.Context(), db, cfg); err != nil {
            response.ErrorBadRequest(c, err.Error())
            return
        }
        response.Success(c, gin.H{"ok": true, "model": cfg.Model})
    }
}

func handleListAIModels(db *gorm.DB) gin.HandlerFunc {
    return func(c *gin.Context) {
        ensureSystemConfigs(db)
        var in aiConfigOverride
        _ = c.ShouldBindJSON(&in)
        var stored SystemAIConfig
        db.First(&stored, 1)
        cfg, err := effectiveAIConfig(db, stored, in)
        if err != nil {
            response.ErrorInternal(c, err.Error())
            return
        }
        if cfg.BaseURL == "" {
            response.ErrorBadRequest(c, "请先填写 Base URL")
            return
        }
        if cfg.APIKeyEncrypted == "" {
            response.ErrorBadRequest(c, "请先填写 API Key")
            return
        }
        key, err := decryptString(db, cfg.APIKeyEncrypted)
        if err != nil {
            response.ErrorBadRequest(c, "已保存的 API Key 无法读取，请重新填写")
            return
        }
        models, err := listAIModels(c.Request.Context(), cfg.BaseURL, key, 30)
        if err != nil {
            response.ErrorBadRequest(c, err.Error())
            return
        }
        response.Success(c, gin.H{"models": models, "count": len(models)})
    }
}

func handleAdminDeliveries(db *gorm.DB) gin.HandlerFunc {
    return func(c *gin.Context) {
        page := utils.Atoi(c.Query("page"), 1)
        pageSize := utils.Atoi(c.Query("pageSize"), 50)
        if pageSize > 200 {
            pageSize = 200
        }
        q := db.Model(&NotificationJob{})
        if s := c.Query("status"); s != "" {
            q = q.Where("status = ?", s)
        }
        if t := c.Query("channel_type"); t != "" {
            q = q.Where("channel_type = ?", t)
        }
        var total int64
        q.Count(&total)
        var rows []NotificationJob
        q.Order("created_at DESC").Offset(utils.Offset(page, pageSize)).Limit(pageSize).Find(&rows)
        items := make([]DeliveryDTO, 0, len(rows))
        for _, j := range rows {
            items = append(items, DeliveryDTO{ID: j.ID, BookID: j.BookID, BookName: bookName(db, j.BookID), ChannelType: j.ChannelType, EventID: j.EventID, Kind: j.Kind, Status: j.Status, Attempts: j.Attempts, RunAt: j.RunAt, LastError: j.LastErrorMessage, CreatedAt: j.CreatedAt})
        }
        response.SuccessPage(c, items, total, page, pageSize)
    }
}

func handleAdminRetryDelivery(db *gorm.DB) gin.HandlerFunc {
    return func(c *gin.Context) {
        id, ok := parseUintParam(c, "id")
        if !ok {
            return
        }
        now := nowUTC()
        // 与用户侧一致：卡死（锁已过期）的 sending 也允许管理员手动重试。
        result := db.Model(&NotificationJob{}).Where("id = ? AND status IN ?", id, []string{JobFailed, JobCanceled, JobSending}).
            Where("status <> ? OR locked_at IS NULL OR locked_at < ?", JobSending, now.Add(-staleSendingAfter)).
            Updates(map[string]interface{}{"status": JobPending, "run_at": now, "attempts": 0, "next_retry_at": nil, "locked_at": nil, "worker_id": "", "last_error_code": "", "last_error_message": ""})
        if result.Error != nil {
            response.ErrorInternal(c, "重试失败")
            return
        }
        if result.RowsAffected == 0 {
            response.ErrorBadRequest(c, "该投递不能重试")
            return
        }
        response.Success(c, gin.H{"ok": true})
    }
}

func handleAdminHealth(db *gorm.DB) gin.HandlerFunc {
    return func(c *gin.Context) {
        var pending, failed int64
        db.Model(&NotificationJob{}).Where("status IN ?", []string{JobPending, JobRetry}).Count(&pending)
        db.Model(&NotificationJob{}).Where("status = ?", JobFailed).Count(&failed)
        response.Success(c, gin.H{"pending_jobs": pending, "failed_jobs": failed, "server_time": nowUTC()})
    }
}

// ---------- OCR 辅助（可选）：先读字再让文本模型整理清单 ----------

func handleGetOCRConfig(db *gorm.DB) gin.HandlerFunc {
    return func(c *gin.Context) {
        ensureSystemConfigs(db)
        var oc SystemOCRConfig
        db.First(&oc, 1)
        response.Success(c, gin.H{
            "enabled": oc.Enabled, "provider": oc.Provider, "base_url": oc.BaseURL, "api_key": oc.APIKey,
            "app_id": oc.AppID, "model": oc.Model, "has_secret": oc.SecretKeyEncrypted != "",
        })
    }
}

func handleSaveOCRConfig(db *gorm.DB) gin.HandlerFunc {
    return func(c *gin.Context) {
        var in struct {
            Enabled   *bool  `json:"enabled"`
            Provider  string `json:"provider"`
            BaseURL   string `json:"base_url"`
            APIKey    string `json:"api_key"`
            SecretKey string `json:"secret_key"`
            AppID     string `json:"app_id"`
            Model     string `json:"model"`
        }
        if err := c.ShouldBindJSON(&in); err != nil {
            response.ErrorBadRequest(c, "参数无效")
            return
        }
        ensureSystemConfigs(db)
        var oc SystemOCRConfig
        db.First(&oc, 1)
        if in.Enabled != nil {
            oc.Enabled = *in.Enabled
        }
        if in.Provider != "" {
            switch in.Provider {
            case "baidu", "tencent", "aliyun", "youdao", "deepseek", "custom":
            default:
                response.ErrorBadRequest(c, "不支持的 OCR 服务商")
                return
            }
            oc.Provider = in.Provider
        }
        oc.APIKey = strings.TrimSpace(in.APIKey)
        oc.BaseURL = strings.TrimRight(strings.TrimSpace(in.BaseURL), "/")
        oc.AppID = strings.TrimSpace(in.AppID)
        oc.Model = strings.TrimSpace(in.Model)
        if in.SecretKey != "" {
            enc, err := encryptString(db, in.SecretKey)
            if err != nil {
                response.ErrorInternal(c, "加密 OCR Secret Key 失败")
                return
            }
            oc.SecretKeyEncrypted = enc
        }
        if oc.Enabled {
            if oc.APIKey == "" {
                response.ErrorBadRequest(c, "启用 OCR 辅助需要先填 API Key / 应用 ID")
                return
            }
            if oc.Provider == "custom" {
                if oc.BaseURL == "" || oc.Model == "" {
                    response.ErrorBadRequest(c, "自定义 OCR 需要填写 Base URL 与模型")
                    return
                }
            } else if ocrNeedsSecret(oc.Provider) && oc.SecretKeyEncrypted == "" {
                response.ErrorBadRequest(c, "启用 OCR 辅助需要先填 Secret Key / AccessKeySecret")
                return
            }
        }
        oc.UpdatedAt = nowUTC()
        if err := db.Save(&oc).Error; err != nil {
            response.ErrorInternal(c, "保存 OCR 配置失败")
            return
        }
        response.Success(c, gin.H{"ok": true})
    }
}

// handleTestOCRConfig 只验证双 Key 能换到令牌，不消耗 OCR 识别额度。
func handleTestOCRConfig(db *gorm.DB) gin.HandlerFunc {
    return func(c *gin.Context) {
        ensureSystemConfigs(db)
        var oc SystemOCRConfig
        db.First(&oc, 1)
        // 允许前端带上「还没保存」的配置来测试，填完直接点测试即可（与 AI 测试一致）。
        var in struct {
            Provider  string `json:"provider"`
            BaseURL   string `json:"base_url"`
            APIKey    string `json:"api_key"`
            SecretKey string `json:"secret_key"`
            Model     string `json:"model"`
        }
        _ = c.ShouldBindJSON(&in)
        if v := strings.TrimSpace(in.Provider); v != "" {
            oc.Provider = v
        }
        if v := strings.TrimSpace(in.BaseURL); v != "" {
            oc.BaseURL = v
        }
        if v := strings.TrimSpace(in.APIKey); v != "" {
            oc.APIKey = v
        }
        if v := strings.TrimSpace(in.Model); v != "" {
            oc.Model = v
        }
        plainSecret := strings.TrimSpace(in.SecretKey)
        if oc.APIKey == "" {
            response.ErrorBadRequest(c, "请先填写并保存 API Key")
            return
        }
        if ocrNeedsSecret(oc.Provider) && oc.SecretKeyEncrypted == "" {
            response.ErrorBadRequest(c, "请先填写并保存 Secret Key / 应用密钥")
            return
        }
        if oc.Provider == "custom" && (oc.BaseURL == "" || oc.Model == "") {
            response.ErrorBadRequest(c, "自定义 OCR 需要填写 Base URL 与模型")
            return
        }
        // 百度能直接验双 Key；其余服务商签名要带真实图片才能验，这里只做配置完整性检查，
        // 明确告诉用户用一张单据实测一次，避免把「测不通」误报成配置错误。
        if oc.Provider == "" || oc.Provider == "baidu" {
            sk := plainSecret
            if sk == "" {
                decrypted, err := decryptString(db, oc.SecretKeyEncrypted)
                if err != nil {
                    response.ErrorInternal(c, "OCR Secret Key 解密失败，请重新保存")
                    return
                }
                sk = decrypted
            }
            ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Second)
            defer cancel()
            if _, err := baiduAccessToken(ctx, oc.APIKey, sk); err != nil {
                response.ErrorInternal(c, err.Error())
                return
            }
        }
        response.Success(c, gin.H{"ok": true, "note": "配置检查通过；实际识别效果请用一张单据试一次"})
    }
}

// ocrNeedsSecret 该服务商是否需要「第二个密钥」：
// 百度（Secret Key）、腾讯云（SecretKey）、阿里云（AccessKeySecret）、有道（应用密钥）需要；
// DeepSeek 只认一个 API Key，自定义走 OpenAI 兼容也只认 API Key。
// 之前用「不是 custom 就要」判断，会把 DeepSeek 误判成必填（2026-11-17 用户反馈）。
func ocrNeedsSecret(provider string) bool {
    switch provider {
    case "baidu", "tencent", "aliyun", "youdao":
        return true
    }
    return false
}
