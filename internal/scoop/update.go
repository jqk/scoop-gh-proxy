package scoop

import (
	"fmt"
	"io"
	"strings"
	"sync"
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

// scoopFailureMarkers scoop install/update 失败的输出标志（大小写不敏感子串），
// 取自 scoop 源码（lib\install.ps1 / lib\download.ps1）：失败时先输出 ERROR 行
// （abort 型失败随后以退出码 1 结束），另有静默失败（error 后直接 return，退出码 0）。
// 命中仅用于捕获失败原因行供输出层提示；成败判定仍以成功标志为最终定论
var scoopFailureMarkers = []string{
	"download failed",                      // 下载失败（aria2 与默认下载器）：Download failed! (Error N) ...
	"hash check failed",                    // 哈希校验失败：Hash check failed!
	"installation aborted",                 // 安装器/安装脚本执行失败：Installation aborted. ...
	"is not valid",                         // 下载 URL 不可用：ERROR URL <url> is not valid
	"couldn't find manifest",               // manifest 缺失或损坏（本工具已预检，兜底）
	"doesn't support current architecture", // 架构不支持（静默失败：error 后直接 return）
}

// matchScoopFailure 判断一行（已去 ANSI 的）scoop 输出是否命中失败标志
func matchScoopFailure(line string) bool {
	line = strings.ToLower(line)
	for _, marker := range scoopFailureMarkers {
		if strings.Contains(line, marker) {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// 准备与分组
// ---------------------------------------------------------------------------

// UpdatePlan update 的准备结果：遗留清理 + status 分组。
// Skipped / NotGithub / Proxied 是 All 中元素的分组视图（同一份记录的指针）
type UpdatePlan struct {
	Leftovers []OutdatedApp  // 开工前自动还原的遗留备份（Status 为 Success / Failed）。这是因为再次执行 update 时，gh_proxy 可能不同
	All       []OutdatedApp  // 更新目标在 scoop status -l 中的行（按原顺序），Status 为分组结果；全量更新为全部行，点名更新为匹配到的行
	Skipped   []*OutdatedApp // 不处理：Skipped、Manifest not found、Manifest error 等（保留待用）
	NotGithub []*OutdatedApp // 无待修改 URL，可直接更新（保留待用，当前不更新）
	Proxied   []*OutdatedApp // Is github / Proxy set，本次更新的处理组
	NoUpdate  []string       // 点名更新（update <app...>）时：点名但不在 outdated 列表中的 app（已最新或未安装），无可更新；全量更新时为空
}

// PrepareUpdate update 的准备阶段：
//  1. 清理上次运行遗留的备份 manifest 与 proxy，保证从干净状态开始
//     （直接复用 restore 主流程 RestoreOutdatedAppManifests；其恢复的 proxy
//     已同步到内存 cfg，无需重新获取配置）
//  2. 执行 scoop status -l，逐个分析并把更新目标分为三组。
//     requested 为点名的 app 列表（不区分大小写，静默去重）；空 = 全量更新
func PrepareUpdate(cfg *ScoopConfig, requested []string) (UpdatePlan, error) {
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

	// 点名更新：先挑出匹配的行（未点名的行不做 manifest 分析），再进入统一分组
	if len(requested) > 0 {
		apps, plan.NoUpdate = pickRequestedApps(apps, requested)
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

// pickRequestedApps 从 outdated 行中挑出点名的 app：
//   - 匹配不区分大小写（与 scoop / Windows 文件系统一致）
//   - 点名静默去重（不区分大小写，保留首次出现，不产生任何输出）：
//     如点名为 git GIT git，实际只更新一次；picked 顺序与去重后点名首次出现的顺序一致
//   - 同名 outdated 行只取第一行
//   - 不在 apps 中的点名归入 noUpdate（原样记录输入的大小写）
func pickRequestedApps(apps []OutdatedApp, requested []string) (picked []OutdatedApp, noUpdate []string) {
	seen := make(map[string]bool, len(requested)) // 已处理过的点名（小写），用于静默去重
	for _, name := range requested {
		key := strings.ToLower(name)
		if seen[key] {
			continue
		}
		seen[key] = true

		found := false
		for i := range apps {
			if strings.EqualFold(apps[i].Name, name) {
				picked = append(picked, apps[i])
				found = true
				break
			}
		}
		if !found {
			noUpdate = append(noUpdate, name)
		}
	}
	return picked, noUpdate
}

// ---------------------------------------------------------------------------
// 逐个更新
// ---------------------------------------------------------------------------

// runResult 一次 scoop update/install 流式执行的判定摘要
type runResult struct {
	successSeen bool   // 是否命中成功标志
	failureLine string // 最后命中的失败标志行（空 = 未命中），失败时供输出层提示原因
}

// runScoopForApp 流式执行 scoop <cmd> <spec>：启动下载进度采样、透传输出，
// 逐行扫描成功/失败标志，返回判定摘要。update 与 install 共用
func runScoopForApp(cfg *ScoopConfig, w io.Writer, onProgress func(Progress), cmd, spec string) runResult {
	dp := newDownloadProgressFor(cfg, onProgress)
	stop := dp.Start()
	defer stop()

	var mu sync.Mutex // 保护 failureLine：两个管道的读取协程并发回调
	failureLine := ""
	observe := func(line string) {
		dp.Feed(line)
		if matchScoopFailure(line) {
			mu.Lock()
			failureLine = line // 记录最后一次命中：最接近真实失败原因
			mu.Unlock()
		}
	}

	successSeen, _ := runScoopStream(w, observe, cmd, spec)
	return runResult{successSeen: successSeen, failureLine: failureLine}
}

// proxiedAppCmd 在 manifest 保护罩内执行 scoop <cmd> <spec>：
// 先把 gh_proxy 前缀写入 manifest，结束后无论成败都还原 manifest。
// verdict 决定成功标志命中时的结果状态（update 为 Updated，install 为 Installed）；
// 还原失败意味着 manifest 仍处于已修改状态，优先展示为 RestoreFailed
func proxiedAppCmd(cfg *ScoopConfig, app *OutdatedApp, w io.Writer, onProgress func(Progress), cmd, spec string, verdict func(bool) OutdatedAppStatus) {
	if err := setProxiedManifest(app); err != nil {
		// 备份完成但写入失败时 manifest 缺失，必须把备份还原回去
		_ = restoreProxiedManifest(app)
		app.Status = ManifestError
		return
	}
	if app.Status != IsGitHub { // BackupExists 等状态：无法安全执行，跳过
		return
	}

	r := runScoopForApp(cfg, w, onProgress, cmd, spec)
	app.Failure = r.failureLine // 失败原因行（未命中为空），install 失败时由输出层提示
	v := verdict(r.successSeen)

	_ = restoreProxiedManifest(app) // Status 被覆盖为 RestoreSuccess / RestoreFailed
	if app.Status != RestoreSuccess {
		v = RestoreFailed // 还原失败优先展示
	}
	app.Status = v
}

// UpdatePlainApp 更新 Not github 组的 app：直接执行 scoop update <app_name>。
// w 为 scoop 输出的透传流；onProgress 为结构化下载进度回调（nil = 不需要）。
// 结果记录在 app.Status（Updated / Update failed）。
// 保留待用：当前 update 只更新 Proxied 组，本函数暂无调用方
func UpdatePlainApp(cfg *ScoopConfig, app *OutdatedApp, w io.Writer, onProgress func(Progress)) {
	if app.Status != NotGitHub {
		return
	}
	r := runScoopForApp(cfg, w, onProgress, "update", app.Name)
	app.Failure = r.failureLine
	app.Status = updateVerdict(r.successSeen)
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
		proxiedAppCmd(cfg, app, w, onProgress, "update", app.Name, updateVerdict)
		return
	}
	// Proxy set：直接执行，manifest 不动
	r := runScoopForApp(cfg, w, onProgress, "update", app.Name)
	app.Failure = r.failureLine
	app.Status = updateVerdict(r.successSeen)
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
