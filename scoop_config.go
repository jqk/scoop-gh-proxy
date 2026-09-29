package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type scoopConfig struct {
	RootPath      string
	Proxy         string
	GoBackupProxy string
	GhProxy       string
}

// getScoopConfig 执行 scoop config 获取配置值
func getScoopConfig() (scoopConfig, error) {
	out, err := exec.Command("scoop", "config").Output()
	if err != nil {
		return scoopConfig{}, fmt.Errorf("执行 scoop config 失败: %w", err)
	}

	cfg := scoopConfig{}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		idx := strings.Index(line, ":")
		if idx < 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		val := strings.TrimSpace(line[idx+1:])
		switch key {
		case "root-path":
			cfg.RootPath = val
		case "proxy":
			cfg.Proxy = val
		case "go-backup-for-scoop-proxy":
			cfg.GoBackupProxy = val
		case "gh-proxy":
			cfg.GhProxy = val
		}
	}

	if err := validateConfig(&cfg); err != nil {
		return cfg, err
	}

	return cfg, nil
}

// validateConfig 校验 root_path / gh_proxy / proxy / go_backup_for_scoop_proxy
func validateConfig(cfg *scoopConfig) error {
	// 3.1 root_path
	if cfg.RootPath == "" {
		return fmt.Errorf("scoop config root-path 不存在或为空")
	}
	if st, err := os.Stat(cfg.RootPath); err != nil || !st.IsDir() {
		return fmt.Errorf("scoop root path 目录不存在: %s", cfg.RootPath)
	}

	// 3.1.1 gh_proxy
	if cfg.GhProxy == "" {
		return fmt.Errorf("scoop config gh-proxy 不存在或为空")
	}
	if !strings.HasSuffix(cfg.GhProxy, "/") {
		cfg.GhProxy += "/"
	}

	// 3.2 proxy 可以为空，无需校验

	// 3.3 go_backup_for_scoop_proxy 与 proxy 成对
	hasProxy := cfg.Proxy != ""
	hasBackup := cfg.GoBackupProxy != ""
	switch {
	case !hasProxy && hasBackup:
		warning("scoop config 中 proxy 为空，但 go-backup-for-scoop-proxy 不为空，请检查")
	case hasProxy && !hasBackup:
		warning("scoop config 中 proxy 不为空，但 go-backup-for-scoop-proxy 为空，请检查")
	}

	_ = filepath.Join // 保持引用
	return nil
}
