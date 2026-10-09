package cli

import (
	"fmt"
	"os"

	"github.com/jqk/scoop-gh-proxy/internal/scoop"
)

const version = "1.0.0"

// ---------------------------------------------------------------------------
// set
// ---------------------------------------------------------------------------

// RunSet --set：定位 + 备份 + 修改 [app].json。返回退出码：1 = 配置错误
func RunSet(dryRun bool) int {
	cfg, err := scoop.GetScoopConfig()
	if err != nil {
		error_("%s", err)
		return 1
	}

	apps, err := scoop.SetProxyForOutdatedApps(&cfg, dryRun)
	if err != nil {
		error_("%s", err)
	}
	printSetSummary(apps)
	return 0
}

// ---------------------------------------------------------------------------
// restore
// ---------------------------------------------------------------------------

// RunRestore --restore：还原 backup。返回退出码：1 = 配置错误
func RunRestore(dryRun bool) int {
	cfg, err := scoop.GetScoopConfig()
	if err != nil {
		error_("%s", err)
		return 1
	}

	items, err := scoop.RestoreOutdatedAppManifests(&cfg, dryRun)
	if err != nil {
		error_("%s", err)
	}
	printRestoreSummary(items, !dryRun)
	return 0
}

// ---------------------------------------------------------------------------
// status（dry-run：先 restore 明细，后 set 明细，不修改任何文件）
// ---------------------------------------------------------------------------

// RunStatus --status：依次以 dry-run 执行 restore 与 set。返回退出码：1 = 配置错误
func RunStatus() int {
	if code := RunRestore(true); code != 0 {
		return code
	}
	fmt.Println() // 分隔 restore 明细与 set 明细
	return RunSet(true)
}

// ---------------------------------------------------------------------------
// update（自动化更新：清理遗留 → 分组 → 保护罩内逐个 scoop update <app> → 恢复 proxy → 汇总）
// ---------------------------------------------------------------------------

// RunUpdate --update。返回退出码：1 = 配置错误、status -l 失败或 proxy 清空失败
func RunUpdate() int {
	cfg, err := scoop.GetScoopConfig()
	if err != nil {
		error_("%s", err)
		return 1
	}

	plan, err := scoop.PrepareUpdate(&cfg)
	if err != nil {
		error_("%s", err)
		return 1
	}

	// 任何返回路径（含 panic）都在函数退出时恢复 proxy；
	// RestoreScoopProxy 幂等，无备份或已恢复时为 no-op
	defer func() {
		if err := scoop.RestoreScoopProxy(&cfg); err != nil {
			error_("恢复 scoop config proxy 失败: %s", err)
		}
	}()

	// 遗留还原警告
	if len(plan.Leftovers) > 0 {
		warning("发现上次运行遗留的备份，已自动还原: %d", len(plan.Leftovers))
		printRestoreTable(plan.Leftovers)
		fmt.Println()
	}

	// 分组明细：status -l 解析出的全部 app 及其归类
	info("Apps to Update: %d", len(plan.All))
	if len(plan.All) == 0 {
		return 0
	}
	printSetTable(plan.All)
	fmt.Println()

	total := len(plan.All)
	done := 0

	// 只更新 Proxied 组（Is github / Proxy set）；Skipped / Not github 组保留待用。
	// 全部 scoop update 之前清空 proxy（备份到 gh_scoop_proxy_backup），退出时由 defer 恢复
	if len(plan.Proxied) > 0 {
		if err := scoop.SetScoopProxy(&cfg); err != nil {
			error_("备份并清空 scoop config proxy 失败: %s", err)
			return 1
		}

		for _, app := range plan.Proxied {
			if app.Status == scoop.ProxySet {
				warning("%s: manifest 已带 gh_proxy 前缀且无备份，更新后保持原状", app.Name)
			}
			done++
			info("[%d/%d] %s", done, total, app.Name)
			scoop.UpdateProxiedApp(app, os.Stdout)
			printAppOutcome(app)
		}
	}

	printUpdateSummary(&plan)
	return 0
}

// ---------------------------------------------------------------------------
// help / version
// ---------------------------------------------------------------------------

// RunHelp --help / -h：输出用法
func RunHelp() int {
	printUsage()
	return 0
}

// RunVersion --version / -v：输出版本号
func RunVersion() int {
	fmt.Println(version)
	return 0
}
