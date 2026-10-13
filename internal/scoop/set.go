package scoop

import (
	"fmt"
)

// SetProxyForOutdatedApps set 命令主流程：查找需要更新的软件，为匹配的软件添加代理
func SetProxyForOutdatedApps(cfg *ScoopConfig, dryRun bool) ([]ManagedApp, error) {
	apps, err := fetchOutdatedApps()
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

	if !dryRun && count > 0 {
		err = ClearScoopProxy(cfg)
	}

	return apps, err
}
