package cli

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/jqk/scoop-gh-proxy/internal/scoop"
)

// loadConfig 获取并校验 scoop 配置；失败时打印错误，code 非 0 为应退出的退出码
func loadConfig() (cfg scoop.ScoopConfig, code int) {
	cfg, err := scoop.GetScoopConfig()
	if err != nil {
		errorf("%s", err)
		return cfg, 1
	}
	return cfg, 0
}

// ---------------------------------------------------------------------------
// set
// ---------------------------------------------------------------------------

// RunSet set：定位 + 备份 + 修改 [app].json。返回退出码：1 = 配置错误
func RunSet() int {
	defer fmt.Println() // 输出以空行结尾
	printBanner()

	cfg, code := loadConfig()
	if code != 0 {
		return code
	}
	return runSet(&cfg, false)
}

func runSet(cfg *scoop.ScoopConfig, dryRun bool) int {
	apps, err := scoop.SetProxyForOutdatedApps(cfg, dryRun)
	if err != nil {
		errorf("%s", err)
	}
	printSetSummary(apps)
	return 0
}

// ---------------------------------------------------------------------------
// restore
// ---------------------------------------------------------------------------

// RunRestore restore：还原 backup。返回退出码：1 = 配置错误
func RunRestore() int {
	defer fmt.Println() // 输出以空行结尾
	printBanner()

	cfg, code := loadConfig()
	if code != 0 {
		return code
	}
	return runRestore(&cfg, false)
}

func runRestore(cfg *scoop.ScoopConfig, dryRun bool) int {
	items, err := scoop.RestoreOutdatedAppManifests(cfg, dryRun)
	if err != nil {
		errorf("%s", err)
	}
	printRestoreSummary(items, !dryRun)
	return 0
}

// ---------------------------------------------------------------------------
// status（dry-run：先 restore 明细，后 set 明细，不修改任何文件）
// ---------------------------------------------------------------------------

// RunStatus status：依次以 dry-run 执行 restore 与 set（配置只取一次）。返回退出码：1 = 配置错误
func RunStatus() int {
	defer fmt.Println() // 输出以空行结尾
	printBanner()

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
	defer fmt.Println() // 输出以空行结尾
	printBanner()

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
		errorf("%s", err)
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
			errorf("备份并清空 scoop config proxy 失败: %s", err)
			return 1
		}

		for _, app := range plan.Proxied {
			fmt.Println() // 每个 app 的输出块之间空一行分隔
			if app.Status == scoop.ProxySet {
				caution("%s: manifest 已带 gh_proxy 前缀且无备份，更新后保持原状", app.Name)
			}
			done++
			info("[%d/%d] %s", done, total, app.Name)
			pp := newProgressPrinter(os.Stdout)
			scoop.UpdateProxiedApp(&cfg, app, pp, pp.printProgress)
			pp.finishLine()
			printAppOutcome(app)
		}

		if err := scoop.RestoreScoopProxy(&cfg); err != nil {
			errorf("恢复 scoop config proxy 失败: %s", err)
		}
	}

	printUpdateSummary(&plan)
	return 0
}

// ---------------------------------------------------------------------------
// install（单 app 安装：定位 bucket manifest → 状态门控 → Is github 保护罩内安装 → 恢复）
// ---------------------------------------------------------------------------

// parseInstallArg 解析 install 参数：<bucket/app>，bucket 可省略（省略时为 <app>）。
// app 不允许为空或包含 /
func parseInstallArg(arg string) (bucket, app string, err error) {
	b, a, ok := strings.Cut(arg, "/")
	if !ok { // 无 /：bucket 省略
		if arg == "" {
			return "", "", errors.New("缺少 app 名称")
		}
		return "", arg, nil
	}
	if b == "" || a == "" || strings.Contains(a, "/") {
		return "", "", fmt.Errorf("参数格式错误: %q（应为 <bucket/app>，bucket 可省略）", arg)
	}
	return b, a, nil
}

// RunInstall install <bucket/app>：定位 bucket manifest 并分析状态。
// Not github 提示直接用 scoop install；Is github 在保护罩内安装
// （gh_proxy 前缀 + 清空 proxy，结束后无论成败都恢复）；其它状态报错。
// 返回退出码：1 = 配置/参数错误、已安装、bucket 或 app 不存在、状态不允许、
// proxy 清空失败、安装未成功（未捕获成功标志或还原失败）
func RunInstall(args []string) int {
	defer fmt.Println() // 输出以空行结尾
	printBanner()

	if len(args) != 1 {
		errorf("用法: scoop-gh install <bucket/app>（bucket 可省略）")
		return 1
	}
	bucket, app, err := parseInstallArg(args[0])
	if err != nil {
		errorf("%s", err)
		return 1
	}

	cfg, code := loadConfig()
	if code != 0 {
		return code
	}

	rec, err := scoop.LocateInstallApp(&cfg, bucket, app)
	if err != nil {
		errorf("%s", err)
		return 1
	}
	spec := rec.Bucket + "/" + rec.Name
	if bucket == "" {
		info("已定位到 bucket: %s", rec.Bucket) // 省略桶名时输出实际命中的桶
	}

	switch rec.Status {
	case scoop.NotGitHub:
		// 无 github 下载链接，本工具无用武之地
		info("%s: Not github，无需本工具处理，直接执行: scoop install %s", spec, spec)
		return 0
	case scoop.IsGitHub:
		info("%s: Is github，安装期间临时应用 gh_proxy 前缀，结束后还原", spec)

		// 保护罩：安装前清空 proxy（备份到 gh_scoop_proxy_backup），结束后恢复。
		// 所有提前 return 都发生在清空之前，无需 defer 兜底
		if err := scoop.ClearScoopProxy(&cfg); err != nil {
			errorf("备份并清空 scoop config proxy 失败: %s", err)
			return 1
		}
		pp := newProgressPrinter(os.Stdout)
		scoop.InstallApp(&cfg, &rec, pp, pp.printProgress)
		pp.finishLine()
		if err := scoop.RestoreScoopProxy(&cfg); err != nil {
			errorf("恢复 scoop config proxy 失败: %s", err)
		}
	default:
		errorf("%s: %s，无法安装", spec, rec.Status)
		return 1
	}

	printAppOutcome(&rec)
	if rec.Status == scoop.InstallFailed && rec.Failure != "" {
		// 流式输出中最后命中的失败标志行（ERROR Download failed! / Hash check failed! 等），
		// 作为失败原因摘要跟在结果行后；走 stdout 与结果行同流保序
		fail("失败原因: %s", rec.Failure)
	}
	if rec.Status != scoop.Installed {
		return 1
	}
	return 0
}

// ---------------------------------------------------------------------------
// help / version
// ---------------------------------------------------------------------------

// RunHelp help：输出用法
func RunHelp() int {
	defer fmt.Println() // 输出以空行结尾
	printBanner()

	printUsage()
	return 0
}

// RunVersion version：版本信息在程序信息头第二行
func RunVersion() int {
	defer fmt.Println() // 输出以空行结尾
	printBanner()
	return 0
}
