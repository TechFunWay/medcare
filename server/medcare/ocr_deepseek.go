package medcare

import (
    "bytes"
    "context"
    "encoding/json"
    "errors"
    "fmt"
    "io"
    "mime/multipart"
    "net/http"
    "regexp"
    "strings"
    "time"
)

// DeepSeek OCR（2026-11-17 按官方文档接入）：
// POST {base}/v1/tools/ocr，multipart/form-data 传 file + purpose，
// 按页返回 pages[].text。图片逐页识别，输出里带 <｜ref｜>/<｜box｜> 标记与归一化坐标，
// 我们只做「读字」这一层，标记与坐标对后续整理无意义，统一清掉。
var deepseekOCREndpoint = "https://api.deepseek.com/v1/tools/ocr"

// deepseekOCRClient 总时长按文档 180 秒放宽（上传 120 秒 + 识别），默认 40 秒不够。
func deepseekOCRClient() *http.Client {
    return &http.Client{Timeout: 200 * time.Second, Transport: appHTTPTransport()}
}

var (
    dsTagPattern = regexp.MustCompile("<｜/?[A-Za-z_]+｜>")
    dsBoxPattern = regexp.MustCompile(`\[\[[0-9,\s]+\]\]`)
)

// cleanDeepSeekOCRText 去掉 <｜ref｜>…<｜/ref｜> 之类的结构标记与 [[x1,y1,x2,y2]] 坐标，
// 只留纯文字；连续空行压成一行，方便交给文本模型整理。
func cleanDeepSeekOCRText(s string) string {
    s = dsTagPattern.ReplaceAllString(s, "")
    s = dsBoxPattern.ReplaceAllString(s, "")
    lines := strings.Split(s, "\n")
    out := make([]string, 0, len(lines))
    for _, l := range lines {
        l = strings.TrimSpace(l)
        if l != "" {
            out = append(out, l)
        }
    }
    return strings.Join(out, "\n")
}

func deepseekOCRText(ctx context.Context, oc SystemOCRConfig, image []byte, mime string) (string, int, error) {
    if strings.TrimSpace(oc.APIKey) == "" {
        return "", 0, errors.New("DeepSeek OCR 需要填写 API Key")
    }
    endpoint := strings.TrimSpace(oc.BaseURL)
    if endpoint == "" {
        // 未自定义时用内置端点（变量形式便于单测指向假服务）。
        endpoint = deepseekOCREndpoint
    } else {
        endpoint = strings.TrimRight(endpoint, "/")
        if !strings.HasSuffix(endpoint, "/tools/ocr") {
            if strings.HasSuffix(endpoint, "/v1") {
                endpoint += "/tools/ocr"
            } else {
                endpoint += "/v1/tools/ocr"
            }
        }
    }
    var buf bytes.Buffer
    w := multipart.NewWriter(&buf)
    name := "upload.png"
    switch mime {
    case "image/jpeg":
        name = "upload.jpg"
    case "image/webp":
        name = "upload.webp"
    }
    part, err := w.CreateFormFile("file", name)
    if err != nil {
        return "", 0, err
    }
    if _, err := part.Write(image); err != nil {
        return "", 0, err
    }
    // 图片里通常有版面信息，用 text_recognition 即可（图片一律逐页识别）。
    if err := w.WriteField("purpose", "text_recognition"); err != nil {
        return "", 0, err
    }
    if err := w.Close(); err != nil {
        return "", 0, err
    }
    req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, &buf)
    if err != nil {
        return "", 0, err
    }
    req.Header.Set("Content-Type", w.FormDataContentType())
    req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(oc.APIKey))
    resp, err := deepseekOCRClient().Do(req)
    if err != nil {
        return "", 0, fmt.Errorf("DeepSeek OCR 连接失败：%w", err)
    }
    defer resp.Body.Close()
    raw, err := io.ReadAll(io.LimitReader(resp.Body, 16*1024*1024))
    if err != nil {
        return "", 0, fmt.Errorf("DeepSeek OCR 响应读取失败：%w", err)
    }
    var out struct {
        Pages []struct {
            Text string `json:"text"`
        } `json:"pages"`
        Error *struct {
            Message string `json:"message"`
            Code    string `json:"code"`
        } `json:"error"`
        Usage struct {
            InputTokens  int `json:"input_tokens"`
            OutputTokens int `json:"output_tokens"`
        } `json:"usage"`
    }
    if err := json.Unmarshal(bytes.TrimSpace(raw), &out); err != nil {
        if resp.StatusCode < 200 || resp.StatusCode >= 300 {
            return "", 0, fmt.Errorf("DeepSeek OCR 返回 %s：%s", resp.Status, trimRunes(string(raw), 150))
        }
        return "", 0, fmt.Errorf("DeepSeek OCR 响应无法解析：%s", trimRunes(string(raw), 150))
    }
    // 识别开始后状态码已是 200，失败信息放在 error 里，必须查。
    if out.Error != nil && out.Error.Message != "" {
        return "", 0, fmt.Errorf("DeepSeek OCR 错误：%s", trimRunes(out.Error.Message, 150))
    }
    if resp.StatusCode < 200 || resp.StatusCode >= 300 {
        return "", 0, fmt.Errorf("DeepSeek OCR 返回 %s", resp.Status)
    }
    var sb strings.Builder
    for i, p := range out.Pages {
        t := cleanDeepSeekOCRText(p.Text)
        if t == "" {
            continue
        }
        if i > 0 && sb.Len() > 0 {
            sb.WriteString("\n")
        }
        sb.WriteString(t)
    }
    text := strings.TrimSpace(sb.String())
    if text == "" {
        return "", 0, errors.New("DeepSeek OCR 未识别到文字")
    }
    return text, out.Usage.InputTokens + out.Usage.OutputTokens, nil
}
