package medcare

import (
    "bytes"
    "context"
    "crypto/hmac"
    "crypto/sha256"
    "crypto/tls"
    "encoding/base64"
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    "net/smtp"
    "net/url"
    "strings"
    "time"

    "gorm.io/gorm"
)

type notificationMessage struct {
    Title    string
    Lines    []string
    Link     string
    BookName string
}

type deliveryError struct {
    Code      string
    Message   string
    Permanent bool
}

func (e *deliveryError) Error() string { return e.Message }

func messageText(msg notificationMessage) string {
    var b strings.Builder
    if msg.BookName != "" {
        b.WriteString("【用药管理】")
        b.WriteString(msg.BookName)
        b.WriteString("\n")
    }
    b.WriteString(msg.Title)
    b.WriteString("\n")
    for _, line := range msg.Lines {
        b.WriteString("- ")
        b.WriteString(line)
        b.WriteString("\n")
    }
    // 通知正文不再拼接访问路径：对方看到 /medicines、/today 这类路径既看不懂也不好看，
    // 需要点进来看的用邮件里的「打开用药管理」按钮即可。
    return strings.TrimSpace(b.String())
}

func decryptChannelConfig(db *gorm.DB, ch NotificationChannel, target interface{}) error {
    plain, err := decryptString(db, ch.ConfigEncrypted)
    if err != nil {
        return &deliveryError{Code: "CONFIG_DECRYPT_FAILED", Message: "渠道配置无法读取，请重新填写", Permanent: true}
    }
    if err := json.Unmarshal([]byte(plain), target); err != nil {
        return &deliveryError{Code: "CONFIG_INVALID", Message: "渠道配置无效，请重新填写", Permanent: true}
    }
    return nil
}

func sendChannelMessage(ctx context.Context, db *gorm.DB, ch NotificationChannel, msg notificationMessage) (string, error) {
    switch ch.Type {
    case ChannelEmail:
        return sendEmailChannel(ctx, db, ch, msg)
    case ChannelDingTalk:
        return sendDingTalkChannel(ctx, db, ch, msg)
    case ChannelFeishu:
        return sendFeishuChannel(ctx, db, ch, msg)
    default:
        return "", &deliveryError{Code: "CHANNEL_UNSUPPORTED", Message: "不支持的通知渠道", Permanent: true}
    }
}

func systemNotificationConfig(db *gorm.DB) SystemNotificationConfig {
    var cfg SystemNotificationConfig
    if err := db.First(&cfg, 1).Error; err != nil {
        cfg = SystemNotificationConfig{ID: 1, FromName: "用药管理", Enabled: true}
        db.Create(&cfg)
    }
    return cfg
}

func sendEmailChannel(ctx context.Context, db *gorm.DB, ch NotificationChannel, msg notificationMessage) (string, error) {
    var cfg emailChannelConfig
    if err := decryptChannelConfig(db, ch, &cfg); err != nil {
        return "", err
    }
    sys := systemNotificationConfig(db)
    if !sys.Enabled || sys.SMTPHost == "" || sys.FromAddress == "" {
        return "", &deliveryError{Code: "SMTP_NOT_CONFIGURED", Message: "邮件服务尚未配置，请联系管理员", Permanent: true}
    }
    plain := messageText(msg)
    subject := msg.Title
    html := "<h3>" + htmlEscape(msg.Title) + "</h3><ul>"
    for _, line := range msg.Lines {
        html += "<li>" + htmlEscape(line) + "</li>"
    }
    html += "</ul>"
    if msg.Link != "" {
        html += "<p><a href=\"" + htmlEscape(msg.Link) + "\">打开用药管理</a></p>"
    }
    boundary := "medcare-" + fmt.Sprintf("%d", nowUTC().UnixNano())
    body := "From: " + sys.FromName + " <" + sys.FromAddress + ">\r\n" +
        "To: " + cfg.Address + "\r\n" +
        "Subject: =?UTF-8?B?" + base64.StdEncoding.EncodeToString([]byte(subject)) + "?=\r\n" +
        "MIME-Version: 1.0\r\n" +
        "Content-Type: multipart/alternative; boundary=\"" + boundary + "\"\r\n\r\n" +
        "--" + boundary + "\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n" + plain + "\r\n" +
        "--" + boundary + "\r\nContent-Type: text/html; charset=UTF-8\r\n\r\n" + html + "\r\n" +
        "--" + boundary + "--\r\n"
    var auth smtp.Auth
    if sys.SMTPUsername != "" {
        password, err := decryptString(db, sys.SMTPPasswordEncrypted)
        if err != nil {
            return "", &deliveryError{Code: "SMTP_PASSWORD_INVALID", Message: "SMTP 密码无法读取，请重新配置", Permanent: true}
        }
        auth = smtp.PlainAuth("", sys.SMTPUsername, password, sys.SMTPHost)
    }
    if err := sendSMTP(sys.SMTPHost, sys.SMTPPort, sys.SMTPSecurity, auth, sys.FromAddress, []string{cfg.Address}, []byte(body)); err != nil {
        return "", &deliveryError{Code: "SMTP_SEND_FAILED", Message: smtpErrorHint(sys.SMTPHost, err)}
    }
    return "", nil
}

func sendSMTP(host string, port int, security string, auth smtp.Auth, from string, to []string, message []byte) error {
    if port == 0 {
        port = 587
    }
    addr := fmt.Sprintf("%s:%d", host, port)
    if security == "ssl" || port == 465 {
        conn, err := tls.Dial("tcp", addr, &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12})
        if err != nil {
            return err
        }
        defer conn.Close()
        client, err := smtp.NewClient(conn, host)
        if err != nil {
            return err
        }
        defer client.Quit()
        if auth != nil {
            if err := client.Auth(auth); err != nil {
                return err
            }
        }
        if err := client.Mail(from); err != nil {
            return err
        }
        for _, r := range to {
            if err := client.Rcpt(r); err != nil {
                return err
            }
        }
        w, err := client.Data()
        if err != nil {
            return err
        }
        if _, err := w.Write(message); err != nil {
            _ = w.Close()
            return err
        }
        return w.Close()
    }
    return smtp.SendMail(addr, auth, from, to, message)
}

func smtpErrorHint(host string, err error) string {
    detail := strings.ToLower(err.Error())
    if strings.Contains(detail, "535") || strings.Contains(detail, "authentication failed") || strings.Contains(detail, "login") {
        if strings.EqualFold(host, "smtp.qq.com") {
            return "QQ 邮箱 SMTP 登录失败：请开启 SMTP 服务并使用授权码，不是登录密码"
        }
        return "SMTP 登录失败：请检查用户名，并使用邮箱授权码或应用专用密码"
    }
    return "邮件发送失败：" + trimRunes(err.Error(), 160)
}

func sendDingTalkChannel(ctx context.Context, db *gorm.DB, ch NotificationChannel, msg notificationMessage) (string, error) {
    var cfg dingTalkConfig
    if err := decryptChannelConfig(db, ch, &cfg); err != nil {
        return "", err
    }
    endpoint := cfg.Webhook
    if cfg.Secret != "" {
        ts := fmt.Sprintf("%d", nowUTC().UnixMilli())
        stringToSign := ts + "\n" + cfg.Secret
        mac := hmac.New(sha256.New, []byte(cfg.Secret))
        mac.Write([]byte(stringToSign))
        sign := url.QueryEscape(base64.StdEncoding.EncodeToString(mac.Sum(nil)))
        sep := "?"
        if strings.Contains(endpoint, "?") {
            sep = "&"
        }
        endpoint += sep + "timestamp=" + ts + "&sign=" + sign
    }
    // 不再在正文前拼「自定义关键词」前缀（用户反馈消息里多出一段【关键词】很难看）。
    // 机器人安全设置请改用「加签」或 IP 白名单。
    text := messageText(msg)
    payload := map[string]interface{}{
        "msgtype": "markdown",
        "markdown": map[string]string{
            "title": msg.Title,
            "text":  text,
        },
    }
    var out struct {
        ErrCode *int   `json:"errcode"`
        ErrMsg  string `json:"errmsg"`
    }
    if err := postJSON(ctx, endpoint, "", payload, &out); err != nil {
        return "", err
    }
    if out.ErrCode != nil && *out.ErrCode != 0 {
        code := *out.ErrCode
        permanent := code == 310000 || code == 400101 || code == 400102
        return "", &deliveryError{Code: fmt.Sprintf("DINGTALK_%d", code), Message: "钉钉发送失败：" + trimRunes(out.ErrMsg, 150), Permanent: permanent}
    }
    return "", nil
}

func sendFeishuChannel(ctx context.Context, db *gorm.DB, ch NotificationChannel, msg notificationMessage) (string, error) {
    var cfg feishuConfig
    if err := decryptChannelConfig(db, ch, &cfg); err != nil {
        return "", err
    }
    // 同上：飞书正文也不再拼自定义关键词前缀，安全设置请用「签名校验」。
    feishuText := messageText(msg)
    payload := map[string]interface{}{
        "msg_type": "text",
        "content":  map[string]string{"text": feishuText},
    }
    if cfg.Secret != "" {
        ts := fmt.Sprintf("%d", nowUTC().Unix())
        mac := hmac.New(sha256.New, []byte(ts+"\n"+cfg.Secret))
        mac.Write([]byte(""))
        payload["timestamp"] = ts
        payload["sign"] = base64.StdEncoding.EncodeToString(mac.Sum(nil))
    }
    var out struct {
        Code *int   `json:"code"`
        Msg  string `json:"msg"`
    }
    if err := postJSON(ctx, cfg.Webhook, "", payload, &out); err != nil {
        return "", err
    }
    if out.Code != nil && *out.Code != 0 {
        code := *out.Code
        permanent := code == 19021 || code == 19001
        return "", &deliveryError{Code: fmt.Sprintf("FEISHU_%d", code), Message: "飞书发送失败：" + trimRunes(out.Msg, 150), Permanent: permanent}
    }
    return "", nil
}

func postJSON(ctx context.Context, endpoint, bearer string, payload interface{}, out interface{}) error {
    body, err := json.Marshal(payload)
    if err != nil {
        return err
    }
    req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
    if err != nil {
        return &deliveryError{Code: "REQUEST_INVALID", Message: "通知地址无效", Permanent: true}
    }
    req.Header.Set("Content-Type", "application/json")
    if bearer != "" {
        req.Header.Set("Authorization", "Bearer "+bearer)
    }
    client := &http.Client{
        Timeout: 15 * time.Second,
        CheckRedirect: func(req *http.Request, via []*http.Request) error {
            return http.ErrUseLastResponse
        },
    }
    resp, err := client.Do(req)
    if err != nil {
        return &deliveryError{Code: "NETWORK_ERROR", Message: "网络请求失败：" + trimRunes(err.Error(), 120)}
    }
    defer resp.Body.Close()
    raw, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
    if resp.StatusCode < 200 || resp.StatusCode >= 300 {
        return &deliveryError{Code: fmt.Sprintf("HTTP_%d", resp.StatusCode), Message: "通知服务返回 " + resp.Status, Permanent: resp.StatusCode >= 400 && resp.StatusCode < 500}
    }
    if out != nil && len(raw) > 0 {
        _ = json.Unmarshal(raw, out)
    }
    return nil
}

func htmlEscape(s string) string {
    replacer := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", "\"", "&#34;", "'", "&#39;")
    return replacer.Replace(s)
}
