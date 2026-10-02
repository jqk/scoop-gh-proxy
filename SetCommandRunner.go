package main

import (
	"fmt"
	"os/exec"
)

func RunSetCommand(cfg *ScoopConfig, dryRun bool) ([]SetCommandItem, error) {
	rows, err := parseScoopStatus(cfg.RootPath)
	if err != nil {
		return rows, fmt.Errorf("执行 scoop status -l 失败: %v", err)
	}

	count := 0

	for i := range rows {
		r := &rows[i]

		if r.Status != Unknown {
			continue // 只处理刚刚初始化，没有被处理过的
		}
		if !fillBucketManifest(cfg, r) {
			continue // 找不到对应的 manifest 的文件信息，无法继续处理，就结束
		}
		if matched, err := locateManifest(r, cfg.GhProxy); err != nil || !matched {
			continue // 操作 manifest 文件失败，或者没有待修改的 url，结束处理
		}

		count++ // 找到需要修改 GitHub 代理的数量

		if !dryRun {
			if err := backupManifest(r); err != nil {
				r.Status = BackupFailed
			}
		}
	}

	err = nil
	if !dryRun && count > 0 {
		err = backupAndSetProxy(cfg)
	}

	return rows, err
}

func fillBucketManifest(cfg *ScoopConfig, r *SetCommandItem) bool {
	if r.Bucket = findBucket(cfg.RootPath, r.Name); r.Bucket == "" {
		r.Status = NoManifest
		return false
	}
	if r.Manifest = findManifest(cfg.RootPath, r.Name, r.Bucket); r.Manifest == "" {
		r.Status = NoManifest
		return false
	}

	r.ManifestBackup = createManifestBackupName(r.Manifest) // 有 manifest 才确定备份文件名
	return true
}

func backupAndSetProxy(cfg *ScoopConfig) error {
	if cfg.Proxy != "" {
		if cfg.Proxy != cfg.GhScoopProxyBackup { // 保存一下，因为后面会清空此值
			if err := exec.Command("scoop", "config", "gh_scoop_proxy_backup", cfg.Proxy).Run(); err != nil {
				return err
			}
		}
		if err := exec.Command("scoop", "config", "rm", "proxy").Run(); err != nil { //清空
			return err
		}
	}

	return nil
}
