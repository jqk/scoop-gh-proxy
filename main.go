package main

import (
	"os"

	"github.com/fatih/color"

	"github.com/jqk/scoop-gh-proxy/internal/cli"
)

func main() {
	color.NoColor = !cli.IsTTY(os.Stdout)

	if len(os.Args) == 1 {
		cli.RunHelp()
		os.Exit(1)
	}

	var code int
	switch os.Args[1] {
	case "set":
		code = cli.RunSet(false)
	case "restore":
		code = cli.RunRestore(false)
	case "status":
		code = cli.RunStatus()
	case "update":
		code = cli.RunUpdate(os.Args[2:])
	case "help":
		code = cli.RunHelp()
	case "version":
		code = cli.RunVersion()
	default:
		color.Red("未知命令: %s\n", os.Args[1])
		cli.RunHelp()
		code = 1
	}

	// Run* 只返回退出码，统一在这里退出
	os.Exit(code)
}
