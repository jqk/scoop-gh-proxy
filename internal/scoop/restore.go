package scoop

import (
	"fmt"
)

// RestoreOutdatedAppManifests restore 命令主流程：还原所有已备份的 manifest，
// 并恢复 scoop config 的 proxy。单条还原失败只记录在 apps[i].Status；
// 返回的 err 表示扫描 buckets 失败或恢复 proxy 失败
func RestoreOutdatedAppManifests(cfg *ScoopConfig, dryRun bool) ([]ManagedApp, error) {
	apps, err := findProxiedManifests(cfg.RootPath)
	if err != nil {
		return apps, fmt.Errorf("扫描 buckets 目录失败: %v", err)
	}

	if !dryRun {
		for i := range apps {
			_ = restoreProxiedManifest(&apps[i]) // 失败已记录在 apps[i].Status，由明细表展示
		}

		if err := RestoreScoopProxy(cfg); err != nil {
			return apps, fmt.Errorf("恢复 scoop config proxy 失败: %w", err)
		}
	}

	return apps, err
}
