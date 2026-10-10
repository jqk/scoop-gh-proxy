package scoop

import (
	"fmt"
	"io"
	"strings"
)

// ---------------------------------------------------------------------------
// 错误分类
// ---------------------------------------------------------------------------

// updateErrorPrefixes 确定性失败的行首前缀（配合 classifyUpdateLine，已小写、
// 已剥 "Download: " 前缀）。来源为 scoop 源码（e:\scoop\apps\scoop）：
//   - "error "           scoop error 助手（core.ps1：Write-Host "ERROR <msg>"，含 app
//                        pre_uninstall 脚本的权限检查、hash 失败等）
//   - "error: "          git 的 error: 前缀
//   - "fatal: "          git 的 fatal: 前缀（无法访问远端、桶损坏等）
//   - "please try again" new_issue_msg（buckets.ps1，abort 红字无前缀，下载/hash 失败；
//                        API 限流、SourceForge 提示中的同片段也均在失败语境）
//   - "exception "       PowerShell 未捕获异常
var updateErrorPrefixes = []string{
	"error ",
	"error: ",
	"fatal: ",
	"please try again",
	"exception ",
}

// classifyUpdateLine 判断一行（已去 ANSI 的）scoop 输出是否为确定性失败。
// 只认行首前缀/整句形态，不做宽泛子串匹配：aria2 失败重试的瞬时噪声
// （errorCode=1、"(ERR):error occurred"、表格 ERR 行）与 WARN 提示
// （"Download failed!" 后 Fallback 重试）都不是终态——最终成败以成功标志
// （updateVerdict）为准，本函数的命中只用于失败原因的收集展示
func classifyUpdateLine(line string) bool {
	lower := strings.ToLower(strings.TrimSpace(line))
	lower = strings.TrimPrefix(lower, "download: ") // aria2 输出经 scoop 转发带的行前缀

	if strings.HasPrefix(lower, "warn") { // scoop warn 助手 / PowerShell WARNING：提示性输出，非终态
		return false
	}
	if lower == "running process detected, skip updating." { // scoop-update.ps1：普通输出，无前缀
		return true
	}
	for _, prefix := range updateErrorPrefixes {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}
	// 钩子脚本场景：scoop 以 -NoNewline 打印 "Running pre_uninstall script... "，
	// 脚本内 error 助手的 "ERROR <msg>" 拼接在同一行，只能句中匹配。
	// 瞬时噪声均为小写无空格形态（errorCode、):error），不会命中
	return strings.Contains(lower, " error ")
}

// updateSuccessMarkers scoop update <app> 成功的输出标志（大小写不敏感子串）。
// 两者都由 scoop 在流程末尾打印，是最终定论；其后可能还有 notes 等输出，不影响判定
var updateSuccessMarkers = []string{
	"was installed successfully",                 // install.ps1：安装/更新完成（其后 notes 不定长）
	"latest versions for all apps are installed", // scoop-update.ps1：已是最新版本，无需更新
}

// matchUpdateSuccess 判断一行（已去 ANSI 的）scoop 输出是否命中成功标志
func matchUpdateSuccess(line string) bool {
	return containsMarker(line, updateSuccessMarkers)
}

func containsMarker(line string, markers []string) bool {
	line = strings.ToLower(line)
	for _, marker := range markers {
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
	_, successSeen, _ := runScoopStream(w, "update", app.Name)
	app.Status = updateVerdict(successSeen)
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

	_, successSeen, _ := runScoopStream(w, "update", app.Name)
	verdict := updateVerdict(successSeen)

	if app.Status == IsGitHub { // 无论更新成败都还原 manifest
		_ = restoreProxiedManifest(app) // Status 被覆盖为 RestoreSuccess / RestoreFailed
		if app.Status != RestoreSuccess {
			verdict = RestoreFailed // 还原失败意味着 manifest 仍处于已修改状态，优先展示
		}
	}

	app.Status = verdict
}

// updateVerdict 判定更新成败：以成功标志为准。
// 成功标志（"was installed successfully" / "Latest versions ..."）由 scoop 在流程末尾
// 打印，是最终定论——即使之前有错误标记命中（可能是瞬时重试或误报），只要最终装上即为成功；
// 反之未见成功标志一律判失败（scoop 出错时退出码常为 0，错误标记/超时/退出码只能旁证，不能翻案）
func updateVerdict(successSeen bool) OutdatedAppStatus {
	if successSeen {
		return Updated
	}
	return UpdateFailed
}
