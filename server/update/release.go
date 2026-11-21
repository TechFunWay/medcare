package update

// 发行版渠道层：应用更新文件统一从 GitHub 与 Gitee 的同名发版仓库
// （TechFunWay/<应用>）的 Release 附件获取，fpk 安装包本身就是更新包。
//
// NAS 多数访问不了 GitHub，Gitee 是主要通路：默认先 Gitee 后 GitHub 逐个尝试，
// 任一渠道成功即用；UPDATE_SOURCES 可插入自定义渠道（返回 GitHub 格式的
// release JSON 的任意 URL），供自建渠道与测试使用。

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"smallgo/server/version"
)

// ConfigLookup 按键读取系统配置；生产实现是 sysconfig.GetConfig(db, key, 0)，
// 测试里用内存闭包代替。
type ConfigLookup func(key string) (string, error)

const (
	giteeAPIBase  = "https://gitee.com/api/v5/repos/TechFunWay"
	githubAPIBase = "https://api.github.com/repos/TechFunWay"
	giteeWebBase  = "https://gitee.com/TechFunWay"
	githubWebBase = "https://github.com/TechFunWay"

	// fetchCacheTTL 发行版信息的缓存时长。自动检查每天至多外呼几次即可。
	fetchCacheTTL = time.Hour
)

// checkClient 只用于查询发行版元数据，超时要短：GitHub 不可达时不能拖慢接口。
var checkClient = &http.Client{Timeout: 8 * time.Second}

// Asset 是发行版附件的统一视图。
type Asset struct {
	Name string `json:"name"`
	URL  string `json:"url"`
	Size int64  `json:"size"`
}

// Release 是发行版的统一视图，来源字段记录命中了哪个渠道。
type Release struct {
	Tag       string    `json:"tag"`
	Name      string    `json:"name"`
	Notes     string    `json:"notes"`
	Source    string    `json:"source"`
	Assets    []Asset   `json:"assets"`
	FetchedAt time.Time `json:"fetched_at"`
}

// PageURL 返回「前往下载」应跳转的发行版页面地址。
// 内置渠道按仓库名 + 版本号拼页面地址；自定义渠道没有固定页面形态，
// 退化到第一个附件地址（通常是 fpk 下载直链）。
func (r *Release) PageURL() string {
	if r == nil {
		return ""
	}
	repo := RepoName()
	switch r.Source {
	case "gitee":
		return giteeWebBase + "/" + repo + "/releases/tag/" + r.Tag
	case "github":
		return githubWebBase + "/" + repo + "/releases/tag/" + r.Tag
	}
	for _, a := range r.Assets {
		if a.URL != "" {
			return a.URL
		}
	}
	return ""
}

// repoName 返回发版仓库名：默认从包名推导（techfunway-<应用> → <应用>，
// bookmarks 的点号形式同样剥掉），UPDATE_REPO 环境变量可覆盖。
func RepoName() string {
	if r := strings.TrimSpace(os.Getenv("UPDATE_REPO")); r != "" {
		return r
	}
	name := version.AppName
	name = strings.TrimPrefix(name, "techfunway-")
	name = strings.TrimPrefix(name, "techfunway.")
	return name
}

// channelRelease 是 Gitee / GitHub releases/latest 响应中共有的字段，
// 两家 API 的这几个字段名一致，可共用一套解析。
type channelRelease struct {
	TagName string `json:"tag_name"`
	Name    string `json:"name"`
	Body    string `json:"body"`
	Assets  []struct {
		Name               string `json:"name"`
		Size               int64  `json:"size"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

func fetchChannel(client *http.Client, url, sourceName string) (*Release, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", version.AppName+"/"+version.Version)
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s 返回 %d", sourceName, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	var cr channelRelease
	if err := json.Unmarshal(body, &cr); err != nil {
		return nil, fmt.Errorf("%s 响应解析失败: %w", sourceName, err)
	}
	if cr.TagName == "" {
		return nil, fmt.Errorf("%s 响应缺少 tag_name", sourceName)
	}
	rel := &Release{
		Tag:       cr.TagName,
		Name:      cr.Name,
		Notes:     cr.Body,
		Source:    sourceName,
		FetchedAt: time.Now(),
	}
	for _, a := range cr.Assets {
		if a.BrowserDownloadURL == "" {
			continue
		}
		rel.Assets = append(rel.Assets, Asset{Name: a.Name, URL: a.BrowserDownloadURL, Size: a.Size})
	}
	return rel, nil
}

// customSources 返回 UPDATE_SOURCES 里配置的自定义渠道（逗号分隔的 URL，
// 每个都要返回 GitHub 格式的 release JSON）。
func customSources() []string {
	var urls []string
	for _, u := range strings.Split(os.Getenv("UPDATE_SOURCES"), ",") {
		if u = strings.TrimSpace(u); u != "" {
			urls = append(urls, u)
		}
	}
	return urls
}

// channelOrder 按系统配置 update_channel 决定内置渠道顺序：
// auto（默认）→ gitee、github；也接受 gitee / github 单选。
// UPDATE_SOURCES 的自定义渠道无论配置如何都排在最前。
func channelOrder(lookup ConfigLookup) (names []string) {
	names = []string{"gitee", "github"}
	if lookup != nil {
		if v, err := lookup("update_channel"); err == nil {
			switch strings.TrimSpace(v) {
			case "gitee":
				names = []string{"gitee"}
			case "github":
				names = []string{"github"}
			}
		}
	}
	return names
}

func fetchChannelByName(name string) (*Release, error) {
	repo := RepoName()
	switch name {
	case "gitee":
		return fetchChannel(checkClient, giteeAPIBase+"/"+repo+"/releases/latest", "gitee")
	case "github":
		return fetchChannel(checkClient, githubAPIBase+"/"+repo+"/releases/latest", "github")
	default:
		return nil, fmt.Errorf("未知渠道 %q", name)
	}
}

var (
	fetchMu       sync.Mutex
	fetchCache    *Release
	fetchCacheAt  time.Time
	fetchCacheErr error
)

// FetchLatest 查询最新发行版，带 1 小时缓存；force 时绕过缓存重新外呼。
// 依次尝试自定义渠道与内置渠道，全部失败才返回错误。
func FetchLatest(lookup ConfigLookup, force bool) (*Release, error) {
	fetchMu.Lock()
	defer fetchMu.Unlock()
	if !force && fetchCache != nil && time.Since(fetchCacheAt) < fetchCacheTTL {
		return fetchCache, fetchCacheErr
	}

	var errs []string
	try := func(rel *Release, err error) bool {
		if err != nil {
			errs = append(errs, err.Error())
			return false
		}
		fetchCache, fetchCacheAt, fetchCacheErr = rel, time.Now(), nil
		return true
	}

	// 自定义渠道（UPDATE_SOURCES）最优先：自建发布通道与测试环境用。
	for _, u := range customSources() {
		if try(fetchChannel(checkClient, u, "custom")) {
			return fetchCache, nil
		}
	}
	for _, name := range channelOrder(lookup) {
		if try(fetchChannelByName(name)) {
			return fetchCache, nil
		}
	}

	fetchCache, fetchCacheErr = nil, fmt.Errorf("发行版渠道全部不可达: %s", strings.Join(errs, "；"))
	return nil, fetchCacheErr
}

// PickFPKAsset 从发行版附件里挑出当前平台架构的 fpk 安装包。
// 产物命名两代并存（<包名>_v<版本>_x86.fpk 与旧式 fnos-amd64），
// 因此按架构令牌匹配而不是按完整文件名：x86 优先于 amd64。
// InvalidateFetchCache 清空发行版缓存（测试用）。
func InvalidateFetchCache() {
	fetchMu.Lock()
	defer fetchMu.Unlock()
	fetchCache = nil
	fetchCacheErr = nil
}

func PickFPKAsset(assets []Asset) (*Asset, error) {
	var tokens []string
	switch runtime.GOARCH {
	case "amd64":
		tokens = []string{"x86", "amd64"}
	case "arm64":
		tokens = []string{"arm64"}
	default:
		return nil, fmt.Errorf("架构 %s 没有对应的 fpk 产物", runtime.GOARCH)
	}
	var fpks []Asset
	for _, a := range assets {
		if strings.HasSuffix(strings.ToLower(a.Name), ".fpk") {
			fpks = append(fpks, a)
		}
	}
	if len(fpks) == 0 {
		return nil, fmt.Errorf("发行版附件里没有 fpk 安装包")
	}
	for _, token := range tokens {
		for i := range fpks {
			if strings.Contains(strings.ToLower(fpks[i].Name), token) {
				return &fpks[i], nil
			}
		}
	}
	return nil, fmt.Errorf("发行版附件里没有 %s 架构的 fpk（找到：%s）",
		runtime.GOARCH, assetNames(fpks))
}

func assetNames(assets []Asset) string {
	names := make([]string, 0, len(assets))
	for _, a := range assets {
		names = append(names, a.Name)
	}
	return strings.Join(names, ", ")
}
