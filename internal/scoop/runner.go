package scoop

import (
	"fmt"
)

func SetProxyForOutdatedApps(cfg *ScoopConfig, dryRun bool) ([]OutdatedApp, error) {
	apps, err := getOutdatedApps()
	if err != nil {
		return apps, fmt.Errorf("执行 scoop status -l 失败: %v", err)
	}

	count := 0

	for i := range apps {
		app := &apps[i]

		if app.Status != Unknown {
			continue // 只处理刚刚初始化，没有被处理过的
		}
		if !fillBucketManifest(cfg, app) {
			continue // 找不到对应的 manifest 的文件信息，无法继续处理，就结束
		}
		if matched, err := locateManifest(app, cfg.GhProxy); err != nil || !matched {
			continue // 操作 manifest 文件失败，或者没有待修改的 url（含已设置代理），结束处理
		}

		count++ // 找到需要修改 GitHub 代理的数量

		if !dryRun {
			if err := backupManifest(app); err != nil {
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


