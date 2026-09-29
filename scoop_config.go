package main

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

type scoopConfig struct {
	RootPath           string
	Proxy              string
	GhScoopProxyBackup string
	GhProxy            string
}

// stripAnsi 去除输出中的 ANSI 转义码（如 \x1b[32;1m、\x1b[0m）
var ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func stripAnsi(s string) string {
	return ansiRe.ReplaceAllString(s, "")
}

// getScoopConfig 执行 scoop config 获取配置值
func getScoopConfig() (scoopConfig, error) {
	out, err := exec.Command("scoop", "config").Output()
	if err != nil {
		return scoopConfig{}, fmt.Errorf("执行 scoop config 失败: %w", err)
	}

	cfg := scoopConfig{}
	s := stripAnsi(string(out))

	lines := strings.SplitSeq(s, "\n")
	for line := range lines {
		line = strings.TrimSpace(strings.TrimRight(line, "\r"))
		if line == "" {
			continue
		}

		before, after, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}

		key := strings.TrimSpace(before)
		val := strings.TrimSpace(after)

		switch key {
		case "root_path":
			cfg.RootPath = val
		case "proxy":
			cfg.Proxy = val
		case "gh_scoop_proxy_backup":
			cfg.GhScoopProxyBackup = val
		case "gh_proxy":
			cfg.GhProxy = val
		}
	}

	if err := validateConfig(&cfg); err != nil {
		return cfg, err
	}

	return cfg, nil
}

// validateConfig 校验 root_path / gh_proxy / proxy / gh_scoop_proxy_backup
func validateConfig(cfg *scoopConfig) error {
	// 3.1 root_path
	if cfg.RootPath == "" {
		return fmt.Errorf("scoop config 中 root_path 不存在或为空")
	}
	if st, err := os.Stat(cfg.RootPath); err != nil || !st.IsDir() {
		return fmt.Errorf("scoop config 中 root_path 对应目录不存在: %s", cfg.RootPath)
	}

	// 3.2 proxy 可以为空，无需校验
	// 3.3 gh_proxy
	if cfg.GhProxy == "" {
		return fmt.Errorf("scoop config 中 gh_proxy 不存在或为空")
	}
	if !strings.HasSuffix(cfg.GhProxy, "/") {
		cfg.GhProxy += "/"
	}

	// 3.4 gh_scoop_proxy_backup 与 proxy 成对
	hasProxy := cfg.Proxy != ""
	hasBackup := cfg.GhScoopProxyBackup != ""

	switch {
	case !hasProxy && hasBackup:
		warning("scoop config 中 proxy 为空，但 gh_scoop_proxy_backup 不为空，请检查")
	case hasProxy && !hasBackup:
		warning("scoop config 中 proxy 不为空，但 gh_scoop_proxy_backup 为空，请检查")
	}

	return nil
}
