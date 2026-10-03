package main

import (
	"fmt"
	"os/exec"
	"strings"
)

type OutdatedAppStatus string // SetCommandStatus set 命令中 Status 列的枚举值

const (
	Unknown       OutdatedAppStatus = "Unknown"                // 刚刚初始化，应继续执行
	IsGitHub      OutdatedAppStatus = "Is github"              // 下载链接是 github 的，到当前执行阶段仍是成功的，可继续执行
	NotGitHub     OutdatedAppStatus = "Not github"             // 下载链接不是 github 的，终止执行
	SetSkipped    OutdatedAppStatus = "Skipped"                // 由于信息不全，或 hold 等原因，跳过并终止执行
	NoManifest    OutdatedAppStatus = "Manifest not found"     // manifest 文件不存在，终止执行
	ManifestError OutdatedAppStatus = "Manifest error"         // manifest 文件存在但读取失败，终止执行
	BackupExists  OutdatedAppStatus = "Manifest backup exists" // manifest 备份已存在，终止执行
	BackupFailed  OutdatedAppStatus = "Manifest backup failed" // manifest 备份失败，终止执行
)

// OutdatedApp 对应 scoop status -l 中一行 app 记录，还附加有本程序使用的属性
type OutdatedApp struct {
	Name             string            // scoop status -l 返回信息行：应用名
	Installed        string            // scoop status -l 返回信息行：已安装版本
	Latest           string            // scoop status -l 返回信息行：最新版本
	Missing          string            // scoop status -l 返回信息行：缺失的依赖
	Info             string            // scoop status -l 返回信息行：其它信息
	Bucket           string            // 本程序的属性：桶名
	Manifest         string            // 本程序的属性：manifest 文件名
	ManifestBackup   string            // 本程序的属性：manifest 备份文件名
	Status           OutdatedAppStatus // 本程序的属性：状态值
	OriginalManifest []byte            // 本程序的属性：manifest 原始内容
	Edits            []manifestEdit    // 本程序的属性：manifest 中待应用的 url 修改清单
}

// getOutdatedApps 执行并解析 scoop status -l，并据 rootPath 填充每行的 bucket。
// 非空行第一行为标题行，第二行为分隔线，其余为数据行。
// 按分隔线确定各列起始位置，据此切分每行数据。
func getOutdatedApps() (apps []OutdatedApp, e error) {
	out, err := exec.Command("scoop", "status", "-l").CombinedOutput()
	if err != nil {
		return nil, err
	}

	s := stripAnsi(string(out))
	lines := strings.SplitSeq(s, "\n") // 分为两个语句是为了调试方便

	// 如果命令执行结果有空行，直接处理会增加许多逻辑判断，所以先把可能的空行排除
	var nonEmptyLines []string // 非空的，有内容的行
	for line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			nonEmptyLines = append(nonEmptyLines, line)
		}
	}

	lineCount := len(nonEmptyLines)
	if lineCount == 1 && nonEmptyLines[0] == "Everything updated" {
		return apps, nil // 没有待升级的软件，直接返回。scoop status -l 不会什么都不返回
	} else if lineCount < 2 { // 至少要有标题行和分隔线
		return nil, fmt.Errorf("scoop status -l 输出缺少标题行或分隔线")
	}

	// 分隔线行的 "-" 字符位置决定了各列的起始位置，是有内容的第 2 行，且有 5 列
	starts := findColumnStarts(nonEmptyLines[1])
	if len(starts) < 5 {
		return nil, fmt.Errorf("scoop status -l 分隔线列数不足 5，无法解析")
	}

	for _, line := range nonEmptyLines[2:] { // 状态内容从第 3 行开始
		apps = append(apps, createOutdatedApp(line, starts))
	}

	return apps, nil
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

// createOutdatedApp 按列起始位置切分一行，返回 OutdatedApp 结构体。
func createOutdatedApp(line string, starts []int) OutdatedApp {
	length := len(line)

	cut := func(a, b int) string { // 从行字符串中，按起止位置切分段落的闭包
		if a >= length {
			return ""
		}
		end := min(b, length)
		if a >= end {
			return ""
		}

		return strings.TrimSpace(line[a:end])
	}

	app := OutdatedApp{
		Name:      cut(starts[0], starts[1]),
		Installed: cut(starts[1], starts[2]),
		Latest:    cut(starts[2], starts[3]),
		Missing:   cut(starts[3], starts[4]),
		Info:      cut(starts[4], length),
		Status:    Unknown, // 初始化，待后续处理时更新
	}

	if app.Missing != "" || app.Info != "" || app.Latest == "" || app.Installed == "" {
		app.Status = SetSkipped // 信息不全，或过多，如被值过 scoop hold，或者已经 deprecated 等
	}

	return app
}
