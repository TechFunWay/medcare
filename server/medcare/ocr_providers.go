package medcare

import (
    "context"
    "crypto/hmac"
    "crypto/sha1"
    "crypto/sha256"
    "encoding/base64"
    "encoding/json"
    "errors"
    "fmt"
    "io"
    "net/http"
    "net/url"
    "sort"
    "strings"
    "time"

    "gorm.io/gorm"
)

// 各家 OCR 的端点做成变量，单测可指向假服务，不打真实外网。
var (
    tencentOCREndpoint = "https://ocr.tencentcloudapi.com"
    aliyunOCREndpoint  = "https://ocr-api.cn-hangzhou.aliyuncs.com"
    youdaoOCREndpoint  = "https://openapi.youdao.com/ocrapi"
)

func ocrHTTPClient() *http.Client {
    return &http.Client{Timeout: 40 * time.Second, Transport: appHTTPTransport()}
}

// collectStrings 递归收集 JSON 里所有字符串值，供不同服务商的响应兼容解析。
func collectStrings(v interface{}, out *[]string) {
    switch t := v.(type) {
    case string:
        if strings.TrimSpace(t) != "" {
            *out = append(*out, t)
        }
    case []interface{}:
        for _, x := range t {
            collectStrings(x, out)
        }
    case map[string]interface{}:
        keys := make([]string, 0, len(t))
        for k := range t {
            keys = append(keys, k)
        }
        sort.Strings(keys)
        for _, k := range keys {
            collectStrings(t[k], out)
        }
    }
}

// joinOCRStrings 取收集到的字符串里内容量最大的那段，作为识别文本。
func joinOCRStrings(raw []byte) (string, error) {
    var v interface{}
    if err := json.Unmarshal(raw, &v); err != nil {
        return "", fmt.Errorf("OCR 响应无法解析：%s", trimRunes(string(raw), 120))
    }
    var all []string
    collectStrings(v, &all)
    best, bestScore := "", 0
    for _, s := range all {
        if score := len([]rune(s)); score > bestScore {
            best, bestScore = s, score
        }
    }
    if bestScore == 0 {
        return "", errors.New("OCR 未返回任何文字")
    }
    return best, nil
}

// ---------- 腾讯云 OCR（TC3-HMAC-SHA256，GeneralAccurateOCR）----------

func tencentOCRText(ctx context.Context, db *gorm.DB, oc SystemOCRConfig, image []byte) (string, int, error) {
    secretKey, err := decryptString(db, oc.SecretKeyEncrypted)
    if err != nil {
        return "", 0, errors.New("腾讯云 SecretKey 解密失败，请重新保存")
    }
    payload, _ := json.Marshal(map[string]interface{}{"ImageBase64": base64.StdEncoding.EncodeToString(image)})
    host := strings.TrimPrefix(strings.TrimPrefix(tencentOCREndpoint, "https://"), "http://")
    if i := strings.Index(host, "/"); i >= 0 {
        host = host[:i]
    }
    ts := time.Now().Unix()
    date := time.Unix(ts, 0).UTC().Format("2006-01-02")
    ct := "application/json; charset=utf-8"
    canonicalRequest := "POST\n/\n\ncontent-type:" + ct + "\nhost:" + host + "\n\ncontent-type;host\n" + sha256Hex(string(payload))
    stringToSign := "TC3-HMAC-SHA256\n" + fmt.Sprint(ts) + "\n" + date + "/ocr/tc3_request\n" + sha256Hex(canonicalRequest)
    kDate := hmacSHA256([]byte("TC3"+secretKey), date)
    kService := hmacSHA256(kDate, "ocr")
    kSigning := hmacSHA256(kService, "tc3_request")
    signature := fmt.Sprintf("%x", hmacSHA256(kSigning, stringToSign))
    auth := "TC3-HMAC-SHA256 Credential=" + oc.APIKey + "/" + date + "/ocr/tc3_request, SignedHeaders=content-type;host, Signature=" + signature
    req, err := http.NewRequestWithContext(ctx, http.MethodPost, tencentOCREndpoint, strings.NewReader(string(payload)))
    if err != nil {
        return "", 0, err
    }
    req.Header.Set("Content-Type", ct)
    req.Header.Set("Host", host)
    req.Header.Set("Authorization", auth)
    req.Header.Set("X-TC-Action", "GeneralAccurateOCR")
    req.Header.Set("X-TC-Version", "2018-11-19")
    req.Header.Set("X-TC-Region", "ap-guangzhou")
    req.Header.Set("X-TC-Timestamp", fmt.Sprint(ts))
    resp, err := ocrHTTPClient().Do(req)
    if err != nil {
        return "", 0, fmt.Errorf("腾讯云 OCR 连接失败：%w", err)
    }
    defer resp.Body.Close()
    raw, err := io.ReadAll(io.LimitReader(resp.Body, 4*1024*1024))
    if err != nil {
        return "", 0, fmt.Errorf("腾讯云 OCR 响应读取失败：%w", err)
    }
    var out struct {
        Response struct {
            TextDetections []struct {
                DetectedText string `json:"DetectedText"`
            } `json:"TextDetections"`
            Error *struct {
                Code    string `json:"Code"`
                Message string `json:"Message"`
            } `json:"Error"`
        } `json:"Response"`
    }
    if json.Unmarshal(raw, &out) == nil && out.Response.Error != nil {
        return "", 0, fmt.Errorf("腾讯云 OCR 错误 %s：%s", out.Response.Error.Code, trimRunes(out.Response.Error.Message, 120))
    }
    lines := make([]string, 0, len(out.Response.TextDetections))
    for _, d := range out.Response.TextDetections {
        if strings.TrimSpace(d.DetectedText) != "" {
            lines = append(lines, d.DetectedText)
        }
    }
    if len(lines) == 0 {
        text, err := joinOCRStrings(raw)
        return text, 0, err
    }
    return strings.Join(lines, "\n"), 0, nil
}

func hmacSHA256(key []byte, data string) []byte {
    m := hmac.New(sha256.New, key)
    m.Write([]byte(data))
    return m.Sum(nil)
}

// ---------- 阿里云 OCR（RPC 签名 v1 + octet-stream，RecognizeGeneral）----------

func aliyunOCRText(ctx context.Context, db *gorm.DB, oc SystemOCRConfig, image []byte) (string, int, error) {
    secret, err := decryptString(db, oc.SecretKeyEncrypted)
    if err != nil {
        return "", 0, errors.New("阿里云 AccessKeySecret 解密失败，请重新保存")
    }
    params := map[string]string{
        "Action":           "RecognizeGeneral",
        "Version":          "2021-07-07",
        "Format":           "JSON",
        "SignatureMethod":  "HMAC-SHA1",
        "SignatureVersion": "1.0",
        "SignatureNonce":   fmt.Sprintf("%d", time.Now().UnixNano()),
        "AccessKeyId":      oc.APIKey,
        "Timestamp":        time.Now().UTC().Format("2006-01-02T15:04:05Z"),
    }
    keys := make([]string, 0, len(params))
    for k := range params {
        keys = append(keys, k)
    }
    sort.Strings(keys)
    var sb strings.Builder
    for i, k := range keys {
        if i > 0 {
            sb.WriteString("&")
        }
        sb.WriteString(percentEncode(k) + "=" + percentEncode(params[k]))
    }
    stringToSign := "POST&%2F&" + percentEncode(sb.String())
    mac := hmac.New(sha1.New, []byte(secret+"&"))
    mac.Write([]byte(stringToSign))
    params["Signature"] = base64.StdEncoding.EncodeToString(mac.Sum(nil))
    q := url.Values{}
    for k, v := range params {
        q.Set(k, v)
    }
    req, err := http.NewRequestWithContext(ctx, http.MethodPost, aliyunOCREndpoint+"/?"+q.Encode(), strings.NewReader(string(image)))
    if err != nil {
        return "", 0, err
    }
    req.Header.Set("Content-Type", "application/octet-stream")
    resp, err := ocrHTTPClient().Do(req)
    if err != nil {
        return "", 0, fmt.Errorf("阿里云 OCR 连接失败：%w", err)
    }
    defer resp.Body.Close()
    raw, err := io.ReadAll(io.LimitReader(resp.Body, 4*1024*1024))
    if err != nil {
        return "", 0, fmt.Errorf("阿里云 OCR 响应读取失败：%w", err)
    }
    text, err := joinOCRStrings(raw)
    return text, 0, err
}

func percentEncode(s string) string {
    e := url.QueryEscape(s)
    e = strings.ReplaceAll(e, "+", "%20")
    e = strings.ReplaceAll(e, "*", "%2A")
    e = strings.ReplaceAll(e, "%7E", "~")
    return e
}

// ---------- 有道智云 OCR（signType=v3）----------

func youdaoOCRText(ctx context.Context, db *gorm.DB, oc SystemOCRConfig, image []byte) (string, int, error) {
    secret, err := decryptString(db, oc.SecretKeyEncrypted)
    if err != nil {
        return "", 0, errors.New("有道应用密钥解密失败，请重新保存")
    }
    img := base64.StdEncoding.EncodeToString(image)
    salt := fmt.Sprintf("%d", time.Now().UnixNano())
    curtime := fmt.Sprintf("%d", time.Now().Unix())
    sign := sha256Hex(oc.APIKey + img + salt + curtime + secret)
    form := url.Values{}
    form.Set("img", img)
    form.Set("langType", "zh_cn")
    form.Set("detectType", "10012")
    form.Set("imageType", "1")
    form.Set("appKey", oc.APIKey)
    form.Set("salt", salt)
    form.Set("curtime", curtime)
    form.Set("sign", sign)
    form.Set("signType", "v3")
    form.Set("docType", "json")
    req, err := http.NewRequestWithContext(ctx, http.MethodPost, youdaoOCREndpoint, strings.NewReader(form.Encode()))
    if err != nil {
        return "", 0, err
    }
    req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
    resp, err := ocrHTTPClient().Do(req)
    if err != nil {
        return "", 0, fmt.Errorf("有道 OCR 连接失败：%w", err)
    }
    defer resp.Body.Close()
    raw, err := io.ReadAll(io.LimitReader(resp.Body, 4*1024*1024))
    if err != nil {
        return "", 0, fmt.Errorf("有道 OCR 响应读取失败：%w", err)
    }
    var probe struct {
        ErrorCode []string `json:"errorCode"`
    }
    if json.Unmarshal(raw, &probe) == nil && len(probe.ErrorCode) > 0 && probe.ErrorCode[0] != "0" {
        return "", 0, fmt.Errorf("有道 OCR 错误码 %s", probe.ErrorCode[0])
    }
    text, err := joinOCRStrings(raw)
    return text, 0, err
}

// ---------- 自定义：OpenAI 兼容的多模态模型直接吐文字 ----------

func customVisionOCRText(ctx context.Context, oc SystemOCRConfig, image []byte, mime string) (string, int, error) {
    if strings.TrimSpace(oc.BaseURL) == "" || strings.TrimSpace(oc.Model) == "" {
        return "", 0, errors.New("自定义 OCR 需要填写 Base URL 与模型")
    }
    if oc.APIKey == "" {
        return "", 0, errors.New("自定义 OCR 需要填写 API Key")
    }
    endpoint := strings.TrimRight(strings.TrimSpace(oc.BaseURL), "/")
    if !strings.HasSuffix(endpoint, "/chat/completions") {
        endpoint += "/chat/completions"
    }
    dataURL := "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(image)
    payload, _ := json.Marshal(map[string]interface{}{
        "model":       oc.Model,
        "temperature": 0,
        "messages": []map[string]interface{}{{
            "role": "user",
            "content": []map[string]interface{}{
                {"type": "text", "text": "把这张图片里的所有文字逐行原样输出，不要翻译、不要总结、不要加任何说明；表格按行输出，列之间用一个空格分隔。"},
                {"type": "image_url", "image_url": map[string]string{"url": dataURL}},
            },
        }},
    })
    req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(string(payload)))
    if err != nil {
        return "", 0, err
    }
    req.Header.Set("Content-Type", "application/json")
    req.Header.Set("Authorization", "Bearer "+oc.APIKey)
    resp, err := ocrHTTPClient().Do(req)
    if err != nil {
        return "", 0, fmt.Errorf("自定义 OCR 连接失败：%w", err)
    }
    defer resp.Body.Close()
    raw, err := io.ReadAll(io.LimitReader(resp.Body, 8*1024*1024))
    if err != nil {
        return "", 0, fmt.Errorf("自定义 OCR 响应读取失败：%w", err)
    }
    if resp.StatusCode < 200 || resp.StatusCode >= 300 {
        return "", 0, fmt.Errorf("自定义 OCR 返回 %s：%s", resp.Status, trimRunes(string(raw), 150))
    }
    var out aiChatResponse
    if err := json.Unmarshal(raw, &out); err != nil || len(out.Choices) == 0 {
        return "", 0, fmt.Errorf("自定义 OCR 响应无法解析：%s", trimRunes(string(raw), 150))
    }
    if out.Error != nil {
        return "", 0, fmt.Errorf("自定义 OCR 错误：%s", trimRunes(out.Error.Message, 150))
    }
    return out.Choices[0].Message.Content, out.Usage.PromptTokens + out.Usage.CompletionTokens, nil
}
