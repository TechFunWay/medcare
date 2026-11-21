// Package update 提供「检查新版本」能力：查询发布渠道的最新版本，
// 与当前运行版本做 semver 比较，供前端展示升级提示条。
//
// 任何检查失败都必须优雅降级（返回 has_update=false），绝不影响正常使用。
//
// 渠道口径与「应用自更新」（release.go）保持一致：默认按 Gitee → GitHub 逐个尝试。
// 飞牛 NAS 普遍访问不了 GitHub，早期只用 GitHub 时 NAS 上永远查不到新版本，
// 前端的升级提示条也就永远不出现。UPDATE_CHECK_URL 设置时改走该单一渠道
// （测试与自建渠道），结果缓存 24 小时。
package update

import (
	"encoding/json"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"smallgo/server/response"
	"smallgo/server/version"
)

const cacheTTL = 24 * time.Hour

var httpClient = &http.Client{Timeout: 5 * time.Second}

// CheckURL 返回发布渠道的最新版本查询地址。
// UPDATE_CHECK_URL 环境变量可覆盖（默认 GitHub Releases；自建发布渠道或测试时覆盖）。
func CheckURL() string {
	if u := os.Getenv("UPDATE_CHECK_URL"); u != "" {
		return u
	}
	return "https://api.github.com/repos/TechFunWay/smallgo/releases/latest"
}

type cacheEntry struct {
	latest    string
	download  string
	checkedAt time.Time
}

var (
	mu    sync.Mutex
	cache *cacheEntry
)

// githubRelease 是发布渠道响应中用到的字段。
type githubRelease struct {
	TagName string `json:"tag_name"`
	HTMLURL string `json:"html_url"`
}

// Check 查询最新版本并比较。任何错误都返回 has_update=false 的结果。
func Check(current string) Info {
	// 显式指定单一渠道（测试 / 自建渠道）时保持原行为。
	if os.Getenv("UPDATE_CHECK_URL") != "" {
		return checkSingleChannel(CheckURL(), current)
	}

	// 默认走发行版渠道层：Gitee → GitHub（UPDATE_SOURCES 自定义渠道优先），
	// 由 FetchLatest 做 1 小时缓存，NAS 上 GitHub 不可达时仍能查到 Gitee 上的版本。
	rel, err := FetchLatest(nil, false)
	if err != nil || rel == nil || rel.Tag == "" {
		return Info{Current: current}
	}
	return Info{
		Current:     current,
		Latest:      rel.Tag,
		DownloadURL: rel.PageURL(),
		HasUpdate:   IsNewer(rel.Tag, current),
	}
}

// checkSingleChannel 走 UPDATE_CHECK_URL 指定的单一渠道，结果缓存 24 小时。
func checkSingleChannel(url, current string) Info {
	mu.Lock()
	defer mu.Unlock()
	if cache != nil && time.Since(cache.checkedAt) < cacheTTL {
		return Info{
			Current:     current,
			Latest:      cache.latest,
			DownloadURL: cache.download,
			HasUpdate:   IsNewer(cache.latest, current),
		}
	}

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return Info{Current: current}
	}
	req.Header.Set("User-Agent", version.AppName+"/"+current)
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return Info{Current: current}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Info{Current: current}
	}

	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil || release.TagName == "" {
		return Info{Current: current}
	}

	cache = &cacheEntry{latest: release.TagName, download: release.HTMLURL, checkedAt: time.Now()}
	return Info{
		Current:     current,
		Latest:      release.TagName,
		DownloadURL: release.HTMLURL,
		HasUpdate:   IsNewer(release.TagName, current),
	}
}

// Info 是版本检查的 API 视图。
type Info struct {
	Current     string `json:"current"`
	Latest      string `json:"latest"`
	DownloadURL string `json:"download_url"`
	HasUpdate   bool   `json:"has_update"`
}

// RegisterRoutes 挂载需要登录的版本检查端点。
func RegisterRoutes(auth *gin.RouterGroup) {
	auth.GET("/version/check", func(c *gin.Context) {
		response.Success(c, Check(version.Version))
	})
}

var semverPart = regexp.MustCompile(`^\d+`)

// ParseVersion 把 "v1.2.3" 解析为 [1,2,3]，非数字段按 0 处理。
func ParseVersion(v string) [3]int {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	parts := strings.SplitN(v, ".", 3)
	var nums [3]int
	for i, p := range parts {
		if m := semverPart.FindString(p); m != "" {
			n, err := strconv.Atoi(m)
			if err == nil {
				nums[i] = n
			}
		}
	}
	return nums
}

// IsNewer 判断 latest 是否比 current 更新（逐位比较，缺省段补 0）。
func IsNewer(latest, current string) bool {
	if latest == "" || current == "" {
		return false
	}
	l, c := ParseVersion(latest), ParseVersion(current)
	for i := 0; i < 3; i++ {
		if l[i] != c[i] {
			return l[i] > c[i]
		}
	}
	return false
}

// InvalidateCache 清空检查缓存（测试用）。
func InvalidateCache() {
	mu.Lock()
	defer mu.Unlock()
	cache = nil
}
