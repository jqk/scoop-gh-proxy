package scoop

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// ScoopConfig 是本程序用到的 scoop config 中定义的属性
type ScoopConfig struct {
	RootPath           string // scoop 使用的属性，定义 scoop 的根目录
	Proxy              string // scoop 使用的属性，定义 scoop update 命令使用的 http 代理。本程序的 set 命令会清空此值，restore 命令会恢复此值
	GhScoopProxyBackup string // 本程序专用属性，备份 Proxy 值。restore 命令会使用此值恢复 Proxy
	GhProxy            string // 本程序专用属性，定义面向 github 下载链接的数据代理
}

// GetScoopConfig 执行 scoop config 命令，获取配置值。
func GetScoopConfig() (ScoopConfig, error) {
	out, err := exec.Command("scoop", "config").Output() // 此处返回的是包含转义码在内的字节数组
	if err != nil {
		return ScoopConfig{}, fmt.Errorf("执行 scoop config 失败: %w", err)
	}

	cfg := ScoopConfig{}
	s := stripAnsi(string(out))
	lines := strings.SplitSeq(s, "\n") // 分为两个语句是为了调试方便

	for line := range lines {
		if key, val, found := getKeyValue(line); found {
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

	if cfg.GhProxy != "" && !strings.HasSuffix(cfg.GhProxy, "/") {
		cfg.GhProxy += "/" // 处理后可直接使用，不必再判断、添加
	}

	if err := validateConfig(&cfg); err != nil {
		return cfg, err
	}

	return cfg, nil
}

// getKeyValue 分析行信息，得到 key 和 value 值
func getKeyValue(line string) (key string, val string, found bool) {
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
	// 5.1 root_path
	if cfg.RootPath == "" {
		return fmt.Errorf("scoop config 中 root_path 不存在或为空")
	}
	if st, err := os.Stat(cfg.RootPath); err != nil || !st.IsDir() {
		return fmt.Errorf("scoop config 中 root_path 对应目录不存在: %s", cfg.RootPath)
	}

	// 5.3 gh_proxy
	if cfg.GhProxy == "" {
		return fmt.Errorf("scoop config 中 gh_proxy 不存在或为空")
	}

	return nil
}

func setScoopProxy(cfg *ScoopConfig) error {
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

func restoreScoopProxy(cfg *ScoopConfig) error {
	if cfg.GhScoopProxyBackup != "" && cfg.Proxy != cfg.GhScoopProxyBackup {
		if err := exec.Command("scoop", "config", "proxy", cfg.GhScoopProxyBackup).Run(); err != nil {
			return err
		}
	}
	return nil
}
