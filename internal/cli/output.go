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
