package medcare

import (
    "context"
    "net/http"
    "net/http/httptest"
    "strings"
    "testing"
)

func TestListAIModelsParsesOpenAIResponse(t *testing.T) {
    var gotPath, gotAuth string
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        gotPath = r.URL.Path
        gotAuth = r.Header.Get("Authorization")
        w.Header().Set("Content-Type", "application/json")
        w.Write([]byte(`{"data":[{"id":"mimo-v2.6-flash"},{"id":"mimo-v2.6-vision"},{"id":"mimo-v2.6-flash"}]}`))
    }))
    defer srv.Close()

    ids, err := listAIModels(context.Background(), srv.URL, "sk-test", 10)
    if err != nil {
        t.Fatalf("listAIModels error: %v", err)
    }
    if gotPath != "/v1/models" {
        t.Fatalf("path = %q, want /v1/models", gotPath)
    }
    if gotAuth != "Bearer sk-test" {
        t.Fatalf("auth = %q", gotAuth)
    }
    if len(ids) != 2 || ids[0] != "mimo-v2.6-flash" || ids[1] != "mimo-v2.6-vision" {
        t.Fatalf("ids = %v, want deduped two models", ids)
    }
}

func TestListAIModelsHonorsExplicitPath(t *testing.T) {
    var gotPath string
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        gotPath = r.URL.Path
        w.Write([]byte(`{"models":[{"id":"a"},{"id":"b"}]}`))
    }))
    defer srv.Close()

    if _, err := listAIModels(context.Background(), srv.URL+"/v1/models", "sk-test", 10); err != nil {
        t.Fatalf("listAIModels error: %v", err)
    }
    if gotPath != "/v1/models" {
        t.Fatalf("path = %q, want /v1/models (no duplicate suffix)", gotPath)
    }
}

func TestListAIModelsErrors(t *testing.T) {
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusUnauthorized)
        w.Write([]byte(`{"error":{"message":"bad key"}}`))
    }))
    defer srv.Close()

    if _, err := listAIModels(context.Background(), srv.URL, "sk-test", 10); err == nil {
        t.Fatal("expected error on 401")
    }
    if _, err := listAIModels(context.Background(), "", "sk-test", 10); err == nil {
        t.Fatal("expected error on empty base url")
    }
    if _, err := listAIModels(context.Background(), srv.URL, "", 10); err == nil {
        t.Fatal("expected error on empty key")
    }
}

func TestRecognizeKindPrompts(t *testing.T) {
    boxSys := recognizeKindSystemPrompt(RecognizeKindBox)
    if !strings.Contains(boxSys, "包装盒") {
        t.Fatalf("box system prompt missing 包装盒: %q", boxSys)
    }
    receiptText := recognizeKindUserText(RecognizeKindReceipt)
    if !strings.Contains(receiptText, "逐条提取") {
        t.Fatalf("receipt user text missing 逐条提取: %q", receiptText)
    }
    // 三种类型的提示词必须彼此不同，避免串味。
    seen := map[string]bool{}
    for _, k := range []string{RecognizeKindPrescription, RecognizeKindBox, RecognizeKindReceipt} {
        p := recognizeKindUserText(k)
        if seen[p] {
            t.Fatalf("kind %s shares prompt with another kind", k)
        }
        seen[p] = true
    }
}

func TestNormalizeRecognizeKind(t *testing.T) {
    cases := map[string]string{
        "":             RecognizeKindPrescription,
        "box":          RecognizeKindBox,
        "receipt":      RecognizeKindReceipt,
        "  box  ":      RecognizeKindBox,
        "prescription": RecognizeKindPrescription,
        "unknown":      RecognizeKindPrescription,
    }
    for in, want := range cases {
        if got := normalizeRecognizeKind(in); got != want {
            t.Fatalf("normalizeRecognizeKind(%q) = %q, want %q", in, got, want)
        }
    }
}
