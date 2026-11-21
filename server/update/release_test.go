package update

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"testing"

	"smallgo/server/version"
)

func TestPickFPKAsset(t *testing.T) {
	assets := []Asset{
		{Name: "techfunway-testapp_v2.0.0_linux_amd64.tar.gz", URL: "http://x/1", Size: 1},
		{Name: "techfunway-testapp-v2.0.0-fnos-amd64.fpk", URL: "http://x/2", Size: 2},
		{Name: "techfunway-testapp_v2.0.0_x86.fpk", URL: "http://x/3", Size: 3},
		{Name: "techfunway-testapp_v2.0.0_arm64.fpk", URL: "http://x/4", Size: 4},
		{Name: "techfunway-testapp-v2.0.0-multiarch.oci.tar", URL: "http://x/5", Size: 5},
	}
	switch runtime.GOARCH {
	case "amd64":
		a, err := PickFPKAsset(assets)
		if err != nil {
			t.Fatal(err)
		}
		// x86 命名优先于旧式 amd64 命名
		if a.Name != "techfunway-testapp_v2.0.0_x86.fpk" {
			t.Fatalf("amd64 应选 x86 命名的 fpk，选了 %s", a.Name)
		}
	case "arm64":
		a, err := PickFPKAsset(assets)
		if err != nil {
			t.Fatal(err)
		}
		if a.Name != "techfunway-testapp_v2.0.0_arm64.fpk" {
			t.Fatalf("arm64 应选 arm64 fpk，选了 %s", a.Name)
		}
	}
	if _, err := PickFPKAsset([]Asset{{Name: "only.tar.gz", URL: "http://x", Size: 1}}); err == nil {
		t.Fatal("没有 fpk 附件应报错")
	}
}

func TestRepoName(t *testing.T) {
	orig := version.AppName
	defer func() { version.AppName = orig }()

	t.Setenv("UPDATE_REPO", "")
	version.AppName = "techfunway-smallgo"
	if RepoName() != "smallgo" {
		t.Fatalf("RepoName() = %q", RepoName())
	}
	version.AppName = "techfunway.bookmarks"
	if RepoName() != "bookmarks" {
		t.Fatalf("点号包名应剥成 bookmarks，得到 %q", RepoName())
	}
	t.Setenv("UPDATE_REPO", "custom-repo")
	if RepoName() != "custom-repo" {
		t.Fatalf("UPDATE_REPO 应生效，得到 %q", RepoName())
	}
}

func TestChannelOrder(t *testing.T) {
	if got := channelOrder(nil); len(got) != 2 || got[0] != "gitee" || got[1] != "github" {
		t.Fatalf("默认渠道顺序 = %v", got)
	}
	lookup := func(string) (string, error) { return "github", nil }
	if got := channelOrder(lookup); len(got) != 1 || got[0] != "github" {
		t.Fatalf("github 单选 = %v", got)
	}
}

func TestFetchLatestCustomSourceAndCache(t *testing.T) {
	InvalidateFetchCache()
	var calls int
	var srvURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		_ = json.NewEncoder(w).Encode(map[string]any{
			"tag_name": "v2.0.0",
			"name":     "测试 v2.0.0",
			"body":     "更新说明",
			"assets": []map[string]any{
				{"name": "app_v2.0.0_x86.fpk", "size": 10, "browser_download_url": srvURL + "/app.fpk"},
			},
		})
	}))
	defer srv.Close()
	srvURL = srv.URL
	t.Setenv("UPDATE_SOURCES", srv.URL+", ,http://invalid.invalid")

	rel, err := FetchLatest(func(string) (string, error) { return "", os.ErrNotExist }, true)
	if err != nil {
		t.Fatal(err)
	}
	if rel.Tag != "v2.0.0" || rel.Source != "custom" || len(rel.Assets) != 1 {
		t.Fatalf("release = %+v", rel)
	}
	// 缓存生效：第二次不再外呼
	if _, err = FetchLatest(nil, false); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("缓存未生效，上游被调了 %d 次", calls)
	}
}

func TestReleasePageURL(t *testing.T) {
	orig := version.AppName
	defer func() { version.AppName = orig }()
	t.Setenv("UPDATE_REPO", "")
	version.AppName = "techfunway-smallgo"

	cases := []struct {
		source string
		tag    string
		want   string
	}{
		{"gitee", "v1.2.3", "https://gitee.com/TechFunWay/smallgo/releases/tag/v1.2.3"},
		{"github", "v1.2.3", "https://github.com/TechFunWay/smallgo/releases/tag/v1.2.3"},
	}
	for _, c := range cases {
		rel := &Release{Tag: c.tag, Source: c.source}
		if got := rel.PageURL(); got != c.want {
			t.Fatalf("PageURL(%s) = %q, want %q", c.source, got, c.want)
		}
	}

	// 自定义渠道没有固定发行版页面，退化到第一个附件地址；无附件时返回空串
	custom := &Release{Tag: "v1.0.0", Source: "custom", Assets: []Asset{{Name: "a.fpk", URL: "https://example.com/a.fpk"}}}
	if got := custom.PageURL(); got != "https://example.com/a.fpk" {
		t.Fatalf("自定义渠道 PageURL() = %q", got)
	}
	if got := (&Release{Source: "custom"}).PageURL(); got != "" {
		t.Fatalf("无附件 PageURL() = %q, want 空串", got)
	}
}

func TestFetchLatestAllChannelsFail(t *testing.T) {
	InvalidateFetchCache()
	t.Setenv("UPDATE_SOURCES", "http://invalid.invalid")
	lookup := func(string) (string, error) { return "github", nil }
	if _, err := FetchLatest(lookup, true); err == nil {
		t.Fatal("全部渠道失败应返回错误")
	}
}
