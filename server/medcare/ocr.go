package medcare

import (
    "context"
    "encoding/base64"
    "encoding/json"
    "errors"
    "fmt"
    "io"
    "net/http"
    "net/url"
    "strings"
    "sync"
    "time"

    "gorm.io/gorm"
)

// OCR 辅助识别：先把图片上的字读成文本，再交给文本模型整理清单。
// 支持多家服务商（百度 / 腾讯云 / 阿里云 / 有道 / DeepSeek）与自定义 OpenAI 兼容视觉模型；
// 入口统一为 ocrExtractText，返回（文本, OCR 服务返回的 token 用量, 错误）。
var (
    baiduTokenURL = "https://aip.baidubce.com/oauth/2.0/token"
    baiduOCRURL   = "https://aip.baidubce.com/rest/2.0/ocr/v1/accurate_basic"
)

var ocrTokenCache struct {
    sync.Mutex
    ak, sk string
    token  string
    exp    time.Time
}

// baiduAccessToken 用 API Key + Secret Key 换 access_token，进程内缓存到过期前 5 分钟。
func baiduAccessToken(ctx context.Context, ak, sk string) (string, error) {
    ocrTokenCache.Lock()
    defer ocrTokenCache.Unlock()
    if ocrTokenCache.ak == ak && ocrTokenCache.sk == sk && ocrTokenCache.token != "" && time.Now().Before(ocrTokenCache.exp) {
        return ocrTokenCache.token, nil
    }
    form := url.Values{}
    form.Set("grant_type", "client_credentials")
    form.Set("client_id", ak)
    form.Set("client_secret", sk)
    req, err := http.NewRequestWithContext(ctx, http.MethodPost, baiduTokenURL, strings.NewReader(form.Encode()))
    if err != nil {
        return "", err
    }
    req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
    client := &http.Client{Timeout: 20 * time.Second, Transport: appHTTPTransport()}
    resp, err := client.Do(req)
    if err != nil {
        return "", fmt.Errorf("OCR 令牌服务连接失败：%w", err)
    }
    defer resp.Body.Close()
    raw, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
    if err != nil {
        return "", fmt.Errorf("OCR 令牌响应读取失败：%w", err)
    }
    var out struct {
        AccessToken string `json:"access_token"`
        ExpiresIn   int64  `json:"expires_in"`
        Error       string `json:"error"`
        ErrorDesc   string `json:"error_description"`
    }
    if err := json.Unmarshal(raw, &out); err != nil || out.AccessToken == "" {
        msg := out.ErrorDesc
        if msg == "" {
            msg = out.Error
        }
        if msg == "" {
            msg = trimRunes(string(raw), 120)
        }
        return "", fmt.Errorf("OCR 令牌获取失败：%s", msg)
    }
    ttl := time.Duration(out.ExpiresIn) * time.Second
    if ttl <= 0 {
        ttl = 24 * time.Hour
    }
    ocrTokenCache.ak, ocrTokenCache.sk = ak, sk
    ocrTokenCache.token = out.AccessToken
    ocrTokenCache.exp = time.Now().Add(ttl - 5*time.Minute)
    return out.AccessToken, nil
}

// ocrExtractText 按配置的服务商把图片读成逐行文本；同时回传该服务商报告的 token 用量
// （多数传统 OCR 不返回 token，返回 0）。失败返回错误，调用方回退视觉识别。
func ocrExtractText(ctx context.Context, db *gorm.DB, oc SystemOCRConfig, image []byte, mime string) (string, int, error) {
    switch oc.Provider {
    case "", "baidu":
        return baiduOCRText(ctx, db, oc, image)
    case "tencent":
        return tencentOCRText(ctx, db, oc, image)
    case "aliyun":
        return aliyunOCRText(ctx, db, oc, image)
    case "youdao":
        return youdaoOCRText(ctx, db, oc, image)
    case "deepseek":
        return deepseekOCRText(ctx, oc, image, mime)
    case "custom":
        return customVisionOCRText(ctx, oc, image, mime)
    default:
        return "", 0, fmt.Errorf("不支持的 OCR 服务商：%s", oc.Provider)
    }
}

func baiduOCRText(ctx context.Context, db *gorm.DB, oc SystemOCRConfig, image []byte) (string, int, error) {
    sk, err := decryptString(db, oc.SecretKeyEncrypted)
    if err != nil {
        return "", 0, errors.New("OCR Secret Key 解密失败，请重新保存")
    }
    cctx, cancel := context.WithTimeout(ctx, 40*time.Second)
    defer cancel()
    token, err := baiduAccessToken(cctx, oc.APIKey, sk)
    if err != nil {
        return "", 0, err
    }
    form := url.Values{}
    form.Set("image", base64.StdEncoding.EncodeToString(image))
    form.Set("language_type", "CHN_ENG")
    form.Set("detect_direction", "true")
    req, err := http.NewRequestWithContext(cctx, http.MethodPost, baiduOCRURL+"?access_token="+url.QueryEscape(token), strings.NewReader(form.Encode()))
    if err != nil {
        return "", 0, err
    }
    req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
    client := &http.Client{Timeout: 40 * time.Second, Transport: appHTTPTransport()}
    resp, err := client.Do(req)
    if err != nil {
        return "", 0, fmt.Errorf("OCR 服务连接失败：%w", err)
    }
    defer resp.Body.Close()
    raw, err := io.ReadAll(io.LimitReader(resp.Body, 4*1024*1024))
    if err != nil {
        return "", 0, fmt.Errorf("OCR 响应读取失败：%w", err)
    }
    var out struct {
        WordsResult []struct {
            Words string `json:"words"`
        } `json:"words_result"`
        ErrorCode int    `json:"error_code"`
        ErrorMsg  string `json:"error_msg"`
    }
    if err := json.Unmarshal(raw, &out); err != nil {
        return "", 0, fmt.Errorf("OCR 响应无法解析：%s", trimRunes(string(raw), 120))
    }
    if out.ErrorCode != 0 {
        return "", 0, fmt.Errorf("OCR 服务错误 %d：%s", out.ErrorCode, out.ErrorMsg)
    }
    lines := make([]string, 0, len(out.WordsResult))
    for _, w := range out.WordsResult {
        if strings.TrimSpace(w.Words) != "" {
            lines = append(lines, w.Words)
        }
    }
    return strings.Join(lines, "\n"), 0, nil
}
