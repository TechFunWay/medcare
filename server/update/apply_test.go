package update

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"smallgo/server/database"
	"smallgo/server/version"
)

// setupFakeApp 造一个假的应用目录：二进制 + ui，返回目录路径。
func setupFakeApp(t *testing.T, exeBase string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, exeBase), []byte("old-exe-"+exeBase), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "ui", "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ui", "index.html"), []byte("old-ui"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// resetProgress 把更新进度重置回空闲，隔离测试间的状态泄漏。
func resetProgress() {
	setProgress(func(p *Progress) { *p = Progress{State: "idle"} })
}

// TestApplyPipeline 走完整更新流程：查渠道 → 下载 → 展开 → 校验 → 备份 → 替换 → 重启。
func TestApplyPipeline(t *testing.T) {
	origVersion, origApp := version.Version, version.AppName
	defer func() { version.Version, version.AppName = origVersion, origApp }()
	version.Version, version.AppName = "1.0.0", "techfunway-testapp"
	resetProgress()
	t.Setenv("UPDATE_REPO", "")
	t.Setenv("UPDATE_SOURCES", "")

	exeBase := "smallgo-test"
	appDir := setupFakeApp(t, exeBase)
	dataDir := t.TempDir()

	// 构造发行版：release JSON + fpk 附件（附件名按当前测试架构命名）
	archToken := "x86"
	if runtime.GOARCH == "arm64" {
		archToken = "arm64"
	}
	assetName := "techfunway-testapp_v2.0.0_" + archToken + ".fpk"
	fpkPath := filepath.Join(t.TempDir(), assetName)
	buildTestFPK(t, fpkPath, "techfunway-testapp", "2.0.0", archToken, exeBase, "new-exe",
		map[string]string{"ui/index.html": "new-ui", "ui/assets/app.js": "console.log('new')"})

	var srvURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/", "/latest":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"tag_name": "v2.0.0", "name": "v2.0.0", "body": "新版本",
				"assets": []map[string]any{{
					"name": assetName, "size": 100,
					"browser_download_url": srvURL + "/fpk",
				}},
			})
		case "/fpk":
			data, _ := os.ReadFile(fpkPath)
			_, _ = w.Write(data)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	srvURL = srv.URL
	t.Setenv("UPDATE_SOURCES", srv.URL)

	exePath := filepath.Join(appDir, exeBase)
	origExeFn, origRestart := exePathFn, restartFn
	exePathFn = func() (string, error) { return exePath, nil }
	restarted := false
	restartFn = func() error { restarted = true; return nil }
	defer func() { exePathFn, restartFn = origExeFn, origRestart }()
	InvalidateFetchCache()

	if err := Apply(func(string) (string, error) { return "", os.ErrNotExist }, dataDir, false); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if !restarted {
		t.Fatal("更新完成后应触发重启")
	}
	// 二进制与 ui 已替换
	if got, _ := os.ReadFile(exePath); string(got) != "new-exe" {
		t.Fatalf("二进制未替换: %q", got)
	}
	if got, _ := os.ReadFile(filepath.Join(appDir, "ui", "index.html")); string(got) != "new-ui" {
		t.Fatalf("ui 未替换: %q", got)
	}
	if got, _ := os.ReadFile(filepath.Join(appDir, "ui", "assets", "app.js")); string(got) != "console.log('new')" {
		t.Fatalf("ui 子目录未替换: %q", got)
	}
	// 备份保留旧版本
	if got, err := os.ReadFile(filepath.Join(dataDir, "updates", "backup", exeBase)); err != nil || string(got) != "old-exe-"+exeBase {
		t.Fatalf("备份缺少旧二进制: %v %q", err, got)
	}
	if got, err := os.ReadFile(filepath.Join(dataDir, "updates", "backup", "ui", "index.html")); err != nil || string(got) != "old-ui" {
		t.Fatalf("备份缺少旧 ui: %v %q", err, got)
	}
	// pending 标记已写，等待重启后结算
	raw, err := os.ReadFile(filepath.Join(dataDir, "updates", "pending.json"))
	if err != nil {
		t.Fatal(err)
	}
	var pending pendingUpdate
	if err := json.Unmarshal(raw, &pending); err != nil || pending.To != "2.0.0" || pending.From != "1.0.0" {
		t.Fatalf("pending = %s, %+v", raw, pending)
	}
	// 暂存与下载残留应清理
	if _, err := os.Stat(filepath.Join(appDir, ".update-stage")); !os.IsNotExist(err) {
		t.Fatal("暂存目录应清理")
	}
}

// TestApplyNoUpdate 已是最新版本时不动作。
func TestApplyNoUpdate(t *testing.T) {
	origVersion := version.Version
	defer func() { version.Version = origVersion }()
	version.Version = "9.9.9"
	resetProgress()

	dataDir := t.TempDir()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"tag_name": "v1.0.0", "assets": []map[string]any{},
		})
	}))
	defer srv.Close()
	t.Setenv("UPDATE_SOURCES", srv.URL)
	InvalidateFetchCache()

	err := Apply(func(string) (string, error) { return "", os.ErrNotExist }, dataDir, false)
	if err == nil || err.Error() != ErrNoUpdate.Error() {
		t.Fatalf("应返回 ErrNoUpdate，得到 %v", err)
	}
	if p := GetProgress(); p.State != "idle" {
		t.Fatalf("无更新时状态应回到 idle，得到 %s", p.State)
	}
}

// TestApplyAppMismatch 包名不匹配的 fpk 必须被拒绝且不改动现有文件。
func TestApplyAppMismatch(t *testing.T) {
	origVersion, origApp := version.Version, version.AppName
	defer func() { version.Version, version.AppName = origVersion, origApp }()
	version.Version, version.AppName = "1.0.0", "techfunway-testapp"
	resetProgress()

	exeBase := "smallgo-test"
	appDir := setupFakeApp(t, exeBase)
	dataDir := t.TempDir()

	fpkPath := filepath.Join(t.TempDir(), "other_v2.0.0_x86.fpk")
	buildTestFPK(t, fpkPath, "techfunway-otherapp", "2.0.0", "x86", exeBase, "evil",
		map[string]string{"ui/index.html": "evil"})

	var srvURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/latest":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"tag_name": "v2.0.0",
				"assets": []map[string]any{{
					"name": "other_v2.0.0_x86.fpk", "size": 1,
					"browser_download_url": srvURL + "/fpk",
				}},
			})
		case "/fpk":
			data, _ := os.ReadFile(fpkPath)
			_, _ = w.Write(data)
		}
	}))
	defer srv.Close()
	srvURL = srv.URL

	exePath := filepath.Join(appDir, exeBase)
	origExeFn, origRestart := exePathFn, restartFn
	exePathFn = func() (string, error) { return exePath, nil }
	restartFn = func() error { return nil }
	defer func() { exePathFn, restartFn = origExeFn, origRestart }()
	InvalidateFetchCache()

	if err := Apply(func(string) (string, error) { return "", os.ErrNotExist }, dataDir, false); err == nil {
		t.Fatal("包名不匹配应报错")
	}
	if got, _ := os.ReadFile(exePath); string(got) != "old-exe-"+exeBase {
		t.Fatalf("不匹配的包不应改动现有文件: %q", got)
	}
	if p := GetProgress(); p.State != "failed" {
		t.Fatalf("失败后状态应为 failed，得到 %s", p.State)
	}
}

// TestSettlePendingUpdate 重启后按 pending 标记结算更新记录。
func TestSettlePendingUpdate(t *testing.T) {
	origVersion := version.Version
	defer func() { version.Version = origVersion }()
	version.Version = "2.0.0"

	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := database.InitDB(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer database.CloseDB(db)
	if err := db.AutoMigrate(&UpdateRecord{}); err != nil {
		t.Fatal(err)
	}

	dataDir := t.TempDir()
	rec := UpdateRecord{FromVersion: "1.0.0", ToVersion: "2.0.0", Status: "applying", CreatedAt: time.Now()}
	if err := db.Create(&rec).Error; err != nil {
		t.Fatal(err)
	}
	if err := writePending(dataDir, &pendingUpdate{RecordID: rec.ID, From: "1.0.0", To: "2.0.0", StartedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}

	SettlePendingUpdate(db, dataDir)
	var got UpdateRecord
	if err := db.First(&got, rec.ID).Error; err != nil {
		t.Fatal(err)
	}
	if got.Status != "completed" {
		t.Fatalf("版本一致应结算为 completed，得到 %s（%s）", got.Status, got.Error)
	}
	if _, err := os.Stat(filepath.Join(dataDir, "updates", "pending.json")); !os.IsNotExist(err) {
		t.Fatal("pending 应被清理")
	}

	// 版本不一致 → failed
	version.Version = "1.0.0"
	rec2 := UpdateRecord{FromVersion: "1.0.0", ToVersion: "2.0.0", Status: "applying", CreatedAt: time.Now()}
	db.Create(&rec2)
	if err := writePending(dataDir, &pendingUpdate{RecordID: rec2.ID, From: "1.0.0", To: "2.0.0", StartedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	SettlePendingUpdate(db, dataDir)
	var got2 UpdateRecord
	db.First(&got2, rec2.ID)
	if got2.Status != "failed" {
		t.Fatalf("版本不一致应结算为 failed，得到 %s", got2.Status)
	}
}

// TestAutoUpdateEnabled 配置缺省开启，false 可关闭。
func TestAutoUpdateEnabled(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := database.InitDB(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer database.CloseDB(db)
	if err := db.AutoMigrate(&database.SystemConfig{}, &UpdateRecord{}); err != nil {
		t.Fatal(err)
	}

	if !autoUpdateEnabled(db) {
		t.Fatal("缺省应开启")
	}
	if err := db.Create(&database.SystemConfig{UserID: 0, Key: "auto_update_enabled", Value: "false"}).Error; err != nil {
		t.Fatal(err)
	}
	if autoUpdateEnabled(db) {
		t.Fatal("false 应关闭")
	}
}
