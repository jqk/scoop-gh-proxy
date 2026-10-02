package main

import (
	"fmt"
	"os/exec"
	"strings"
)

type SetCommandStatus string // SetCommandStatus set 命令中 Status 列的枚举值

const (
	Unknown       SetCommandStatus = "Unknown"                // 刚刚初始化，应继续执行
	IsGitHub      SetCommandStatus = "Is github"              // 下载链接是 github 的，到当前执行阶段仍是成功的，可继续执行
	NotGitHub     SetCommandStatus = "Not github"             // 下载链接不是 github 的，终止执行
	SetSkipped    SetCommandStatus = "Skipped"                // 由于信息不全，或 hold 等原因，跳过并终止执行
	NoManifest    SetCommandStatus = "Manifest not found"     // manifest 文件不存在，终止执行
	ManifestError SetCommandStatus = "Manifest error"         // manifest 文件存在但读取失败，终止执行
	BackupExists  SetCommandStatus = "Manifest backup exists" // manifest 备份已存在，终止执行
	BackupFailed  SetCommandStatus = "Manifest backup failed" // manifest 备份失败，终止执行
)

// SetCommandItem 对应 scoop status -l 中一行 app 记录，还有本程序的属性
type SetCommandItem struct {
	Name            string           // scoop status -l 返回信息行：应用名
	Installed       string           // scoop status -l 返回信息行：已安装版本
	Latest          string           // scoop status -l 返回信息行：最新版本
	Missing         string           // scoop status -l 返回信息行：缺失的依赖
	Info            string           // scoop status -l 返回信息行：其它信息
	Bucket          string           // 本程序的属性：桶名
	Manifest        string           // 本程序的属性：manifest 文件名
	ManifestBackup  string           // 本程序的属性：manifest 备份文件名
	Status          SetCommandStatus // 本程序的属性：状态值
	ChangedManifest map[string]any   // 本程序的属性：manifest 更改后的内容
}

// parseScoopStatus 执行并解析 scoop status -l，并据 rootPath 填充每行的 bucket。
// 非空行第一行为标题行，第二行为分隔线，其余为数据行。
// 按分隔线确定各列起始位置，据此切分每行数据。
func parseScoopStatus(rootPath string) ([]SetCommandItem, error) {
	out, err := exec.Command("scoop", "status", "-l").CombinedOutput()
	if err != nil {
		return nil, err
	}

	s := stripAnsi(string(out))
	lines := strings.SplitSeq(s, "\n") // 分为两个语句是为了调试方便

	var nonEmptyLines []string // 非空的，有内容的行
	for line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			nonEmptyLines = append(nonEmptyLines, line)
		}
	}
	if len(nonEmptyLines) < 2 { // 至少要有标题行和分隔线
		return nil, fmt.Errorf("scoop status -l 输出缺少标题行或分隔线")
	}

	// 分隔线行的 "-" 字符位置决定了各列的起始位置，是有内容的第 2 行，且有 5 列
	starts := findColumnStarts(nonEmptyLines[1])
	if len(starts) < 5 {
		return nil, fmt.Errorf("scoop status -l 分隔线列数不足 5，无法解析")
	}

	var rows []SetCommandItem
	for _, line := range nonEmptyLines[2:] { // 状态内容从第 3 行开始
		rows = append(rows, createSetCommandItem(line, starts, rootPath))
	}

	return rows, nil
}

// findColumnStarts 根据分隔线确定 5 列的起始位置（按 "-" 分组）
func findColumnStarts(sep string) (starts []int) {
	length := len(sep)
	i := 0

	for i < length {
		for sep[i] != '-' { // 跳过空格，找到下一个 "-" 的位置
			i++
		}
		// 此时的 i 不会大于等于 length
		starts = append(starts, i)

		for i < length && sep[i] == '-' {
			i++
		}
	}

	return
}

// createSetCommandItem 按列起始位置切分一行，返回 StatusRow 结构体。
func createSetCommandItem(line string, starts []int, rootPath string) SetCommandItem {
	length := len(line)
	cut := func(a, b int) string {
		if a >= length {
			return ""
		}
		end := min(b, length)
		if a >= end {
			return ""
		}

		return strings.TrimSpace(line[a:end])
	}

	item := SetCommandItem{
		Name:      cut(starts[0], starts[1]),
		Installed: cut(starts[1], starts[2]),
		Latest:    cut(starts[2], starts[3]),
		Missing:   cut(starts[3], starts[4]),
		Info:      cut(starts[4], length),
		Status:    Unknown, // 初始化，待后续处理时更新
	}

	if item.Missing != "" || item.Info != "" || item.Latest == "" || item.Installed == "" {
		item.Status = SetSkipped // 信息不全，或过多，如被值过 scoop hold，或者已经 deprecated 等
	}

	return item
}
