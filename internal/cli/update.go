package cli

import (
	"fmt"
	"os"

	"github.com/jqk/scoop-gh-proxy/internal/scoop"
)

// ---------------------------------------------------------------------------
// update（自动化更新：清理遗留 → 分组 → 逐个 scoop update <app> → 汇总）
// ---------------------------------------------------------------------------

func RunUpdate() {
	cfg, err := scoop.GetScoopConfig()
	if err != nil {
		error_("%s", err)
		os.Exit(1)
	}

	newCfg, plan, err := scoop.PrepareUpdate(&cfg)
	if err != nil {
		error_("%s", err)
		os.Exit(1)
	}
	cfg = newCfg

	// 遗留还原警告
	if len(plan.Leftovers) > 0 {
		warning("发现上次运行遗留的备份，已自动还原: %d", len(plan.Leftovers))
		printRestoreTable(plan.Leftovers)
		fmt.Println()
	}

	// 分组明细：status -l 解析出的全部 app 及其归类
	info("Apps to Update: %d", len(plan.All))
	if len(plan.All) == 0 {
		return
	}
	printSetTable(plan.All)
	fmt.Println()

	total := len(plan.All)
	done := 0

	// Not github 组：直接逐个更新
	for _, app := range plan.NotGithub {
		done++
		info("[%d/%d] %s", done, total, app.Name)
		scoop.UpdatePlainApp(app, os.Stdout)
		printAppOutcome(app)
	}

	// Is github / Proxy set 组：proxy 保护罩内逐个更新
	if len(plan.Proxied) > 0 {
		if err := scoop.BeginProxyPhase(&cfg); err != nil {
			error_("备份并清空 scoop config proxy 失败: %s", err)
			os.Exit(1)
		}
		proxyOn := true
		// 任何返回路径都保证恢复 proxy；正常路径在组结束处手动恢复，defer 仅兜底
		defer func() {
			if proxyOn {
				proxyOn = false
				if err := scoop.EndProxyPhase(&cfg); err != nil {
					error_("恢复 scoop config proxy 失败: %s", err)
				}
			}
		}()

		for _, app := range plan.Proxied {
			if app.Status == scoop.ProxySet {
				warning("%s: manifest 已带 gh_proxy 前缀且无备份，更新后保持原状", app.Name)
			}
			done++
			info("[%d/%d] %s", done, total, app.Name)
			scoop.UpdateProxiedApp(app, os.Stdout)
			printAppOutcome(app)
		}

		proxyOn = false
		if err := scoop.EndProxyPhase(&cfg); err != nil {
			error_("恢复 scoop config proxy 失败: %s", err)
		}
	}

	printUpdateSummary(&plan)
}

// printAppOutcome 输出单个 app 的更新结果行
func printAppOutcome(app *scoop.OutdatedApp) {
	switch app.Status {
	case scoop.Updated:
		success("%s: %s", app.Name, app.Status)
	case scoop.UpdateFailed, scoop.RestoreFailed:
		error_("%s: %s", app.Name, app.Status)
	default:
		warning("%s: %s", app.Name, app.Status) // BackupExists 等未参与更新的状态
	}
}

// printUpdateSummary 输出更新汇总：计数 + 更新结果明细表
func printUpdateSummary(plan *scoop.UpdatePlan) {
	updated, failed := 0, 0
	results := make([]scoop.OutdatedApp, 0, len(plan.NotGithub)+len(plan.Proxied))
	for _, group := range [][]*scoop.OutdatedApp{plan.NotGithub, plan.Proxied} {
		for _, app := range group {
			switch app.Status {
			case scoop.Updated:
				updated++
			case scoop.UpdateFailed, scoop.RestoreFailed:
				failed++
			}
			results = append(results, *app)
		}
	}
	skipped := len(plan.Skipped)

	fmt.Println()
	info("Update Summary: %d updated, %d failed, %d skipped", updated, failed, skipped)
	if len(results) > 0 {
		printSetTable(results)
	}
}
