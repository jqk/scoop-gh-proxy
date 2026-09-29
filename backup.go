package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

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
