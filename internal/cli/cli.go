package cli

import (
	"fmt"
	"os"
	"slices"

	"github.com/jqk/scoop-gh-proxy/internal/scoop"
)

// version 版本号，构建时由 build.bat / release.yml 通过 -ldflags -X 注入
// （规则：最近 tag 的 patch 号 + 其后提交次数）；直接 go build 时为 dev
var version = "dev"

// buildTime 构建时间（2006-01-02_15:04:05），构建时注入；为空表示非脚本构建
var buildTime = ""

// loadConfig 获取并校验 scoop 配置；失败时打印错误，code 非 0 为应退出的退出码
func loadConfig() (cfg scoop.ScoopConfig, code int) {
	cfg, err := scoop.GetScoopConfig()
	if err != nil {
		error_("%s", err)
		return cfg, 1
	}
	return cfg, 0
}

// ---------------------------------------------------------------------------
// set
// ---------------------------------------------------------------------------

// RunSet set：定位 + 备份 + 修改 [app].json。返回退出码：1 = 配置错误
func RunSet(dryRun bool) int {
	cfg, code := loadConfig()
	if code != 0 {
		return code
	}
	return runSet(&cfg, dryRun)
}

func runSet(cfg *scoop.ScoopConfig, dryRun bool) int {
	apps, err := scoop.SetProxyForOutdatedApps(cfg, dryRun)
	if err != nil {
		error_("%s", err)
	}
	printSetSummary(apps)
	return 0
}

// ---------------------------------------------------------------------------
// restore
// ---------------------------------------------------------------------------

// RunRestore restore：还原 backup。返回退出码：1 = 配置错误
func RunRestore(dryRun bool) int {
	cfg, code := loadConfig()
	if code != 0 {
		return code
	}
	return runRestore(&cfg, dryRun)
}

func runRestore(cfg *scoop.ScoopConfig, dryRun bool) int {
	items, err := scoop.RestoreOutdatedAppManifests(cfg, dryRun)
	if err != nil {
		error_("%s", err)
	}
	printRestoreSummary(items, !dryRun)
	return 0
}

// ---------------------------------------------------------------------------
// status（dry-run：先 restore 明细，后 set 明细，不修改任何文件）
// ---------------------------------------------------------------------------

// RunStatus status：依次以 dry-run 执行 restore 与 set（配置只取一次）。返回退出码：1 = 配置错误
func RunStatus() int {
	cfg, code := loadConfig()
	if code != 0 {
		return code
	}
	if code := runRestore(&cfg, true); code != 0 {
		return code
	}
	fmt.Println() // 分隔 restore 明细与 set 明细
	return runSet(&cfg, true)
}

// ---------------------------------------------------------------------------
// update（自动化更新：清理遗留 → 分组 → 保护罩内逐个 scoop update <app> → 恢复 proxy → 汇总）
// update [app...]：点名时只更新匹配到的 app；update * 与 update 等价
// ---------------------------------------------------------------------------

// requestedApps 整理 update 的参数：
//   - 任一参数为 * 时视为全量更新（requested 返回 nil）
//   - 混合形式（既有 * 又点名了 app，如 update app1 app2 * app3）：starMixed 返回 true，
//     由调用方提示后按全量更新处理；仅 * 与仅点名时均为 false
func requestedApps(args []string) (requested []string, starMixed bool) {
	if !slices.Contains(args, "*") {
		return args, false
	}
	return nil, slices.ContainsFunc(args, func(a string) bool { return a != "*" })
}

// RunUpdate update [app...]：无参数或含 * 时全量更新，否则只更新点名的 app。
// 返回退出码：1 = 配置错误、status -l 失败或 proxy 清空失败
func RunUpdate(args []string) int {
	cfg, code := loadConfig()
	if code != 0 {
		return code
	}

	requested, starMixed := requestedApps(args)
	if starMixed {
		warning("参数中有 * 号，忽略指定的 app 名称，按 update * 处理")
	}

	plan, err := scoop.PrepareUpdate(&cfg, requested)
	if err != nil {
		error_("%s", err)
		return 1
	}

	// 遗留还原警告
	if len(plan.Leftovers) > 0 {
		warning("发现上次运行遗留的备份，已自动还原: %d", len(plan.Leftovers))
		printRestoreTable(plan.Leftovers)
		fmt.Println()
	}

	// 点名更新时：不在 outdated 列表中的 app（已最新或未安装），无可更新。
	// 独立消息走 stderr，不与后续 scoop 流式输出交错；序号 %2d 右对齐两位（1-9 前补空格）
	if len(plan.NoUpdate) > 0 {
		warning("以下 app 不在 outdated 列表中，没有可用的更新: %d", len(plan.NoUpdate))
		for i, name := range plan.NoUpdate {
			warning("%2d. %s", i+1, name)
		}
	}

	// 分组明细：status -l 解析出的更新目标及其归类（点名更新时为匹配到的 app）
	info("Apps outdated: %d, to update: %d", len(plan.All), len(plan.Proxied))
	if len(plan.All) == 0 {
		return 0
	}
	printSetTable(plan.All)
	fmt.Println()

	total := len(plan.Proxied) // 表头分母与循环一致：只数 Proxied 组（Is github / Proxy set）
	done := 0

	// 只更新 Proxied 组（Is github / Proxy set）；Skipped / Not github 组保留待用。
	// 全部 scoop update 之前清空 proxy（备份到 gh_scoop_proxy_backup），组结束处恢复。
	// 所有提前 return 都发生在清空之前，无需 defer 兜底
	if len(plan.Proxied) > 0 {
		if err := scoop.ClearScoopProxy(&cfg); err != nil {
			error_("备份并清空 scoop config proxy 失败: %s", err)
			return 1
		}

		for _, app := range plan.Proxied {
			fmt.Println() // 每个 app 的输出块之间空一行分隔
			if app.Status == scoop.ProxySet {
				caution("%s: manifest 已带 gh_proxy 前缀且无备份，更新后保持原状", app.Name)
			}
			done++
			info("[%d/%d] %s", done, total, app.Name)
			scoop.UpdateProxiedApp(&cfg, app, os.Stdout, printDownloadProgress)
			printAppOutcome(app)
		}

		if err := scoop.RestoreScoopProxy(&cfg); err != nil {
			error_("恢复 scoop config proxy 失败: %s", err)
		}
	}

	printUpdateSummary(&plan)
	return 0
}

// ---------------------------------------------------------------------------
// help / version
// ---------------------------------------------------------------------------

// RunHelp help：输出用法
func RunHelp() int {
	printUsage()
	return 0
}

// RunVersion version：输出版本号（脚本构建时附带构建时间）
func RunVersion() int {
	if buildTime != "" {
		fmt.Printf("%s (built %s)\n", version, buildTime)
	} else {
		fmt.Println(version)
	}
	return 0
}
