package scoop

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// buildInstallTestRoot 构造临时的 scoop 根目录：
//   - buckets\main\bucket\：git.json（github url）、curl.json（github url，未安装）、
//     wget.json（非 github url）、bad.json（坏 json）
//   - buckets\empty\：空桶目录
//   - apps\git\current\install.json：git 已安装（预检用）
func buildInstallTestRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	write := func(path, content string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	bucketDir := filepath.Join(root, "buckets", "main", "bucket")
	write(filepath.Join(bucketDir, "git.json"), `{"version":"1.0","url":"https://github.com/x/y.zip"}`)
	write(filepath.Join(bucketDir, "curl.json"), `{"version":"2.0","url":"https://github.com/a/b.zip"}`)
	write(filepath.Join(bucketDir, "wget.json"), `{"version":"1.2","url":"https://example.com/x.zip"}`)
	write(filepath.Join(bucketDir, "bad.json"), `{"version":`) // 坏 json
	write(filepath.Join(root, "buckets", "empty", ".keep"), "")
	write(filepath.Join(root, "apps", "git", "current", "install.json"), `{"bucket":"main","architecture":"64bit"}`)
	return root
}

func TestLocateInstallApp(t *testing.T) {
	cfg := &ScoopConfig{RootPath: buildInstallTestRoot(t), GhProxy: "https://mirror.example/"}

	// 错误类：bucket 不存在 / 指定桶找不到 app / 省略桶找不到 app / 已安装预检
	if _, err := LocateInstallApp(cfg, "ghost", "app"); err == nil || !strings.Contains(err.Error(), "bucket 不存在") {
		t.Errorf("bucket 不存在应报错, got %v", err)
	}
	if _, err := LocateInstallApp(cfg, "main", "nope"); err == nil || !strings.Contains(err.Error(), "app 不存在: main/nope") {
		t.Errorf("指定桶找不到 app 应报错, got %v", err)
	}
	if _, err := LocateInstallApp(cfg, "", "nope"); err == nil || !strings.Contains(err.Error(), "app 不存在: nope") {
		t.Errorf("省略桶找不到 app 应报错, got %v", err)
	}
	if _, err := LocateInstallApp(cfg, "main", "git"); err == nil || !strings.Contains(err.Error(), "已安装") {
		t.Errorf("已安装的 app 应被预检拦截, got %v", err)
	}

	// 省略桶：逐桶查找命中 main，github url → IsGitHub，记录编辑清单与原始内容
	rec, err := LocateInstallApp(cfg, "", "curl")
	if err != nil {
		t.Fatalf("LocateInstallApp(省略桶, curl) 意外出错: %v", err)
	}
	if rec.Bucket != "main" {
		t.Errorf("Bucket = %q, want main", rec.Bucket)
	}
	if rec.Status != IsGitHub {
		t.Errorf("Status = %v, want IsGitHub", rec.Status)
	}
	if len(rec.Edits) == 0 || len(rec.OriginalManifest) == 0 {
		t.Error("IsGitHub 应记录编辑清单与原始 manifest 内容")
	}
	if rec.ManifestBackup == "" || !strings.HasSuffix(rec.ManifestBackup, "curl"+backupSuffix) {
		t.Errorf("ManifestBackup = %q, 应以 curl-gh-backup.json 结尾", rec.ManifestBackup)
	}

	// 指定桶：非 github url → NotGitHub
	rec, err = LocateInstallApp(cfg, "main", "wget")
	if err != nil {
		t.Fatalf("LocateInstallApp(main, wget) 意外出错: %v", err)
	}
	if rec.Status != NotGitHub {
		t.Errorf("Status = %v, want NotGitHub", rec.Status)
	}

	// 坏 json：分析错误记录在 Status（ManifestError），不作为 error 返回
	rec, err = LocateInstallApp(cfg, "main", "bad")
	if err != nil {
		t.Fatalf("LocateInstallApp(main, bad) 意外出错: %v", err)
	}
	if rec.Status != ManifestError {
		t.Errorf("Status = %v, want ManifestError", rec.Status)
	}
}

func TestInstallSpec(t *testing.T) {
	app := OutdatedApp{Name: "curl", Bucket: "main"}
	if got := installSpec(&app); got != "main/curl" {
		t.Errorf("installSpec = %q, want main/curl", got)
	}
}

func TestInstallVerdict(t *testing.T) {
	if got := installVerdict(true); got != Installed {
		t.Errorf("installVerdict(true) = %v, want Installed", got)
	}
	if got := installVerdict(false); got != InstallFailed {
		t.Errorf("installVerdict(false) = %v, want InstallFailed", got)
	}
}
