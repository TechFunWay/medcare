package medcare

import (
    "context"
    "encoding/json"
    "io"
    "net/http"
    "net/http/httptest"
    "strings"
    "testing"

    "smallgo/server/database"
    "smallgo/server/sysconfig"

    "gorm.io/gorm"
)

func setupOCRDB(t *testing.T) *gorm.DB {
    t.Helper()
    db := setupPrescriptionDB(t)
    if err := db.AutoMigrate(&database.SystemConfig{}, &SystemOCRConfig{}); err != nil {
        t.Fatalf("migrate ocr: %v", err)
    }
    if err := sysconfig.InitDefaultConfigs(db); err != nil {
        t.Fatalf("init configs: %v", err)
    }
    return db
}

const ocrDraftJSON = `{"doc_type":"receipt","items":[{"name":"氨氯地平片","spec":"5mg×7片","quantity":2,"unit":"盒"}]}`

func aiResponseWithContent(content string) string {
    b, _ := json.Marshal(content)
    return `{"choices":[{"message":{"content":` + string(b) + `}}]}`
}

// fakeAIServer 同一个假服务承担两种角色：
// 带 image_url 的请求当 OCR（返回逐行文本），不带的请求当整理清单（返回草稿 JSON）。
// fakeAIServer：带 image_url 的请求返回 imageContent（视觉识别 / 自定义 OCR 读字），
// 不带图的请求返回草稿 JSON（OCR 文本整理）。
func fakeAIServer(t *testing.T, imageContent string) (*httptest.Server, *bool) {
    t.Helper()
    var sawImage bool
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        body, _ := io.ReadAll(r.Body)
        if strings.Contains(string(body), "image_url") {
            sawImage = true
            _, _ = w.Write([]byte(aiResponseWithContent(imageContent)))
            return
        }
        _, _ = w.Write([]byte(aiResponseWithContent(ocrDraftJSON)))
    }))
    t.Cleanup(srv.Close)
    return srv, &sawImage
}

// OCR 命中（百度假服务）：整理阶段不再发图，草稿带 OCR 原文。
func TestRecognizeUsesOCRTextFirst(t *testing.T) {
    db := setupOCRDB(t)
    tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        _, _ = w.Write([]byte(`{"access_token":"tok","expires_in":3600}`))
    }))
    defer tokenSrv.Close()
    ocrSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        _, _ = w.Write([]byte(`{"words_result":[{"words":"硫酸羟氯喹片 2 盒"},{"words":"氨氯地平片 5mg×7片 2 盒"}]}`))
    }))
    defer ocrSrv.Close()
    oldT, oldO := baiduTokenURL, baiduOCRURL
    baiduTokenURL, baiduOCRURL = tokenSrv.URL, ocrSrv.URL
    defer func() { baiduTokenURL, baiduOCRURL = oldT, oldO }()

    aiSrv, sawImage := fakeAIServer(t, ocrDraftJSON)
    keyEnc, _ := encryptString(db, "sk-test")
    secretEnc, _ := encryptString(db, "sk-secret")
    db.Create(&SystemOCRConfig{ID: 1, Enabled: true, Provider: "baidu", APIKey: "ak", SecretKeyEncrypted: secretEnc})
    cfg := SystemAIConfig{Enabled: true, BaseURL: aiSrv.URL, Model: "m", APIKeyEncrypted: keyEnc, TimeoutSeconds: 30}
    draft, err := recognizePrescription(context.Background(), db, cfg, []byte("fake-image"), "image/png", RecognizeKindReceipt)
    if err != nil {
        t.Fatalf("recognize: %v", err)
    }
    if *sawImage {
        t.Fatal("OCR 命中时整理清单不应再发图")
    }
    if draft.OCRText == "" || !strings.Contains(draft.OCRText, "氨氯地平片") {
        t.Fatalf("草稿应带 OCR 原文，got %q", draft.OCRText)
    }
    if len(draft.Items) != 1 {
        t.Fatalf("清单条目数不对: %+v", draft.Items)
    }
}

// 自定义服务商（OpenAI 兼容视觉模型）也能当 OCR：发图拿到文本，再整理。
func TestRecognizeWithCustomOCRProvider(t *testing.T) {
    db := setupOCRDB(t)
    aiSrv, sawImage := fakeAIServer(t, "氨氯地平片 5mg×7片 2 盒")
    keyEnc, _ := encryptString(db, "sk-test")
    db.Create(&SystemOCRConfig{ID: 1, Enabled: true, Provider: "custom", BaseURL: aiSrv.URL, Model: "vl", APIKey: "sk-ocr"})
    cfg := SystemAIConfig{Enabled: true, BaseURL: aiSrv.URL, Model: "m", APIKeyEncrypted: keyEnc, TimeoutSeconds: 30}
    draft, err := recognizePrescription(context.Background(), db, cfg, []byte("img"), "image/png", RecognizeKindReceipt)
    if err != nil {
        t.Fatalf("自定义 OCR 应成功: %v", err)
    }
    if !*sawImage {
        t.Fatal("自定义 OCR 需要发图给视觉模型")
    }
    if !strings.Contains(draft.OCRText, "氨氯地平片") {
        t.Fatalf("应带上自定义 OCR 读出的文本，got %q", draft.OCRText)
    }
}

// OCR 失败时回退视觉识别：仍发图、仍能成功、OCR 原文为空。
func TestRecognizeFallsBackToVisionWhenOCRFails(t *testing.T) {
    db := setupOCRDB(t)
    tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        _, _ = w.Write([]byte(`{"access_token":"tok","expires_in":3600}`))
    }))
    defer tokenSrv.Close()
    ocrSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        _, _ = w.Write([]byte(`{"error_code":110,"error_msg":"Access token invalid"}`))
    }))
    defer ocrSrv.Close()
    oldT, oldO := baiduTokenURL, baiduOCRURL
    baiduTokenURL, baiduOCRURL = tokenSrv.URL, ocrSrv.URL
    defer func() { baiduTokenURL, baiduOCRURL = oldT, oldO }()

    aiSrv, sawImage := fakeAIServer(t, ocrDraftJSON)
    keyEnc, _ := encryptString(db, "sk-test")
    secretEnc, _ := encryptString(db, "sk-secret")
    db.Create(&SystemOCRConfig{ID: 1, Enabled: true, Provider: "baidu", APIKey: "ak", SecretKeyEncrypted: secretEnc})
    cfg := SystemAIConfig{Enabled: true, BaseURL: aiSrv.URL, Model: "m", APIKeyEncrypted: keyEnc, TimeoutSeconds: 30}
    draft, err := recognizePrescription(context.Background(), db, cfg, []byte("img"), "image/png", RecognizeKindReceipt)
    if err != nil {
        t.Fatalf("回退视觉识别应成功: %v", err)
    }
    if !*sawImage {
        t.Fatal("OCR 失败后应回退发图识别")
    }
    if draft.OCRText != "" {
        t.Fatalf("回退路径不应带 OCR 原文: %q", draft.OCRText)
    }
}

// 未启用 OCR 时行为不变：直接视觉识别。
func TestRecognizeWithoutOCRConfig(t *testing.T) {
    db := setupOCRDB(t)
    aiSrv, sawImage := fakeAIServer(t, ocrDraftJSON)
    keyEnc, _ := encryptString(db, "sk-test")
    cfg := SystemAIConfig{Enabled: true, BaseURL: aiSrv.URL, Model: "m", APIKeyEncrypted: keyEnc, TimeoutSeconds: 30}
    if _, err := recognizePrescription(context.Background(), db, cfg, []byte("img"), "image/png", RecognizeKindReceipt); err != nil {
        t.Fatalf("recognize: %v", err)
    }
    if !*sawImage {
        t.Fatal("未配 OCR 时应走视觉识别")
    }
}

// DeepSeek OCR：multipart 上传 + 按页返回，标记与坐标要清掉。
func TestRecognizeWithDeepSeekOCRProvider(t *testing.T) {
    db := setupOCRDB(t)
    var gotPurpose, gotAuth bool
    dsSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if r.Header.Get("Authorization") == "Bearer sk-ds" {
            gotAuth = true
        }
        _ = r.ParseMultipartForm(8 << 20)
        if r.FormValue("purpose") == "text_recognition" {
            gotPurpose = true
        }
        if _, _, err := r.FormFile("file"); err != nil {
            t.Errorf("缺少 file 字段: %v", err)
        }
        // 1x1 PNG，解码够用即可
        _, _ = w.Write([]byte(`{"id":"ocr-1","object":"ocr.result","page_count":1,"pages":[{"index":0,"text":"<｜ref｜>氨氯地平片 5mg×7片 2 盒<｜/ref｜><｜box｜>[[52, 40, 948, 96]]<｜/box｜>\n<｜ref｜>测试新药X 1 盒<｜/ref｜>","finish_reason":"stop"}]}`))
    }))
    defer dsSrv.Close()
    old := deepseekOCREndpoint
    deepseekOCREndpoint = dsSrv.URL
    defer func() { deepseekOCREndpoint = old }()

    aiSrv, sawImage := fakeAIServer(t, ocrDraftJSON)
    db.Create(&SystemOCRConfig{ID: 1, Enabled: true, Provider: "deepseek", APIKey: "sk-ds"})
    cfg := SystemAIConfig{Enabled: true, BaseURL: aiSrv.URL, Model: "m", APIKeyEncrypted: mustEncrypt(t, db, "sk-ai"), TimeoutSeconds: 30}
    draft, err := recognizePrescription(context.Background(), db, cfg, []byte("img"), "image/png", RecognizeKindReceipt)
    if err != nil {
        t.Fatalf("DeepSeek OCR 应成功: %v", err)
    }
    if !gotAuth || !gotPurpose {
        t.Fatalf("请求头/参数不对 auth=%v purpose=%v", gotAuth, gotPurpose)
    }
    if *sawImage {
        t.Fatal("OCR 命中后整理清单不应再发图")
    }
    if strings.Contains(draft.OCRText, "｜") || strings.Contains(draft.OCRText, "[[") {
        t.Fatalf("OCR 原文应清掉结构标记与坐标，got %q", draft.OCRText)
    }
    if !strings.Contains(draft.OCRText, "氨氯地平片") || !strings.Contains(draft.OCRText, "测试新药X") {
        t.Fatalf("两行文字都应保留，got %q", draft.OCRText)
    }
}

func mustEncrypt(t *testing.T, db *gorm.DB, s string) string {
    t.Helper()
    v, err := encryptString(db, s)
    if err != nil {
        t.Fatalf("encrypt: %v", err)
    }
    return v
}


// 需要「第二个密钥」的服务商清单：DeepSeek 与自定义只需 API Key，
// 之前所有非 custom 都被要求填 Secret Key，导致 DeepSeek 提示必填。
func TestOCRNeedsSecret(t *testing.T) {
    cases := map[string]bool{
        "baidu": true, "tencent": true, "aliyun": true, "youdao": true,
        "deepseek": false, "custom": false, "": false,
    }
    for provider, want := range cases {
        if got := ocrNeedsSecret(provider); got != want {
            t.Errorf("ocrNeedsSecret(%q) = %v, want %v", provider, got, want)
        }
    }
}

