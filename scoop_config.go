package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// ScoopConfig 是本程序用到的 scoop config 中定义的属性
type ScoopConfig struct {
	RootPath           string // scoop 使用的属性，定义 scoop 的根目录
	Proxy              string // scoop 使用的属性，定义其 http 代理
	GhScoopProxyBackup string // 本程序专用属性，备份 Proxy 值
	GhProxy            string // 本程序专用属性，定义 github 数据代理
}

// getScoopConfig 执行 scoop config 命令，获取配置值。
func getScoopConfig() (ScoopConfig, error) {
	out, err := exec.Command("scoop", "config").Output() // 此处返回的是包含转义码在内的字节数组
	if err != nil {
		return ScoopConfig{}, fmt.Errorf("执行 scoop config 失败: %w", err)
	}

	cfg := ScoopConfig{}
	s := stripAnsi(string(out)) // 分为两个语句是为了调试方便
	lines := strings.SplitSeq(s, "\n")

	for line := range lines {
		if key, val, found := getKeyValueFromLine(line); found {
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
	}

	if err := validateConfig(&cfg); err != nil {
		return cfg, err
	}

	return cfg, nil
}

// getKeyValueFromLine 分析行信息，得到 key 和 value 值
func getKeyValueFromLine(line string) (key string, val string, found bool) {
	line = strings.TrimSpace(line)
	if line == "" {
		return "", "", false
	}

	before, after, ok := strings.Cut(line, ":")
	if !ok {
		return "", "", false
	}

	key = strings.TrimSpace(before)
	val = strings.TrimSpace(after)
	found = true

	return
}

// validateConfig 校验 ScoopConfig 对象的属性
func validateConfig(cfg *ScoopConfig) error {
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
		warning("scoop config 中 proxy 为空，但 gh_scoop_proxy_backup 不为空")
	case hasProxy && !hasBackup:
		warning("scoop config 中 proxy 不为空，但 gh_scoop_proxy_backup 为空")
	}

	return nil
}
