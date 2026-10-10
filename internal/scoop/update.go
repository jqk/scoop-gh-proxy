package scoop

import (
	"fmt"
	"io"
	"strings"
)

// ---------------------------------------------------------------------------
// 成败判定
// ---------------------------------------------------------------------------

// updateSuccessMarkers scoop update <app> 成功的输出标志（大小写不敏感子串）。
// 两者都由 scoop 在流程末尾打印，是最终定论；其后可能还有 notes 等输出，不影响判定
var updateSuccessMarkers = []string{
	"was installed successfully",                 // install.ps1：安装/更新完成（其后 notes 不定长）
	"latest versions for all apps are installed", // scoop-update.ps1：已是最新版本，无需更新
}

// matchUpdateSuccess 判断一行（已去 ANSI 的）scoop 输出是否命中成功标志
func matchUpdateSuccess(line string) bool {
	line = strings.ToLower(line)
	for _, marker := range updateSuccessMarkers {
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
//     （直接复用 restore 主流程 RestoreOutdatedAppManifests；其恢复的 proxy
//     已同步到内存 cfg，无需重新获取配置）
//  2. 执行 scoop status -l，逐个分析并把 outdated apps 分为三组
func PrepareUpdate(cfg *ScoopConfig) (UpdatePlan, error) {
	plan := UpdatePlan{}

	leftovers, err := RestoreOutdatedAppManifests(cfg, false)
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

// ---------------------------------------------------------------------------
// 逐个更新
// ---------------------------------------------------------------------------

// UpdatePlainApp 更新 Not github 组的 app：直接执行 scoop update <app_name>。
// w 为 scoop 输出的透传流；onProgress 为结构化下载进度回调（nil = 不需要）。
// 结果记录在 app.Status（Updated / Update failed）。
// 保留待用：当前 --update 只更新 Proxied 组，本函数暂无调用方
func UpdatePlainApp(cfg *ScoopConfig, app *OutdatedApp, w io.Writer, onProgress func(Progress)) {
	if app.Status != NotGitHub {
		return
	}
	dp := newDownloadProgressFor(cfg, onProgress)
	stop := dp.Start()
	defer stop()

	successSeen, _ := runScoopStream(w, dp.Feed, "update", app.Name)
	app.Status = updateVerdict(successSeen)
}

// UpdateProxiedApp 更新 proxy 保护罩内的 app：
//   - Is github：先把 gh_proxy 前缀写入 manifest，更新后无论成败都还原 manifest
//   - Proxy set：URL 已带前缀、无备份，直接更新并保持 manifest 原状（由输出层警告）
//
// w 为 scoop 输出的透传流（原始字节，展示策略由调用方决定）；
// onProgress 为结构化下载进度回调（nil = 不需要），核心层不生成任何展示文本。
// 失败只记录在 app.Status，不中断整体流程
func UpdateProxiedApp(cfg *ScoopConfig, app *OutdatedApp, w io.Writer, onProgress func(Progress)) {
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

	dp := newDownloadProgressFor(cfg, onProgress)
	stop := dp.Start()
	defer stop()

	successSeen, _ := runScoopStream(w, dp.Feed, "update", app.Name)
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
