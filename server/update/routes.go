package update

// 管理端 HTTP 接口：更新状态、手动检查、触发更新、环境自检与历史记录。
// 全部挂管理员组；原 /api/version/check（登录即可见，驱动升级提示条）保持不变。

import (
	"errors"
	"runtime"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"smallgo/server/audit"
	"smallgo/server/config"
	"smallgo/server/response"
	"smallgo/server/version"
)

// RegisterAdminRoutes 挂载管理员更新接口，并记住运行时依赖供自动更新任务使用。
func RegisterAdminRoutes(admin *gin.RouterGroup, db *gorm.DB, dataDir string) {
	dbRef, dataRef = db, dataDir
	lookup := configLookup(db)

	admin.GET("/update/status", func(c *gin.Context) {
		resp := gin.H{
			"current":       version.Version,
			"device_type":   config.C.DeviceType,
			"goos":          runtime.GOOS,
			"goarch":        runtime.GOARCH,
			"repo":          RepoName(),
			"auto_enabled":  autoUpdateEnabled(db),
			"progress":      GetProgress(),
			"release_ready": releaseReady(),
		}
		rel, err := FetchLatest(lookup, false)
		if err != nil {
			resp["latest_error"] = err.Error()
		} else {
			resp["latest"] = rel
			resp["has_update"] = IsNewer(rel.Tag, version.Version)
		}
		var records []UpdateRecord
		db.Order("id DESC").Limit(10).Find(&records)
		resp["records"] = records
		response.Success(c, resp)
	})

	admin.POST("/update/check", func(c *gin.Context) {
		rel, err := FetchLatest(lookup, true)
		if err != nil {
			response.ErrorInternal(c, err.Error())
			return
		}
		asset, assetErr := PickFPKAsset(rel.Assets)
		result := gin.H{"latest": rel, "has_update": IsNewer(rel.Tag, version.Version)}
		if assetErr != nil {
			result["asset_error"] = assetErr.Error()
		} else {
			result["asset"] = asset
		}
		response.Success(c, result)
	})

	admin.POST("/update/preflight", func(c *gin.Context) {
		response.Success(c, Preflight(dataDir))
	})

	admin.POST("/update/apply", func(c *gin.Context) {
		err := Apply(lookup, dataDir, false)
		switch {
		case err == nil:
			audit.Log(db, c, "update_apply", "system", 0, "触发应用自更新")
			response.Success(c, gin.H{"ok": true, "progress": GetProgress()})
		case errors.Is(err, ErrNoUpdate):
			response.ErrorBadRequest(c, err.Error())
		case errors.Is(err, ErrInProgress):
			response.ErrorBadRequest(c, err.Error())
		default:
			audit.Log(db, c, "update_apply_failed", "system", 0, err.Error())
			response.ErrorInternal(c, err.Error())
		}
	})

	admin.GET("/update/records", func(c *gin.Context) {
		var records []UpdateRecord
		db.Order("id DESC").Limit(50).Find(&records)
		response.Success(c, records)
	})
}

func releaseReady() bool {
	p := GetProgress()
	return p.State == "idle" || p.State == "done" || p.State == "failed"
}
