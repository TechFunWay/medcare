package update

import (
	"archive/tar"
	"compress/gzip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// buildTestFPK 构造一个与飞牛 fpk 同构的测试包：外层 tar.gz（manifest + app.tgz），
// 内层 app.tgz 是二进制与 ui/ 静态资源。
func buildTestFPK(t *testing.T, path, appname, version, platform, exeBase string, exeContent string, uiFiles map[string]string) {
	t.Helper()

	appTGZ := filepath.Join(t.TempDir(), "app.tgz")
	f, err := os.Create(appTGZ)
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	addFile := func(name, content string, mode int64) {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: mode, Size: int64(len(content))}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	addFile(exeBase, exeContent, 0o755)
	for name, content := range uiFiles {
		addFile(name, content, 0o644)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	appData, err := os.ReadFile(appTGZ)
	if err != nil {
		t.Fatal(err)
	}

	out, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	ogz := gzip.NewWriter(out)
 otw := tar.NewWriter(ogz)
	manifest := "appname                    = " + appname + "\n" +
		"version                    = " + version + "\n" +
		"platform                   = " + platform + "\n"
	if err := otw.WriteHeader(&tar.Header{Name: "manifest", Mode: 0o644, Size: int64(len(manifest))}); err != nil {
		t.Fatal(err)
	}
	if _, err := otw.Write([]byte(manifest)); err != nil {
		t.Fatal(err)
	}
	if err := otw.WriteHeader(&tar.Header{Name: "app.tgz", Mode: 0o644, Size: int64(len(appData))}); err != nil {
		t.Fatal(err)
	}
	if _, err := otw.Write(appData); err != nil {
		t.Fatal(err)
	}
	if err := otw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := ogz.Close(); err != nil {
		t.Fatal(err)
	}
	if err := out.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestParseManifest(t *testing.T) {
	m, err := parseManifest(strings.NewReader(
		"appname = techfunway-smallgo\nversion               = 0.3.1\nplatform                   = x86\nchangelog                  = \"\"\"\n多行内容\n\"\"\"\n"))
	if err != nil {
		t.Fatal(err)
	}
	if m.Appname != "techfunway-smallgo" || m.Version != "0.3.1" || m.Platform != "x86" {
		t.Fatalf("manifest = %+v", m)
	}
	if _, err := parseManifest(strings.NewReader("appname = x\n")); err == nil {
		t.Fatal("缺 version 应该报错")
	}
}

func TestExtractFPK(t *testing.T) {
	dir := t.TempDir()
	fpk := filepath.Join(dir, "test.fpk")
	stage := filepath.Join(dir, "stage")
	buildTestFPK(t, fpk, "techfunway-testapp", "9.9.9", "x86", "smallgo-test", "new-exe",
		map[string]string{"ui/index.html": "<html>new</html>", "ui/assets/a.js": "console.log(1)"})

	m, err := ExtractFPK(fpk, stage, "smallgo-test")
	if err != nil {
		t.Fatal(err)
	}
	if m.Appname != "techfunway-testapp" || m.Version != "9.9.9" || m.Platform != "x86" {
		t.Fatalf("manifest = %+v", m)
	}
	if got, err := os.ReadFile(filepath.Join(stage, "smallgo-test")); err != nil || string(got) != "new-exe" {
		t.Fatalf("二进制未展开: %v %q", err, got)
	}
	info, err := os.Stat(filepath.Join(stage, "smallgo-test"))
	if err != nil || info.Mode()&0o111 == 0 {
		t.Fatalf("二进制缺可执行位: %v %v", err, info.Mode())
	}
	if got, err := os.ReadFile(filepath.Join(stage, "ui", "index.html")); err != nil || string(got) != "<html>new</html>" {
		t.Fatalf("ui 未展开: %v %q", err, got)
	}
	if got, err := os.ReadFile(filepath.Join(stage, "ui", "assets", "a.js")); err != nil || string(got) != "console.log(1)" {
		t.Fatalf("ui 子目录未展开: %v %q", err, got)
	}
}

func TestExtractFPKTraversalRejected(t *testing.T) {
	dir := t.TempDir()
	// 构造带越界路径的包
	fpk := filepath.Join(dir, "evil.fpk")
	stage := filepath.Join(dir, "stage")
	if err := os.MkdirAll(stage, 0o755); err != nil {
		t.Fatal(err)
	}
	buildTestFPK(t, fpk, "techfunway-testapp", "1.0.0", "x86", "smallgo-test", "x",
		map[string]string{})
	// 直接改用底层函数注入越界条目
	payload := "evil"
	if err := func() error {
		tmp := filepath.Join(dir, "inner.tgz")
		f, _ := os.Create(tmp)
		gz := gzip.NewWriter(f)
		tw := tar.NewWriter(gz)
		_ = tw.WriteHeader(&tar.Header{Name: "../escaped.txt", Mode: 0o644, Size: int64(len(payload))})
		_, _ = tw.Write([]byte(payload))
		tw.Close()
		gz.Close()
		f.Close()
		return nil
	}(); err != nil {
		t.Fatal(err)
	}
	inner, _ := os.ReadFile(filepath.Join(dir, "inner.tgz"))
	// 重新打外层包，app.tgz 换成带越界路径的版本
	if err := rewriteAppTGZ(fpk, inner); err != nil {
		t.Fatal(err)
	}
	if _, err := ExtractFPK(fpk, stage, "smallgo-test"); err == nil {
		t.Fatal("越界路径应被拒绝")
	}
	if _, err := os.Stat(filepath.Join(dir, "escaped.txt")); err == nil {
		t.Fatal("越界文件不应被写出")
	}
}

// rewriteAppTGZ 把 fpk 里的 app.tgz 条目替换为指定内容（测试辅助）。
func rewriteAppTGZ(fpkPath string, innerTGZ []byte) error {
	tmp := fpkPath + ".new"
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	ogz := gzip.NewWriter(out)
	otw := tar.NewWriter(ogz)
	if err := otw.WriteHeader(&tar.Header{Name: "app.tgz", Mode: 0o644, Size: int64(len(innerTGZ))}); err != nil {
		return err
	}
	if _, err := otw.Write(innerTGZ); err != nil {
		return err
	}
	otw.Close()
	ogz.Close()
	out.Close()
	return os.Rename(tmp, fpkPath)
}

func TestExtractFPKMissingAppTGZ(t *testing.T) {
	dir := t.TempDir()
	fpk := filepath.Join(dir, "bare.fpk")
	out, _ := os.Create(fpk)
	ogz := gzip.NewWriter(out)
	otw := tar.NewWriter(ogz)
	manifest := "version = 1.0.0\n"
	_ = otw.WriteHeader(&tar.Header{Name: "manifest", Mode: 0o644, Size: int64(len(manifest))})
	_, _ = otw.Write([]byte(manifest))
	otw.Close()
	ogz.Close()
	out.Close()

	if _, err := ExtractFPK(fpk, filepath.Join(dir, "stage"), "exe"); err == nil {
		t.Fatal("缺 app.tgz 应该报错")
	}
}
