package cli

import (
	"fmt"
	"os"

	"github.com/fatih/color"
)

func IsTTY(f *os.File) bool {
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeDevice != 0
}

// info  INFO：普通
func info(format string, args ...any) {
	fmt.Printf(format+"\n", args...)
}

// success SUCCESS：绿色
func success(format string, args ...any) {
	c := color.New(color.FgGreen)
	c.Fprintf(os.Stdout, format+"\n", args...)
}

// warning WARNING：黄色
func warning(format string, args ...any) {
	c := color.New(color.FgYellow)
	c.Fprintf(os.Stderr, format+"\n", args...)
}

// error_ ERROR：红色
func error_(format string, args ...any) {
	c := color.New(color.FgRed)
	c.Fprintf(os.Stderr, format+"\n", args...)
}

// caution 注意：黄色，写 stdout。
// --update 逐 app 的提示行与 scoop 流式输出同写 stdout，保证先后顺序——
// warning 走 stderr，输出重定向（如 PowerShell 2>&1）合并时可能乱序
func caution(format string, args ...any) {
	c := color.New(color.FgYellow)
	c.Fprintf(os.Stdout, format+"\n", args...)
}

// fail 失败：红色，写 stdout。用途同 caution（error_ 走 stderr，仅用于不与流式输出交错的独立消息）
func fail(format string, args ...any) {
	c := color.New(color.FgRed)
	c.Fprintf(os.Stdout, format+"\n", args...)
}
