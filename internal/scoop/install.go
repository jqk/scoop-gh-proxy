package scoop

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// ---------------------------------------------------------------------------
// 定位与分析
// ---------------------------------------------------------------------------

// LocateInstallApp 为 install 定位并分析 bucket manifest。
//   - bucket 非空（显式指定）：先校验 buckets\<bucket> 目录存在，再在该桶内定位 manifest
//   - bucket 为空（省略桶名）：在 buckets 下逐个桶查找，取第一个命中的桶
//
// 定位成功后做已安装预检（scoop 对已安装的 app 执行 install 只警告不安装、
// 且不会打印成功标志，必须提前拦截），最后 analyzeManifest 只读分析——
// 分析错误按项目惯例记录在 Status（ManifestError），不作为 error 返回。
// error 仅对应：已安装、bucket 不存在、app 不存在、buckets 目录读取失败
func LocateInstallApp(cfg *ScoopConfig, bucket, app string) (ManagedApp, error) {
	rec := ManagedApp{Name: app}
	bucketsRoot := filepath.Join(cfg.RootPath, "buckets")

	if bucket != "" {
		if st, err := os.Stat(filepath.Join(bucketsRoot, bucket)); err != nil || !st.IsDir() {
			return rec, fmt.Errorf("bucket 不存在: %s", bucket)
		}
		rec.Bucket = bucket
		rec.Manifest = findManifest(cfg.RootPath, app, bucket)
		if rec.Manifest == "" {
			return rec, fmt.Errorf("app 不存在: %s/%s", bucket, app)
		}
	} else {
		entries, err := os.ReadDir(bucketsRoot)
		if err != nil {
			return rec, fmt.Errorf("读取 buckets 目录失败: %v", err)
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			if m := findManifest(cfg.RootPath, app, entry.Name()); m != "" {
				rec.Bucket = entry.Name()
				rec.Manifest = m
				break // 找到即止，取目录序第一个命中的桶
			}
		}
		if rec.Manifest == "" {
			return rec, fmt.Errorf("app 不存在: %s（所有 bucket 中均未找到）", app)
		}
	}

	// 已安装预检
	if installedBucket := findBucket(cfg.RootPath, app); installedBucket != "" {
		return rec, fmt.Errorf("app 已安装（bucket %s），如需更新请执行 update %s", installedBucket, app)
	}

	rec.ManifestBackup = createManifestBackupName(rec.Manifest)
	analyzeManifest(&rec, cfg.GhProxy) // Status：IsGitHub / NotGitHub / ProxySet / ManifestError
	return rec, nil
}

// ---------------------------------------------------------------------------
// 安装
// ---------------------------------------------------------------------------

// installSpec 构造 scoop install 的 app 参数：<bucket>/<app>。
// bucket/app 形式在 scoop 中是显式桶请求（Get-Manifest 直接读取指定桶的 manifest），
// 保证安装的正是本程序定位并修改过的那份 manifest
func installSpec(app *ManagedApp) string {
	return app.Bucket + "/" + app.Name
}

// InstallApp 在 manifest 保护罩内安装 Is github 的 app：
// 先把 gh_proxy 前缀写入 bucket manifest，执行 scoop install <bucket>/<app>，
// 结束后无论成败都还原 manifest。结果记录在 app.Status
// （Installed / Install failed / Manifest error / Manifest backup exists / Restore failed）。
// 仅处理 IsGitHub，其它状态由调用方门控；w 与 onProgress 的含义同 UpdateProxiedApp
func InstallApp(cfg *ScoopConfig, app *ManagedApp, w io.Writer, onProgress func(Progress)) {
	if app.Status != IsGitHub {
		return
	}
	proxiedAppCmd(cfg, app, w, onProgress, "install", installSpec(app), Installed, InstallFailed)
}
