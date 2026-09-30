package main

import (
	"fmt"
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

// parseScoopStatus 执行并解析 scoop status -l，并据 rootPath 填充每行的 bucket。
// 非空行第一行为标题行，第二行为分隔线，其余为数据行。
// 按分隔线确定各列起始位置，据此切分每行数据。
func parseScoopStatus(rootPath string) ([]StatusRow, error) {
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

	var rows []StatusRow
	for _, line := range nonEmptyLines[2:] { // 状态内容从第 3 行开始
		row := createStatusRow(line, starts)
		row.Bucket = findBucket(rootPath, row.Name)
		rows = append(rows, row)
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

// createStatusRow 按列起始位置切分一行，返回 StatusRow 结构体
func createStatusRow(line string, starts []int) StatusRow {
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

	row := StatusRow{
		Name:      cut(starts[0], starts[1]),
		Installed: cut(starts[1], starts[2]),
		Latest:    cut(starts[2], starts[3]),
		Missing:   cut(starts[3], starts[4]),
		Info:      cut(starts[4], length),
		Status:    StatusIsGitHub, // 大多数都是 github 的，先假设是 github，后续再判断
	}

	if row.Missing != "" || row.Info != "" || row.Latest == "" || row.Installed == "" {
		row.Status = StatusSkipped
	}

	return row
}
