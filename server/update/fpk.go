package update

// fpk 解析层：飞牛安装包本身就是更新包。fpk 是外层 tar.gz，内含
// app.tgz（应用的二进制与 ui 静态资源）、cmd/ 生命周期脚本、manifest
// 与安装向导文件。自更新只取运行所需的两样——二进制与 ui/——其余交给
// 飞牛应用中心在正式升包时管理。

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// FPKManifest 是 fpk 顶层 manifest 里自更新关心的字段。
type FPKManifest struct {
	Appname  string
	Version  string
	Platform string
}

// parseManifest 解析 manifest 的 key = value 行。
func parseManifest(r io.Reader) (FPKManifest, error) {
	var m FPKManifest
	data, err := io.ReadAll(io.LimitReader(r, 64<<10))
	if err != nil {
		return m, err
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		key, value, found := strings.Cut(line, "=")
		if !found {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		switch key {
		case "appname":
			m.Appname = value
		case "version":
			m.Version = value
		case "platform":
			m.Platform = value
		}
	}
	if m.Version == "" {
		return m, fmt.Errorf("manifest 缺少 version 字段")
	}
	return m, nil
}

// ExtractFPK 打开 fpk 文件，把内层 app.tgz 中运行所需的条目展开到
// stageDir：二进制（名字与 exeBase 相同，补可执行位）与 ui/ 全部文件。
// 返回解析出的 manifest 供调用方校验包名与版本。
func ExtractFPK(fpkPath, stageDir, exeBase string) (FPKManifest, error) {
	var manifest FPKManifest
	f, err := os.Open(fpkPath)
	if err != nil {
		return manifest, err
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return manifest, fmt.Errorf("fpk 不是有效的 gzip 包: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	var appTGZSeen bool
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return manifest, fmt.Errorf("fpk 外层包读取失败: %w", err)
		}
		switch path.Clean(hdr.Name) {
		case "manifest":
			if manifest, err = parseManifest(tr); err != nil {
				return manifest, fmt.Errorf("manifest 解析失败: %w", err)
			}
		case "app.tgz":
			appTGZSeen = true
			if err = extractAppTGZ(tr, stageDir, exeBase); err != nil {
				return manifest, err
			}
		}
	}
	if !appTGZSeen {
		return manifest, fmt.Errorf("fpk 里没有 app.tgz")
	}
	return manifest, nil
}

// extractAppTGZ 从 app.tgz 的 tar 流中取出二进制与 ui/，写入 stageDir。
// 只处理普通文件与目录；拒绝越界路径。
func extractAppTGZ(r io.Reader, stageDir, exeBase string) error {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return fmt.Errorf("app.tgz 不是有效的 gzip 包: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("app.tgz 读取失败: %w", err)
		}
		name := path.Clean(hdr.Name)
		if name == "." || name == ".." || strings.HasPrefix(name, "../") {
			return fmt.Errorf("app.tgz 出现越界路径 %q", hdr.Name)
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			if strings.HasPrefix(name, "ui/") {
				if err := os.MkdirAll(filepath.Join(stageDir, filepath.FromSlash(name)), 0o755); err != nil {
					return err
				}
			}
		case tar.TypeReg:
			switch {
			case name == exeBase:
				if err := os.MkdirAll(stageDir, 0o755); err != nil {
					return err
				}
				if err := writeFile(tr, filepath.Join(stageDir, exeBase), 0o755); err != nil {
					return err
				}
			case strings.HasPrefix(name, "ui/"):
				target := filepath.Join(stageDir, filepath.FromSlash(name))
				if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
					return err
				}
				if err := writeFile(tr, target, 0o644); err != nil {
					return err
				}
			}
		default:
			// 软链等其余类型不参与自更新，跳过。
		}
	}
}

func writeFile(r io.Reader, target string, mode os.FileMode) error {
	tmp := target + ".part"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	if _, err = io.Copy(out, r); err != nil {
		out.Close()
		os.Remove(tmp)
		return err
	}
	if err = out.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	// 先落 .part 再改名，避免中断留下半个文件冒充完整文件。
	return os.Rename(tmp, target)
}
