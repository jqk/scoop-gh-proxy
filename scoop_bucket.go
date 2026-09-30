package main

import (
	"encoding/json"
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
	Name           string               // 应用名
	Bucket         string               // 桶名
	Manifest       string               // manifest 文件名
	ManifestBackup string               // manifest 备份文件名
	Status         RestoreCommandStatus // 执行后填充
}

func NewRestoreCommandItem(name, bucket, manifestDir string) RestoreCommandItem {
	return RestoreCommandItem{
		Name:           name,
		Bucket:         bucket,
		Manifest:       filepath.Join(manifestDir, name+".json"),
		ManifestBackup: filepath.Join(manifestDir, name+backupSuffix),
	}
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

func restoreManifest(item *RestoreCommandItem) error {
	if err := os.Remove(item.Manifest); err != nil && !os.IsNotExist(err) {
		item.Status = RestoreFailed
		return err
	}

	if err := os.Rename(item.ManifestBackup, item.Manifest); err != nil { // 备份
		item.Status = RestoreFailed
		return err
	}

	item.Status = RestoreSuccess
	return nil
}

// findRestoreItems 只读扫描，收集 buckets 下所有 backupSuffix 文件
func findRestoreItems(rootPath string) []RestoreCommandItem {
	bucketsDir := filepath.Join(rootPath, "buckets")
	var items []RestoreCommandItem

	entries, err := os.ReadDir(bucketsDir)
	if err != nil {
		return nil
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue // 跳过非目录
		}

		bucketName := entry.Name()
		bucketDir := filepath.Join(bucketsDir, bucketName)
		bucketEntries, err := os.ReadDir(bucketDir)
		if err != nil {
			continue
		}

		for _, bucketEntry := range bucketEntries {
			bucketEntryName := bucketEntry.Name()

			if bucketEntry.IsDir() { // 优先处理目录，因为大多数桶都是 buckets\<bucket>\bucket\<app>.json 这种结构
				if bucketEntryName == "bucket" {
					dir := filepath.Join(bucketDir, "bucket")
					filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
						if err != nil || info.IsDir() {
							return nil // 继续处理
						}
						if !strings.HasSuffix(info.Name(), backupSuffix) {
							return nil // 跳过非备份文件，继续处理
						}

						appName := strings.TrimSuffix(info.Name(), backupSuffix)
						items = append(items, NewRestoreCommandItem(appName, bucketName, dir))
						return nil
					})
				}
			} else if appName, ok := strings.CutSuffix(bucketEntryName, backupSuffix); ok {
				items = append(items, NewRestoreCommandItem(appName, bucketName, bucketDir))
			}
		}
	}

	return items
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
