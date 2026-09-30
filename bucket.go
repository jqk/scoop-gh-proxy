package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// RestoreCommandStatus 表示单条 restore 记录的状态
type RestoreCommandStatus string

const (
	RestoreSuccess RestoreCommandStatus = "Success"
	RestoreSkipped RestoreCommandStatus = "Skipped"
	RestoreFailed  RestoreCommandStatus = "Failed"
)

// RestoreCommandItem 一条待还原的 app 记录
type RestoreCommandItem struct {
	Name   string
	Bucket string
	Path   string               // [app].json 的完整路径（backup 同目录）
	Status RestoreCommandStatus // 执行后填充
}

const backupSuffix = "-gh-backup.json"

func backupManifest(item *SetCommandItem) error {
	if item.Status != IsGitHub {
		return nil // 不是 github 的，跳过
	}
	if fileExists(item.ManifestBackup) {
		item.Status = BackupExists
		return nil // 备份文件已存在，跳过
	}

	if err := os.Rename(item.Manifest, item.ManifestBackup); err != nil { // 备份
		item.Status = ManifestError
		return err
	}

	out, err := json.MarshalIndent(item.ChangedManifest, "", "  ")
	if err != nil {
		return err
	}
	out = append(out, '\n')
	return os.WriteFile(item.Manifest, out, 0644)
}

// collectRestoreItems 只读扫描，收集 buckets 下所有 backupSuffix 文件
func collectRestoreItems(rootPath string) []RestoreCommandItem {
	bucketsDir := filepath.Join(rootPath, "buckets")
	var items []RestoreCommandItem

	filepath.Walk(bucketsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil // 跳过目录
		}
		if !strings.HasSuffix(info.Name(), backupSuffix) {
			return nil // 跳过非备份文件
		}

		appName := stripBackupSuffix(info.Name())
		items = append(items, RestoreCommandItem{
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
	return filepath.Join(dir, strings.TrimSuffix(name, ".json")+backupSuffix)
}

// backupAppJSON 将 [app].json 备份为 [app]-gh-backup.json。
// 如果备份文件已存在，返回 false 表示调用方应跳过。
func backupAppJSON(appJSONPath string) (bool, error) {
	bp := backupPath(appJSONPath)
	if fileExists(bp) {
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
	if !fileExists(bp) {
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
	return strings.TrimSuffix(name, backupSuffix) + ".json"
}

func appendBackupSuffix(name string) string {
	return strings.TrimSuffix(name, ".json") + backupSuffix
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

// findManifest 在 buckets\<bucket>\bucket\<app>.json 或 buckets\<bucket>\<app>.json 中查找
func findManifest(rootPath, appName, bucket string) string {
	if bucket == "" {
		bucket = findBucket(rootPath, appName)
		if bucket == "" {
			return ""
		}
	}

	bucketsBase := filepath.Join(rootPath, "buckets")

	// 按给出数组项的顺序，是先查找 buckets\<bucket>\bucket\<app>.json，再查找 buckets\<bucket>\<app>.json
	// 因为大多数桶都是前者，少数是后者
	for _, candidate := range []string{
		filepath.Join(bucketsBase, bucket, "bucket", appName+".json"),
		filepath.Join(bucketsBase, bucket, appName+".json"),
	} {
		if fileExists(candidate) { // 文件存在
			return candidate
		}
	}

	return ""
}
