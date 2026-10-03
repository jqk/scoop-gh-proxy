package scoop

import (
	"context"
	"os"
	"os/exec"
	"regexp"
	"time"
)

// 执行命令行命令，得到的输出中有以下字符
var ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// stripAnsi 去除输出中的 ANSI 转义码（如 \x1b[32;1m、\x1b[0m）
func stripAnsi(s string) string {
	return ansiRe.ReplaceAllString(s, "")
}

// runScoopTimeout scoop 子命令的超时
const runScoopTimeout = 10 * time.Second

// runScoop 执行 scoop 子命令，返回合并 stdout/stderr 的输出（已去除 ANSI 转义码）。
// 统一收口 scoop 命令的执行并施加超时，避免子进程挂起时程序卡死（GUI 化后尤其重要）
func runScoop(args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), runScoopTimeout)
	defer cancel()

	out, err := exec.CommandContext(ctx, "scoop", args...).CombinedOutput()
	return stripAnsi(string(out)), err
}

// fileExists 判断文件是否存在
func fileExists(path string) bool {
	_, err := os.Stat(path) // 文件存在
	return err == nil
}
