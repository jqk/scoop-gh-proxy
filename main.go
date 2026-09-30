package main

import (
	"fmt"
	"os"

	"github.com/fatih/color"
)

const version = "1.0.0"

func main() {
	color.NoColor = !isTTY(os.Stdout)

	switch len(os.Args) {
	case 1:
		printUsage()
		os.Exit(1)
	default:
		switch os.Args[1] {
		case "--set":
			runSet(false)
		case "--restore":
			runRestore(false)
		case "--status":
			runStatus()
		case "--help", "-h":
			printUsage()
		case "--version", "-v":
			fmt.Println(version)
		default:
			color.Red("未知参数: %s\n", os.Args[1])
			printUsage()
			os.Exit(1)
		}
	}
}

func printUsage() {
	color.New(color.FgWhite).Println(fmt.Sprintf("scoop-gh-proxy %s", version))
	fmt.Println()
	fmt.Println("用法:")
	fmt.Println("  scoop-gh-proxy --set    将 bucket 中的 GitHub URL 替换为带 gh_proxy 的 URL")
	fmt.Println("  scoop-gh-proxy --restore  还原已备份的原始 [app].json")
	fmt.Println("  scoop-gh-proxy --status 预览将要还原/修改的明细（不实际修改）")
	fmt.Println()
	fmt.Println("选项:")
	fmt.Println("  -h, --help     显示帮助")
	fmt.Println("  -v, --version  显示版本")
}
