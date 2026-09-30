package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// restoreStatus 表示单条 restore 记录的结果
type restoreStatus string

const (
	restoreSuccess restoreStatus = "Success"
	restoreSkipped restoreStatus = "Skipped"
	restoreFailed  restoreStatus = "Failed"
)

// restoreItem 一条待还原的 app 记录
type restoreItem struct {
	Name   string
	Bucket string
	Path   string        // [app].json 的完整路径（backup 同目录）
	Status restoreStatus // 执行后填充
}

// collectRestoreItems 只读扫描，收集 buckets 下所有 *-gh-backup.json
func collectRestoreItems(rootPath string) []restoreItem {
	bucketsDir := filepath.Join(rootPath, "buckets")
	var items []restoreItem

	filepath.Walk(bucketsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil // 跳过目录
		}
		if !strings.HasSuffix(info.Name(), "-gh-backup.json") {
			return nil // 跳过非备份文件
		}

		appName := stripBackupSuffix(info.Name())
		items = append(items, restoreItem{
			Name:   appName,
			Bucket: deriveBucketFromPath(bucketsDir, path),
			Path:   stripBackupSuffix(path),
		})
		return nil
	})

	return items
}

// deriveBucketFromPath 从 [app]-gh-backup.json 的路径推出 bucket 名
// 路径形如 buckets\<b>\bucket\<app>-gh-backup.json 或 buckets\<b>\<app>-gh-backup.json
func deriveBucketFromPath(bucketsDir, fullPath string) string {
	rel, err := filepath.Rel(bucketsDir, fullPath)
	if err != nil {
		return ""
	}
	parts := strings.Split(rel, string(filepath.Separator))
	if len(parts) == 0 {
		return ""
	}
	return parts[0]
}

// backupPath 返回 [app].json 对应的备份文件路径
func backupPath(appJSONPath string) string {
	dir := filepath.Dir(appJSONPath)
	name := filepath.Base(appJSONPath)
	return filepath.Join(dir, strings.TrimSuffix(name, ".json")+"-gh-backup.json")
}

// backupAppJSON 将 [app].json 备份为 [app]-gh-backup.json。
// 如果备份文件已存在，返回 false 表示调用方应跳过。
func backupAppJSON(appJSONPath string) (bool, error) {
	bp := backupPath(appJSONPath)
	if _, err := os.Stat(bp); err == nil {
		return false, nil
	}
	content, err := os.ReadFile(appJSONPath)
	if err != nil {
		return false, err
	}
	return true, os.WriteFile(bp, content, 0644)
}

// restoreAppJSON 将 [app]-gh-backup.json 还原为 [app].json
// （删除 [app].json，rename backup → [app].json）
func restoreAppJSON(appJSONPath string) error {
	bp := backupPath(appJSONPath)
	if _, err := os.Stat(bp); err != nil {
		return fmt.Errorf("备份文件不存在: %s", bp)
	}
	_ = os.Remove(appJSONPath)
	return os.Rename(bp, appJSONPath)
}

// findBackupFiles 在指定目录中查找所有 *-gh-backup.json 文件
func findBackupFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var results []string
	for _, e := range entries {
		if e.Type().IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasSuffix(name, "-gh-backup.json") {
			results = append(results, filepath.Join(dir, name))
		}
	}
	return results, nil
}

// stripBackupSuffix 从 [app]-gh-backup.json 还原出 [app].json 的文件名
func stripBackupSuffix(name string) string {
	return strings.TrimSuffix(name, "-gh-backup.json") + ".json"
}

// findBucket 读取 apps\<name>\current\install.json 获取 bucket 名
func findBucket(rootPath, appName string) string {
	installPath := filepath.Join(rootPath, "apps", appName, "current", "install.json")
	data, err := os.ReadFile(installPath)
	if err != nil {
		return ""
	}
	var install struct {
		Bucket string `json:"bucket"`
	}
	if err := json.Unmarshal(data, &install); err != nil {
		return ""
	}
	return install.Bucket
}
