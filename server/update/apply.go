package update

// 应用更新管线：查发行版 → 挑 fpk → 下载校验 → 展开暂存 → 备份 →
// 原子替换二进制与 ui → 原地重启（syscall.Exec，PID 不变，飞牛监管无感）。
//
// 安全边界：
//   - 只替换运行所需的两样（二进制 + ui/），其余文件仍以安装包为准；
//   - 替换前把当前版本完整备份到数据目录 updates/backup/，替换失败自动回滚；
//   - 下载产物用 fpk 内嵌 manifest 校验包名与版本，发行版带 SHA256SUMS 时再做哈希校验；
//   - 暂存目录放在应用目录内（.update-stage），保证替换用的 rename 在同一文件系统上原子完成。
import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"

	"smallgo/server/config"
	"smallgo/server/database"
	"smallgo/server/logger"
	"smallgo/server/scheduler"
	"smallgo/server/sysconfig"
	"smallgo/server/version"
)

// ErrNoUpdate 当前已是最新版本，无需更新。
var ErrNoUpdate = errors.New("已是最新版本")

// ErrInProgress 已有更新任务在执行。
var ErrInProgress = errors.New("已有更新任务在执行中")

// UpdateRecord 记录一次自更新的全过程，供管理端查看历史与排查失败。
type UpdateRecord struct {
	ID          uint       `gorm:"primaryKey" json:"id"`
	FromVersion string     `json:"from_version"`
	ToVersion   string     `json:"to_version"`
	Source      string     `json:"source"`
	Asset       string     `json:"asset"`
	Status      string     `json:"status"` // applying / completed / failed
	Error       string     `json:"error,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

// Progress 是更新任务的实时状态，管理端轮询展示。
type Progress struct {
	State       string    `json:"state"` // idle/checking/downloading/staging/backing_up/swapping/restarting/done/failed
	Stage       string    `json:"stage"`
	Error       string    `json:"error,omitempty"`
	Auto        bool      `json:"auto"`
	FromVersion string    `json:"from_version,omitempty"`
	ToVersion   string    `json:"to_version,omitempty"`
	Source      string    `json:"source,omitempty"`
	Asset       string    `json:"asset,omitempty"`
	Downloaded  int64     `json:"downloaded"`
	Total       int64     `json:"total"`
	StartedAt   time.Time `json:"started_at,omitempty"`
	UpdatedAt   time.Time `json:"updated_at"`
}

var (
	applyMu sync.Mutex
	pMu     sync.RWMutex
	pState  = Progress{State: "idle"}
)

func getProgress() Progress {
	pMu.RLock()
	defer pMu.RUnlock()
	return pState
}

func setProgress(mutate func(*Progress)) {
	pMu.Lock()
	defer pMu.Unlock()
	mutate(&pState)
	pState.UpdatedAt = time.Now()
}

// ---- 运行时依赖（由 RegisterAdminRoutes 注入，供调度任务使用） ----

var (
	dbRef   *gorm.DB
	dataRef string
)

func configLookup(db *gorm.DB) ConfigLookup {
	return func(key string) (string, error) { return sysconfig.GetConfig(db, key, 0) }
}

// ---- 下载与校验 ----

var downloadClient = &http.Client{Timeout: 30 * time.Minute}

// downloadFile 下载 url 到 dest（先写 .part 再改名），回调汇报进度。
func downloadFile(url, dest string, cb func(done, total int64)) error {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", version.AppName+"/"+version.Version)
	resp, err := downloadClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("下载返回 %d", resp.StatusCode)
	}
	total := resp.ContentLength
	tmp := dest + ".part"
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	buf := make([]byte, 256<<10)
	var done int64
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := out.Write(buf[:n]); werr != nil {
				out.Close()
				os.Remove(tmp)
				return werr
			}
			done += int64(n)
			if cb != nil {
				cb(done, total)
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			out.Close()
			os.Remove(tmp)
			return rerr
		}
	}
	if err = out.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, dest)
}

// verifySHA256 用发行版附件里的 SHA256SUMS（若存在）校验 fpk；
// 渠道没有提供校验文件时跳过（fpk 内嵌 manifest 的包名/版本校验仍然生效）。
func verifySHA256(rel *Release, assetName, fpkPath string) error {
	var sums *Asset
	for i := range rel.Assets {
		if strings.EqualFold(rel.Assets[i].Name, "SHA256SUMS") {
			sums = &rel.Assets[i]
			break
		}
	}
	if sums == nil {
		return nil
	}
	sumsPath := fpkPath + ".sha256sums"
	if err := downloadFile(sums.URL, sumsPath, nil); err != nil {
		return fmt.Errorf("下载 SHA256SUMS 失败: %w", err)
	}
	defer os.Remove(sumsPath)
	data, err := os.ReadFile(sumsPath)
	if err != nil {
		return err
	}
	var want string
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) == 2 && fields[1] == assetName {
			want = strings.ToLower(fields[0])
			break
		}
	}
	if want == "" {
		return nil
	}
	f, err := os.Open(fpkPath)
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha256.New()
	if _, err = io.Copy(h, f); err != nil {
		return err
	}
	got := hex.EncodeToString(h.Sum(nil))
	if got != want {
		return fmt.Errorf("SHA256 校验失败：期望 %s，实际 %s", want, got)
	}
	return nil
}

// ---- 备份与复制 ----

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	if err != nil {
		return err
	}
	if _, err = io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

func copyDir(src, dst string) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	if err = os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	for _, e := range entries {
		s, d := filepath.Join(src, e.Name()), filepath.Join(dst, e.Name())
		if e.IsDir() {
			if err = copyDir(s, d); err != nil {
				return err
			}
			continue
		}
		if err = copyFile(s, d); err != nil {
			return err
		}
	}
	return nil
}

// ---- 环境自检 ----

// CheckItem 是环境自检的单项结论。
type CheckItem struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail"`
}

// PreflightReport 是环境自检报告，决定当前环境能不能走自更新。
type PreflightReport struct {
	OK             bool        `json:"ok"`
	DeviceType     string      `json:"device_type"`
	Goos           string      `json:"goos"`
	Goarch         string      `json:"goarch"`
	CurrentVersion string      `json:"current_version"`
	ExePath        string      `json:"exe_path"`
	ExeDir         string      `json:"exe_dir"`
	DataDir        string      `json:"data_dir"`
	Checks         []CheckItem `json:"checks"`
}

func dirWritable(dir string) error {
	probe := filepath.Join(dir, fmt.Sprintf(".update-probe-%d", time.Now().UnixNano()))
	if err := os.WriteFile(probe, []byte("probe"), 0o644); err != nil {
		return err
	}
	if err := os.Rename(probe, probe+"2"); err != nil {
		os.Remove(probe)
		return fmt.Errorf("目录不可重命名: %w", err)
	}
	os.Remove(probe + "2")
	return nil
}

// Preflight 检查当前环境是否具备自更新条件（可执行文件目录可写、ui 存在、
// 数据目录可写、磁盘余量充足）。任何一项不过都会在报告里给出原因。
func Preflight(dataDir string) *PreflightReport {
	report := &PreflightReport{
		DeviceType:     config.C.DeviceType,
		Goos:           runtime.GOOS,
		Goarch:         runtime.GOARCH,
		CurrentVersion: version.Version,
	}
	add := func(name string, err error, detail string) {
		if err != nil {
			detail = err.Error()
		}
		report.Checks = append(report.Checks, CheckItem{Name: name, OK: err == nil, Detail: detail})
		if err != nil {
			report.OK = false
		}
	}
	report.OK = true

	exePath, err := os.Executable()
	if err == nil {
		exePath, err = filepath.EvalSymlinks(exePath)
	}
	report.ExePath = exePath
	report.ExeDir = filepath.Dir(exePath)
	report.DataDir = dataDir

	add("运行程序定位", err, report.ExePath)
	if err == nil {
		err = dirWritable(report.ExeDir)
		add("应用目录可写", err, report.ExeDir+"（更新需要在此替换二进制与 ui）")
	}
	if err == nil {
		info, statErr := os.Stat(filepath.Join(report.ExeDir, "ui"))
		add("ui 目录存在", statErr, func() string {
			if statErr != nil {
				return "找不到 " + report.ExeDir + "/ui"
			}
			if !info.IsDir() {
				return report.ExeDir + "/ui 不是目录"
			}
			return report.ExeDir + "/ui"
		}())
		if statErr != nil || !info.IsDir() {
			report.OK = false
		}
	}
	if err == nil {
		updatesDir := filepath.Join(dataDir, "updates")
		err = os.MkdirAll(updatesDir, 0o755)
		add("数据目录可写", err, updatesDir)
	}
	if err == nil && runtime.GOOS != "windows" {
		var st syscallStatfsT
		if err = statfs(report.ExeDir, &st); err == nil {
			free := freeBytes(&st)
			add("磁盘余量", nil, fmt.Sprintf("应用所在卷可用约 %d MB", free>>20))
			if free < 256<<20 {
				report.OK = false
				report.Checks[len(report.Checks)-1].OK = false
				report.Checks[len(report.Checks)-1].Detail = fmt.Sprintf("可用空间不足 256MB（约 %d MB）", free>>20)
			}
		} else {
			add("磁盘余量", err, "无法查询文件系统剩余空间")
		}
	}
	if runtime.GOOS == "windows" {
		add("平台支持", errors.New("windows 不支持原地自更新"), "请下载新版 zip 覆盖安装")
	}
	return report
}

// ---- pending 结算 ----

type pendingUpdate struct {
	RecordID  uint      `json:"record_id"`
	From      string    `json:"from"`
	To        string    `json:"to"`
	StartedAt time.Time `json:"started_at"`
}

// SettlePendingUpdate 在进程启动后结算上一次自更新的结果：
// 运行版本与 pending 目标一致 → 记录完成；不一致 → 记录失败。
// 任何情况下都清掉 pending 文件，只结算一次。
func SettlePendingUpdate(db *gorm.DB, dataDir string) {
	path := filepath.Join(dataDir, "updates", "pending.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	os.Remove(path)
	var pending pendingUpdate
	if err = json.Unmarshal(data, &pending); err != nil || pending.RecordID == 0 {
		return
	}
	if normalizeVersion(version.Version) == normalizeVersion(pending.To) {
		now := time.Now()
		db.Model(&UpdateRecord{}).Where("id = ?", pending.RecordID).
			Updates(map[string]any{"status": "completed", "completed_at": &now, "error": ""})
		logger.Info("自更新完成：%s → %s", pending.From, pending.To)
		return
	}
	errMsg := fmt.Sprintf("重启后版本不匹配：期望 %s，实际 %s", pending.To, version.Version)
	db.Model(&UpdateRecord{}).Where("id = ?", pending.RecordID).
		Updates(map[string]any{"status": "failed", "error": errMsg, "completed_at": time.Now()})
	logger.Error("自更新结算失败：%s", errMsg)
}

func normalizeVersion(v string) string {
	return strings.TrimPrefix(strings.TrimSpace(v), "v")
}

// ---- 更新主流程 ----

// Apply 执行一次完整的自更新。auto 表示由定时任务自动触发（仅飞牛形态允许）。
// 同一时刻只允许一个更新任务；返回值表示任务是否被受理，
// 之后的进展通过 GetProgress 轮询。
func Apply(lookup ConfigLookup, dataDir string, auto bool) error {
	applyMu.Lock()
	defer applyMu.Unlock()

	cur := getProgress()
	if cur.State != "idle" && cur.State != "done" && cur.State != "failed" {
		return ErrInProgress
	}

	fromVersion := version.Version
	setProgress(func(p *Progress) {
		*p = Progress{State: "checking", Stage: "正在查询发行版渠道", Auto: auto,
			FromVersion: fromVersion, StartedAt: time.Now()}
	})

	rel, err := FetchLatest(lookup, true)
	if err != nil {
		return failProgress(err)
	}
	if !IsNewer(rel.Tag, fromVersion) {
		setProgress(func(p *Progress) { p.State, p.Stage = "idle", "" })
		return ErrNoUpdate
	}
	asset, err := PickFPKAsset(rel.Assets)
	if err != nil {
		return failProgress(err)
	}
	setProgress(func(p *Progress) {
		p.ToVersion, p.Source, p.Asset = normalizeVersion(rel.Tag), rel.Source, asset.Name
		p.Stage = fmt.Sprintf("发现新版本 %s（%s），开始下载 %s", rel.Tag, rel.Source, asset.Name)
	})

	record := UpdateRecord{
		FromVersion: fromVersion, ToVersion: normalizeVersion(rel.Tag),
		Source: rel.Source, Asset: asset.Name, Status: "applying", CreatedAt: time.Now(),
	}
	if dbRef != nil {
		dbRef.Create(&record)
	}

	// 下载 fpk 到数据目录
	downloadDir := filepath.Join(dataDir, "updates", "download")
	if err = os.MkdirAll(downloadDir, 0o755); err != nil {
		return failWithRecord(&record, err)
	}
	fpkPath := filepath.Join(downloadDir, asset.Name)
	err = downloadFile(asset.URL, fpkPath, func(done, total int64) {
		setProgress(func(p *Progress) { p.Downloaded, p.Total = done, total })
	})
	if err == nil {
		setProgress(func(p *Progress) { p.State, p.Stage = "verifying", "校验安装包" })
		err = verifySHA256(rel, asset.Name, fpkPath)
	}
	if err == nil {
		err = stageAndSwap(fpkPath, dataDir, &record)
	}
	if err == nil {
		// 写 pending 标记后原地重启；新进程启动时 SettlePendingUpdate 结算结果。
		if werr := writePending(dataDir, &pendingUpdate{
			RecordID: record.ID, From: fromVersion, To: record.ToVersion, StartedAt: time.Now(),
		}); werr != nil {
			logger.Error("写入更新标记失败（继续重启）：%v", werr)
		}
		setProgress(func(p *Progress) { p.State, p.Stage = "restarting", "正在重启应用" })
		if err = restartFn(); err != nil {
			rollback(record.FromVersion, dataDir)
			return failWithRecord(&record, fmt.Errorf("重启失败，已回滚: %w", err))
		}
		// 生产路径 exec 成功后不会返回；能走到这里说明是可返回的重启实现，标记完成。
		setProgress(func(p *Progress) { p.State, p.Stage, p.Error = "done", "更新完成，已重启", "" })
		return nil
	}
	return failWithRecord(&record, err)
}

// stageAndSwap 展开下载好的 fpk 并完成备份与替换。
func stageAndSwap(fpkPath, dataDir string, record *UpdateRecord) error {
	exePath, err := exePathFn()
	if err != nil {
		return fmt.Errorf("定位运行程序失败: %w", err)
	}
	exeDir := filepath.Dir(exePath)
	exeBase := filepath.Base(exePath)
	uiDir := filepath.Join(exeDir, "ui")

	setProgress(func(p *Progress) { p.State, p.Stage = "staging", "展开安装包" })
	stageDir := filepath.Join(exeDir, ".update-stage")
	os.RemoveAll(stageDir)
	if err = os.MkdirAll(stageDir, 0o755); err != nil {
		return err
	}
	manifest, err := ExtractFPK(fpkPath, stageDir, exeBase)
	if err != nil {
		os.RemoveAll(stageDir)
		return err
	}
	// 包名与版本校验：防止把别的应用的 fpk 装到自己头上。
	if manifest.Appname != "" && manifest.Appname != version.AppName {
		os.RemoveAll(stageDir)
		return fmt.Errorf("安装包应用不匹配：包内 %s，本应用 %s", manifest.Appname, version.AppName)
	}
	if normalizeVersion(manifest.Version) != normalizeVersion(record.ToVersion) {
		os.RemoveAll(stageDir)
		return fmt.Errorf("安装包版本 %s 与发行版 %s 不一致", manifest.Version, record.ToVersion)
	}

	setProgress(func(p *Progress) { p.State, p.Stage = "backing_up", "备份当前版本" })
	backupDir := filepath.Join(dataDir, "updates", "backup")
	os.RemoveAll(backupDir)
	if err = os.MkdirAll(backupDir, 0o755); err != nil {
		os.RemoveAll(stageDir)
		return err
	}
	if err = copyFile(exePath, filepath.Join(backupDir, exeBase)); err != nil {
		os.RemoveAll(stageDir)
		return fmt.Errorf("备份二进制失败: %w", err)
	}
	if err = copyDir(uiDir, filepath.Join(backupDir, "ui")); err != nil {
		os.RemoveAll(stageDir)
		return fmt.Errorf("备份 ui 失败: %w", err)
	}

	setProgress(func(p *Progress) { p.State, p.Stage = "swapping", "替换程序文件" })
	// 二进制：rename 覆盖正在运行的文件在 Linux/macOS 上是原子的，运行中的进程不受影响。
	if err = os.Rename(filepath.Join(stageDir, exeBase), exePath); err != nil {
		os.RemoveAll(stageDir)
		return fmt.Errorf("替换二进制失败: %w", err)
	}
	// ui：旧目录整体挪走再顶上新的，两个 rename 都在同一目录内完成。
	oldUI := filepath.Join(exeDir, ".ui-old")
	os.RemoveAll(oldUI)
	if _, err = os.Stat(uiDir); err == nil {
		if err = os.Rename(uiDir, oldUI); err != nil {
			rollback(record.FromVersion, dataDir)
			os.RemoveAll(stageDir)
			return fmt.Errorf("替换 ui 失败，已回滚: %w", err)
		}
	}
	if err = os.Rename(filepath.Join(stageDir, "ui"), uiDir); err != nil {
		// ui 换失败不影响二进制（新二进制通常兼容旧 ui），但为一致起见回滚。
		rollback(record.FromVersion, dataDir)
		os.RemoveAll(stageDir)
		return fmt.Errorf("替换 ui 失败，已回滚: %w", err)
	}
	os.RemoveAll(oldUI)
	os.RemoveAll(stageDir)
	os.Remove(fpkPath)
	return nil
}

func rollback(fromVersion, dataDir string) {
	backupDir := filepath.Join(dataDir, "updates", "backup")
	exePath, err := os.Executable()
	if err != nil {
		logger.Error("回滚失败，无法定位运行程序: %v", err)
		return
	}
	exeBase := filepath.Base(exePath)
	if err = copyFile(filepath.Join(backupDir, exeBase), exePath); err != nil {
		logger.Error("回滚二进制失败: %v", err)
		return
	}
	uiDir := filepath.Join(filepath.Dir(exePath), "ui")
	oldUI := filepath.Join(filepath.Dir(exePath), ".ui-old")
	if _, err = os.Stat(oldUI); err == nil {
		os.RemoveAll(uiDir)
		if err = os.Rename(oldUI, uiDir); err != nil {
			logger.Error("回滚 ui 失败: %v", err)
		}
	} else if berr := copyDir(filepath.Join(backupDir, "ui"), uiDir); berr != nil {
		logger.Error("回滚 ui 失败: %v", berr)
	}
	logger.Info("已回滚到 %s（备份在 %s）", fromVersion, backupDir)
}

func writePending(dataDir string, pending *pendingUpdate) error {
	dir := filepath.Join(dataDir, "updates")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(pending)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "pending.json"), data, 0o644)
}

// restartFn / exePathFn 供测试替换的重启入口与运行程序定位；生产实现见 restart_unix.go。
var (
	restartFn = restartProcess
	exePathFn = defaultExePath
)

func defaultExePath() (string, error) {
	exePath, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(exePath)
}

func failProgress(err error) error {
	setProgress(func(p *Progress) { p.State, p.Error, p.Stage = "failed", err.Error(), "" })
	return err
}

func failWithRecord(record *UpdateRecord, err error) error {
	record.Status, record.Error = "failed", err.Error()
	now := time.Now()
	record.CompletedAt = &now
	if dbRef != nil {
		dbRef.Model(&UpdateRecord{}).Where("id = ?", record.ID).
			Updates(map[string]any{"status": "failed", "error": err.Error(), "completed_at": &now})
	}
	logger.Error("自更新失败：%v", err)
	return failProgress(err)
}

// GetProgress 返回当前更新任务的实时状态。
func GetProgress() Progress { return getProgress() }

// ---- 自动更新定时任务 ----

// autoUpdateEnabled 读系统配置 auto_update_enabled，缺省开启。
func autoUpdateEnabled(db *gorm.DB) bool {
	v, err := sysconfig.GetConfig(db, "auto_update_enabled", 0)
	if err != nil {
		return true
	}
	return v != "false"
}

func runAutoUpdateCheck() {
	db, dataDir := dbRef, dataRef
	if db == nil || dataDir == "" {
		return
	}
	// 自动应用只在飞牛形态开启：docker 有镜像更新机制，开发机不擅自升级。
	if config.C.DeviceType != "fnos" {
		return
	}
	if !autoUpdateEnabled(db) {
		return
	}
	rel, err := FetchLatest(configLookup(db), false)
	if err != nil {
		logger.Info("自动检查更新失败（不影响使用）：%v", err)
		return
	}
	if !IsNewer(rel.Tag, version.Version) {
		return
	}
	logger.Info("自动检查发现新版本 %s（当前 %s），开始自动更新", rel.Tag, version.Version)
	if err = Apply(configLookup(db), dataDir, true); err != nil && !errors.Is(err, ErrInProgress) {
		logger.Error("自动更新失败：%v", err)
	}
}

func init() {
	database.RegisterModels(&UpdateRecord{})
	scheduler.Register(scheduler.Job{
		Name:       "update_check",
		Interval:   6 * time.Hour,
		RunAtStart: true,
		Run:        runAutoUpdateCheck,
	})
	sysconfig.RegisterConfig(sysconfig.ConfigDef{
		Key:         "auto_update_enabled",
		Scope:       sysconfig.ScopeSystem,
		Type:        sysconfig.TypeBool,
		Default:     "true",
		Group:       "update",
		Label:       "自动更新",
		Description: "定时检查 GitHub / Gitee 发行版，发现新版本后自动下载安装包并原地升级（仅飞牛安装形态生效；docker 部署请用镜像更新）",
	})
	sysconfig.RegisterConfig(sysconfig.ConfigDef{
		Key:         "update_channel",
		Scope:       sysconfig.ScopeSystem,
		Type:        sysconfig.TypeString,
		Default:     "auto",
		Group:       "update",
		Label:       "更新渠道",
		Description: "auto=先 Gitee 后 GitHub；也可填 gitee 或 github 固定单一渠道",
	})
}
