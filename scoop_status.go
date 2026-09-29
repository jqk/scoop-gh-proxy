package main

import (
	"os/exec"
	"strings"
)

// statusRow 对应 scoop status -l 中一行 app 记录
type statusRow struct {
	Name        string
	Installed   string
	Latest      string
	ProxyStatus string // set 结果：Not github / Skipped / 实际 gh_proxy 值
}

// parseScoopStatus 执行并解析 scoop status -l。
// 规则：只处理只有 Name、Installed Version、Latest Version 的行（3 列），
// 其它行（有 Info 列等）记录但标记为 Skipped。
func parseScoopStatus() ([]statusRow, error) {
	out, err := exec.Command("scoop", "status", "-l").CombinedOutput()
	if err != nil {
		return nil, err
	}

	var rows []statusRow
	for raw := range strings.SplitSeq(string(out), "\n") {
		trimmed := strings.TrimSpace(raw)

		if trimmed == "" ||
			strings.HasPrefix(trimmed, "Name") ||
			strings.HasPrefix(trimmed, "----") {
			continue // 跳过空行和标题行。
		}

		fields := strings.Fields(trimmed)
		if len(fields) < 3 {
			continue // 跳过少于 3 列的行。实际上不应该发生。
		}

		name := fields[0]
		installed := fields[1]
		latest := fields[2]

		// 有超过 3 列（Missing 列或 Info 列有值）→ 跳过
		if len(fields) > 3 {
			rows = append(rows, statusRow{
				Name:        name,
				Installed:   installed,
				Latest:      latest,
				ProxyStatus: "Skipped",
			})
			continue
		}

		rows = append(rows, statusRow{Name: name, Installed: installed, Latest: latest})
	}
	return rows, nil
}
