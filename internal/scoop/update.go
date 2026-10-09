package scoop

import (
	"fmt"
	"io"
	"strings"
)

// ---------------------------------------------------------------------------
// 错误分类
// ---------------------------------------------------------------------------

// updateErrorMarkers scoop update 输出中判定更新失败的错误标记（大小写不敏感子串）。
// 取自真实的 scoop / git 报错输出，集中定义便于按实际输出增补
var updateErrorMarkers = []string{
	"unable to access",              // git：无法访问远端
	"could not resolve host",        // git：域名解析失败
	"failed to connect",             // 连接失败
	"download failed",               // scoop：下载失败
	"timed out",                     // 连接/下载超时
	"would be overwritten by merge", // git：本地有未提交改动，与远端冲突
	"your local changes",            // git：本地文件已改动
	"not a git repository",          // git：目录不是 git 仓库（桶损坏）
	"detected dubious ownership",    // git：目录属主可疑
}

// classifyUpdateLine 判断一行（已去 ANSI 的）scoop 输出是否命中错误标记。
// 命中即判定该 app 更新失败
func classifyUpdateLine(line string) bool {
	line = strings.ToLower(line)
	for _, marker := range updateErrorMarkers {
		if strings.Contains(line, marker) {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// 准备与分组
// ---------------------------------------------------------------------------

// UpdatePlan --update 的准备结果：遗留清理 + status 分组。
// Skipped / NotGithub / Proxied 是 All 中元素的分组视图（同一份记录的指针）
type UpdatePlan struct {
	Leftovers []OutdatedApp  // 开工前自动还原的遗留备份（Status 为 Success / Failed）。这是因为再次执行 --update 时，gh_proxy 可能不同
	All       []OutdatedApp  // scoop status -l 的全部行（按原顺序），Status 为分组结果
	Skipped   []*OutdatedApp // 不处理：Skipped、Manifest not found、Manifest error 等（保留待用）
	NotGithub []*OutdatedApp // 无待修改 URL，可直接更新（保留待用，当前不更新）
	Proxied   []*OutdatedApp // Is github / Proxy set，本次更新的处理组
}

// PrepareUpdate --update 的准备阶段：
//  1. 清理上次运行遗留的备份 manifest 与 proxy，保证从干净状态开始
//     （清理改写的 proxy 已由 restoreScoopProxy 同步到内存 cfg，无需重新获取配置）
//  2. 执行 scoop status -l，逐个分析并把 outdated apps 分为三组
func PrepareUpdate(cfg *ScoopConfig) (UpdatePlan, error) {
	plan := UpdatePlan{}

	leftovers, err := cleanupLeftovers(cfg)
	if err != nil {
		return plan, err
	}
	plan.Leftovers = leftovers

	apps, err := getOutdatedApps()
	if err != nil {
		return plan, fmt.Errorf("执行 scoop status -l 失败: %v", err)
	}

	plan.All = apps
	for i := range apps {
		app := &apps[i]
		prepareAppManifest(cfg, app) // 分析结果记录在 app.Status

		switch app.Status {
		case IsGitHub, ProxySet:
			plan.Proxied = append(plan.Proxied, app)
		case NotGitHub:
			plan.NotGithub = append(plan.NotGithub, app)
		default:
			plan.Skipped = append(plan.Skipped, app)
		}
	}

	return plan, nil
}

// cleanupLeftovers 还原上次运行遗留的备份 manifest 与被备份清空的 proxy。
// 单个 app 还原失败只记录在其 Status 中，由输出层展示
func cleanupLeftovers(cfg *ScoopConfig) ([]OutdatedApp, error) {
	apps, err := findProxiedManifests(cfg.RootPath)
	if err != nil {
		return apps, fmt.Errorf("扫描 buckets 目录失败: %v", err)
	}

	for i := range apps {
		_ = restoreProxiedManifest(&apps[i]) // 失败已记录在 apps[i].Status
	}

	if err := RestoreScoopProxy(cfg); err != nil {
		return apps, fmt.Errorf("恢复 scoop config proxy 失败: %v", err)
	}

	return apps, nil
}

// ---------------------------------------------------------------------------
// 逐个更新
// ---------------------------------------------------------------------------

// UpdatePlainApp 更新 Not github 组的 app：直接执行 scoop update <app_name>。
// 结果记录在 app.Status（Updated / Update failed）。
// 保留待用：当前 --update 只更新 Proxied 组，本函数暂无调用方
func UpdatePlainApp(app *OutdatedApp, w io.Writer) {
	if app.Status != NotGitHub {
		return
	}
	errLines, timedOut, runErr := runScoopStream(w, appUpdateTimeout, "update", app.Name)
	app.Status = updateVerdict(errLines, timedOut, runErr)
}

// UpdateProxiedApp 更新 proxy 保护罩内的 app：
//   - Is github：先把 gh_proxy 前缀写入 manifest，更新后无论成败都还原 manifest
//   - Proxy set：URL 已带前缀、无备份，直接更新并保持 manifest 原状（由输出层警告）
//
// 失败只记录在 app.Status，不中断整体流程
func UpdateProxiedApp(app *OutdatedApp, w io.Writer) {
	if app.Status == IsGitHub {
		if err := setProxiedManifest(app); err != nil {
			// 备份完成但写入失败时 manifest 缺失，必须把备份还原回去
			_ = restoreProxiedManifest(app)
			app.Status = ManifestError
			return
		}
		if app.Status != IsGitHub { // BackupExists 等状态：无法安全更新，跳过
			return
		}
	}

	errLines, timedOut, runErr := runScoopStream(w, appUpdateTimeout, "update", app.Name)
	verdict := updateVerdict(errLines, timedOut, runErr)

	if app.Status == IsGitHub { // 无论更新成败都还原 manifest
		_ = restoreProxiedManifest(app) // Status 被覆盖为 RestoreSuccess / RestoreFailed
		if app.Status != RestoreSuccess {
			verdict = RestoreFailed // 还原失败意味着 manifest 仍处于已修改状态，优先展示
		}
	}

	app.Status = verdict
}

// updateVerdict 根据扫描到的错误行、超时与进程退出情况判定更新成败
func updateVerdict(errLines []string, timedOut bool, runErr error) OutdatedAppStatus {
	if timedOut || len(errLines) > 0 || runErr != nil {
		return UpdateFailed
	}
	return Updated
}
