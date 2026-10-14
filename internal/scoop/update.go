package scoop

import (
	"fmt"
	"io"
	"strings"
)

// ---------------------------------------------------------------------------
// 准备与分组
// ---------------------------------------------------------------------------

// UpdatePlan update 的准备结果：遗留清理 + status 分组。
// Skipped / NotGithub / Proxied 是 All 中元素的分组视图（同一份记录的指针）
type UpdatePlan struct {
	Leftovers []ManagedApp  // 开工前自动还原的遗留备份（Status 为 Success / Failed）。这是因为再次执行 update 时，gh_proxy 可能不同
	All       []ManagedApp  // 更新目标在 scoop status -l 中的行（按原顺序），Status 为分组结果；全量更新为全部行，点名更新为匹配到的行
	Skipped   []*ManagedApp // 不处理：Skipped、Manifest not found、Manifest error、Proxy set 等（保留待用）。Proxy set：已带前缀且无备份，无法安全还原，本次跳过
	NotGithub []*ManagedApp // 无待修改 URL，第二更新组：不处理 manifest 与 proxy，直接更新
	Proxied   []*ManagedApp // Is github，第一更新组：proxy 保护罩内更新
	NoUpdate  []string      // 点名更新（update <app...>）时：点名但不在 outdated 列表中的 app（已最新或未安装），无可更新；全量更新时为空
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

	apps, err := fetchOutdatedApps()
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
		case IsGitHub:
			plan.Proxied = append(plan.Proxied, app)
		case NotGitHub:
			plan.NotGithub = append(plan.NotGithub, app)
		default: // Skipped、Manifest not found / error、Proxy set 等：本次不处理
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
func pickRequestedApps(apps []ManagedApp, requested []string) (picked []ManagedApp, noUpdate []string) {
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

// UpdateProxiedApp 更新 proxy 保护罩内的 app（Is github）：
// 先把 gh_proxy 前缀写入 manifest，更新后无论成败都还原 manifest。
// 分组已保证 Proxied 组只含 IsGitHub，其它状态由 proxiedAppCmd 内部防御性跳过。
//
// w 为 scoop 输出的透传流（原始字节，展示策略由调用方决定）；
// onProgress 为结构化下载进度回调（nil = 不需要），核心层不生成任何展示文本。
// 失败只记录在 app.Status，不中断整体流程
func UpdateProxiedApp(cfg *ScoopConfig, app *ManagedApp, w io.Writer, onProgress func(Progress)) {
	proxiedAppCmd(cfg, app, w, onProgress, "update", app.Name, Updated, UpdateFailed)
}

// UpdatePlainApp 更新无需 manifest/proxy 处理的 app（Not github 组）：
// 直接流式执行 scoop update <app>，成败以成功标志为准。
// w 与 onProgress 的含义同 UpdateProxiedApp，失败只记录在 app.Status，不中断整体流程
func UpdatePlainApp(cfg *ScoopConfig, app *ManagedApp, w io.Writer, onProgress func(Progress)) {
	runPlainApp(cfg, app, w, onProgress, "update", app.Name, Updated, UpdateFailed)
}
