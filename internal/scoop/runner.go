package scoop

import (
	"fmt"
)

// prepareAppManifest 定位 app 的 manifest 并分析其中的 GitHub URL。
// 返回 true 表示存在待修改的 GitHub URL（Status 为 IsGitHub），可继续执行代理设置；
// 其余情况返回 false，具体状态记录在 app.Status。set 与 --update 共用
func prepareAppManifest(cfg *ScoopConfig, app *OutdatedApp) bool {
	if app.Status != Unknown {
		return false // 只处理刚刚初始化，没有被处理过的
	}
	if !fillBucketManifest(cfg.RootPath, app) {
		return false // 找不到对应的 bucket 或 manifest 文件，无法继续处理
	}
	// 分析失败与“无待修改 url（含已设置代理）”都记录在 app.Status
	matched, _ := analyzeManifest(app, cfg.GhProxy)
	return matched
}

// SetProxyForOutdatedApps 查找需要更新的软件，为匹配的软件添加代理
func SetProxyForOutdatedApps(cfg *ScoopConfig, dryRun bool) ([]OutdatedApp, error) {
	apps, err := getOutdatedApps()
	if err != nil {
		return apps, fmt.Errorf("执行 scoop status -l 失败: %v", err)
	}

	count := 0

	for i := range apps {
		app := &apps[i]
		if !prepareAppManifest(cfg, app) {
			continue
		}

		count++ // 找到需要修改 GitHub 代理的数量

		if !dryRun {
			if err := setProxiedManifest(app); err != nil {
				app.Status = BackupFailed
			}
		}
	}

	err = nil
	if !dryRun && count > 0 {
		err = setScoopProxy(cfg)
	}

	return apps, err
}

// RestoreOutdatedAppManifests restore 命令主流程：还原所有已备份的 manifest，
// 并恢复 scoop config 的 proxy。单条还原失败只记录在 apps[i].Status；
// 返回的 err 表示扫描 buckets 失败或恢复 proxy 失败
func RestoreOutdatedAppManifests(cfg *ScoopConfig, dryRun bool) ([]OutdatedApp, error) {
	apps, err := findProxiedManifests(cfg.RootPath)
	if err != nil {
		return apps, fmt.Errorf("扫描 buckets 目录失败: %v", err)
	}

	if !dryRun {
		for i := range apps {
			_ = restoreProxiedManifest(&apps[i]) // 失败已记录在 apps[i].Status，由明细表展示
		}

		err = restoreScoopProxy(cfg)
	}

	return apps, err
}
