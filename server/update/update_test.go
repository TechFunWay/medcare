package update

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestCheckUsesReleaseChannels(t *testing.T) {
	// 未设置 UPDATE_CHECK_URL 时必须走发行版渠道层（Gitee → GitHub），
	// 这样飞牛 NAS 上 GitHub 不可达时仍能查到 Gitee 上的新版本。
	// 这里用 UPDATE_SOURCES 的自定义渠道替身（优先于内置渠道），不出外网。
	InvalidateCache()
	InvalidateFetchCache()
	t.Setenv("UPDATE_CHECK_URL", "")
	body := `{"tag_name":"v9.9.9","name":"v9.9.9","body":"更新说明","assets":[{"name":"techfunway-smallgo_v9.9.9_x86.fpk","size":10,"browser_download_url":"https://example.com/fpk"}]}`
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	defer fake.Close()
	t.Setenv("UPDATE_SOURCES", fake.URL)

	got := Check("v0.1.2")
	if !got.HasUpdate || got.Latest != "v9.9.9" {
		t.Fatalf("Check() = %+v", got)
	}
	// 自定义渠道没有固定发行版页面，退化到附件地址
	if got.DownloadURL != "https://example.com/fpk" {
		t.Fatalf("download url = %q", got.DownloadURL)
	}

	InvalidateFetchCache()
}

func TestIsNewer(t *testing.T) {
	cases := []struct {
		latest, current string
		want            bool
	}{
		{"v0.2.0", "v0.1.2", true},
		{"0.2.0", "0.1.99", true},
		{"v1.0.0", "v1.0.0", false},
		{"v1.0.0", "v1.0.1", false},
		{"v1.0", "v1.0.0", false}, // 缺省段补 0
		{"v1.0.0", "v1.0", false}, // 缺省段补 0
		{"v2.0", "v1.9.9", true},  // 段数不同也能比较
		{"", "v1.0.0", false},
		{"v1.0.0", "", false},
		{"release-2026", "v1.0.0", false}, // 非数字前缀按 0
		{"v0.1.2-beta", "v0.1.1", true},   // 取数字段
	}
	for _, c := range cases {
		if got := IsNewer(c.latest, c.current); got != c.want {
			t.Errorf("IsNewer(%q, %q) = %v, want %v", c.latest, c.current, got, c.want)
		}
	}
}

func TestCheckWithFakeReleaseServer(t *testing.T) {
	gin.SetMode(gin.TestMode)

	newFake := func(t *testing.T, tag string, calls *atomic.Int32) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			calls.Add(1)
			_ = json.NewEncoder(w).Encode(githubRelease{TagName: tag, HTMLURL: "https://example.com/rel"})
		}))
	}

	t.Run("has update", func(t *testing.T) {
		InvalidateCache()
		var calls atomic.Int32
		fake := newFake(t, "v9.9.9", &calls)
		defer fake.Close()
		t.Setenv("UPDATE_CHECK_URL", fake.URL)

		got := Check("v0.1.2")
		if !got.HasUpdate || got.Latest != "v9.9.9" {
			t.Fatalf("Check() = %+v", got)
		}
		if got.DownloadURL != "https://example.com/rel" {
			t.Fatalf("download url = %q", got.DownloadURL)
		}

		// 缓存生效：第二次调用不再外呼
		Check("v0.1.2")
		if calls.Load() != 1 {
			t.Fatalf("cache miss: upstream calls = %d, want 1", calls.Load())
		}
	})

	t.Run("no update", func(t *testing.T) {
		InvalidateCache()
		var calls atomic.Int32
		fake := newFake(t, "v0.0.1", &calls)
		defer fake.Close()
		t.Setenv("UPDATE_CHECK_URL", fake.URL)

		if got := Check("v1.0.0"); got.HasUpdate {
			t.Fatalf("Check() = %+v, want no update", got)
		}
	})

	t.Run("upstream failure degrades gracefully", func(t *testing.T) {
		InvalidateCache()
		fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusForbidden) // GitHub API 限流常见
		}))
		defer fake.Close()
		t.Setenv("UPDATE_CHECK_URL", fake.URL)

		got := Check("v1.0.0")
		if got.HasUpdate || got.Current != "v1.0.0" {
			t.Fatalf("Check() = %+v, want graceful degradation", got)
		}
	})

	t.Run("route responds", func(t *testing.T) {
		InvalidateCache()
		var calls atomic.Int32
		fake := newFake(t, "v9.9.9", &calls)
		defer fake.Close()
		t.Setenv("UPDATE_CHECK_URL", fake.URL)

		r := gin.New()
		RegisterRoutes(r.Group("/api"))
		resp := httptest.NewRecorder()
		r.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/api/version/check", nil))
		if resp.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %s", resp.Code, resp.Body.String())
		}
		var body struct {
			Code int `json:"code"`
			Data struct {
				HasUpdate bool `json:"has_update"`
			} `json:"data"`
		}
		if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if body.Code != 0 || !body.Data.HasUpdate {
			t.Fatalf("body = %s", resp.Body.String())
		}
	})
}
