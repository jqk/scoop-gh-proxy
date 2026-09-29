package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/fatih/color"
)

func runSet() {
	cfg, err := getScoopConfig()
	if err != nil {
		error_("%s", err)
		os.Exit(1)
	}

	rows, err := parseStatusOutput()
	if err != nil {
		error_("执行 scoop status -l 失败: %v", err)
		os.Exit(1)
	}

	results := make([]statusRow, 0, len(rows))
	for _, row := range rows {
		if row.ProxyStatus == "Skipped" {
			results = append(results, row)
			continue
		}
		results = append(results, processAppForSet(row, cfg))
	}

	if cfg.Proxy != "" {
		if err := exec.Command("scoop", "config", "rm", "proxy").Run(); err != nil {
			warning("scoop config rm proxy 失败: %v", err)
		} else {
			success("已执行 scoop config rm proxy")
		}
	}

	printResultTable(results, cfg)
}

func processAppForSet(row statusRow, cfg scoopConfig) statusRow {
	manifestPath := findManifest(cfg.RootPath, row.Name)
	if manifestPath == "" {
		warning("未找到 %s 的 manifest，跳过", row.Name)
		row.ProxyStatus = "Skipped"
		return row
	}

	ghURL, err := manifestHasGitHubURL(manifestPath)
	if err != nil {
		warning("读取 %s manifest 失败: %v", row.Name, err)
		row.ProxyStatus = "Skipped"
		return row
	}
	if !ghURL {
		row.ProxyStatus = "Not github"
		return row
	}

	ok, err := backupAppJSON(manifestPath)
	if err != nil {
		warning("备份 %s 失败: %v", row.Name, err)
		row.ProxyStatus = "Skipped"
		return row
	}
	if !ok {
		warning("%s 的备份文件已存在，跳过", row.Name)
		row.ProxyStatus = "Skipped"
		return row
	}

	if _, err := patchGitHubURLs(manifestPath, cfg.GhProxy); err != nil {
		error_("修改 %s manifest 失败: %v，还原备份", row.Name, err)
		_ = os.Rename(backupPath(manifestPath), manifestPath)
		row.ProxyStatus = "Skipped"
		return row
	}

	row.ProxyStatus = cfg.GhProxy
	success("已更新 %s", row.Name)
	return row
}

func runReset() {
	cfg, err := getScoopConfig()
	if err != nil {
		error_("%s", err)
		os.Exit(1)
	}

	bucketsDir := filepath.Join(cfg.RootPath, "buckets")
	restored := 0

	filepath.Walk(bucketsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(info.Name(), "-gh-backup.json") {
			return nil
		}
		appJSONPath := strings.TrimSuffix(path, "-gh-backup.json") + ".json"
		_ = os.Remove(appJSONPath)
		if err := os.Rename(path, appJSONPath); err != nil {
			error_("还原 %s 失败: %v", appJSONPath, err)
			return nil
		}
		restored++
		success("已还原 %s", filepath.Base(appJSONPath))
		return nil
	})

	if restored == 0 {
		info("未找到任何 *-gh-backup.json，无需还原")
	}

	if cfg.GoBackupProxy != "" {
		if err := exec.Command("scoop", "config", "proxy", cfg.GoBackupProxy).Run(); err != nil {
			warning("scoop config proxy %s 失败: %v", cfg.GoBackupProxy, err)
		} else {
			success("已执行 scoop config proxy %s", cfg.GoBackupProxy)
		}
	}
}

// findManifest 读取 apps\<name>\current\install.json 获取 bucket，
// 在 buckets\<bucket>\bucket\<app>.json 或 buckets\<bucket>\<app>.json 中查找
func findManifest(rootPath, appName string) string {
	installPath := filepath.Join(rootPath, "apps", appName, "current", "install.json")
	data, err := os.ReadFile(installPath)
	if err != nil {
		return ""
	}
	var install struct {
		Bucket string `json:"bucket"`
	}
	if err := json.Unmarshal(data, &install); err != nil || install.Bucket == "" {
		return ""
	}
	bucketsBase := filepath.Join(rootPath, "buckets")
	for _, candidate := range []string{
		filepath.Join(bucketsBase, install.Bucket, "bucket", appName+".json"),
		filepath.Join(bucketsBase, install.Bucket, appName+".json"),
	} {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return ""
}

// printResultTable 按第八节格式输出结果表
func printResultTable(results []statusRow, cfg scoopConfig) {
	const (
		hdrName  = "Name"
		hdrVer   = "Installed Version"
		hdrLate  = "Latest Version"
		hdrProxy = "Proxy Status"
	)

	nameW, verW, lateW, proxyW := len(hdrName), len(hdrVer), len(hdrLate), len(hdrProxy)
	for _, r := range results {
		if len(r.Name) > nameW {
			nameW = len(r.Name)
		}
		if len(r.Installed) > verW {
			verW = len(r.Installed)
		}
		if len(r.Latest) > lateW {
			lateW = len(r.Latest)
		}
		if len(r.ProxyStatus) > proxyW {
			proxyW = len(r.ProxyStatus)
		}
	}

	rowFmt := fmt.Sprintf("%%-%ds  %%-%ds  %%-%ds  %%-%ds\n", nameW, verW, lateW, proxyW)
	fmt.Printf(rowFmt, hdrName, hdrVer, hdrLate, hdrProxy)
	fmt.Printf(rowFmt,
		dashRun(nameW), dashRun(verW), dashRun(lateW), dashRun(proxyW))
	fmt.Println()

	for _, r := range results {
		status := r.ProxyStatus
		if status == "" {
			status = "Not github"
		}
		c := pickColor(status)
		c.Fprintf(os.Stdout, rowFmt, r.Name, r.Installed, r.Latest, status)
	}
}

func pickColor(status string) *color.Color {
	switch {
	case status == "Skipped":
		return color.New(color.FgYellow)
	case status == "Not github":
		return color.New(color.FgWhite)
	default:
		return color.New(color.FgGreen)
	}
}

func dashRun(n int) string {
	return strings.Repeat("-", n)
}
