package main

import (
	"os"
	"regexp"
)

var ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// stripAnsi 去除输出中的 ANSI 转义码（如 \x1b[32;1m、\x1b[0m）
func stripAnsi(s string) string {
	return ansiRe.ReplaceAllString(s, "")
}

// fileExists 判断文件是否存在
func fileExists(path string) bool {
	if _, err := os.Stat(path); err == nil { // 文件存在
		return true
	}
	return false
}
