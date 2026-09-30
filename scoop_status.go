package main

import (
	"os/exec"
	"strings"
)

// SetCommondStatus set 命令中 Status 列的枚举值
type SetCommondStatus string

const (
	StatusNotGitHub    SetCommondStatus = "Not github"
	StatusSkipped      SetCommondStatus = "Skipped"
	StatusIsGitHub     SetCommondStatus = "Is github"
	StatusManifestMiss SetCommondStatus = "Manifest not found"
	StatusBackupExists SetCommondStatus = "Manifest backup exists"
)

// StatusRow 对应 scoop status -l 中一行 app 记录
type StatusRow struct {
	Name      string           // scoop status 返回信息行：应用名
	Installed string           // scoop status 返回信息行：已安装版本
	Latest    string           // scoop status 返回信息行：最新版本
	Missing   string           // scoop status 返回信息行：缺失的依赖
	Info      string           // scoop status 返回信息行：其它信息
	Bucket    string           // 本程序的属性：桶名
	Status    SetCommondStatus // 本程序的属性：状态值，将来定义为枚举
}

// parseScoopStatus 执行并解析 scoop status -l。
// 规则：只处理只有 Name、Installed Version、Latest Version 的行（3 列），
// 其它行（有 Info 列等）记录但标记为 Skipped。
func parseScoopStatus() ([]StatusRow, error) {
	out, err := exec.Command("scoop", "status", "-l").CombinedOutput()
	if err != nil {
		return nil, err
	}

	var rows []StatusRow
	s := stripAnsi(string(out))
	lines := strings.SplitSeq(s, "\n") // 分为两个语句是为了调试方便

	for line := range lines {
		line := strings.TrimSpace(line)

		if line == "" ||
			strings.HasPrefix(line, "Name") ||
			strings.HasPrefix(line, "----") {
			continue // 跳过空行和标题行。
		}

		// 此处的解析是有问题的。
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue // 跳过少于 3 列的行。实际上不应该发生。
		}

		name := fields[0]
		installed := fields[1]
		latest := fields[2]

		// 有超过 3 列（Missing 列或 Info 列有值）→ 跳过
		if len(fields) > 3 {
			rows = append(rows, StatusRow{
				Name:      name,
				Installed: installed,
				Latest:    latest,
				Status:    "Skipped",
			})
			continue
		}

		rows = append(rows, StatusRow{Name: name, Installed: installed, Latest: latest})
	}

	return rows, nil
}
