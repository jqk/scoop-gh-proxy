package scoop

import (
	"encoding/json/v2"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// findBucket 读取 apps\<name>\current\install.json 获取 bucket 名
func findBucket(rootPath, appName string) string {
	installPath := filepath.Join(rootPath, "apps", appName, "current")
	installFile := "" // 安装信息文件名，初始以空字符串表示安装信息文件不存在

	// 先确定安装信息文件是否存在
	for _, s := range []string{ // 有以下可能的安装信息文件名
		filepath.Join(installPath, "install.json"),
		filepath.Join(installPath, "scoop-install.json"),
	} {
		if fileExists(s) { // 文件存在
			installFile = s
			break
		}
	}

	if installFile == "" { // 没找到安装信息文件
		return ""
	}

	data, err := os.ReadFile(installFile)
	if err != nil {
		return "" // 按逻辑必然有值，所以返回空字符串表示有错误
	}

	var installInfo struct {
		Bucket string `json:"bucket"` // 安装信息文件中桶名称节点
	}
	if err := json.Unmarshal(data, &installInfo); err != nil {
		return "" // 按逻辑必然有值，所以返回空字符串表示有错误
	}

	return installInfo.Bucket
}

// findManifest 在 buckets 中查找 app 的 manifest 文件。返回空字符串表示没找到
func findManifest(rootPath, appName, bucketName string) string {
	if bucketName == "" { // 确保桶名称有效
		bucketName = findBucket(rootPath, appName)
		if bucketName == "" {
			return "" // 按逻辑必然有值，所以返回空字符串表示有错误
		}
	}

	scoopBucketsRoot := filepath.Join(rootPath, "buckets")
	manifestFileName := appName + ".json" // 不包含路径的 manifestPath 文件名
	manifestPath := filepath.Join(scoopBucketsRoot, bucketName, manifestFileName)

	if fileExists(manifestPath) { // 少数桶直接将 manifest 放在桶目录下
		return manifestPath
	}

	manifestPath = "" // 假设没找到，设置为空
	bucketDir := filepath.Join(scoopBucketsRoot, bucketName, "bucket")

	// 多数桶放在 bucket 目录下。但有部分桶还采用多级目录结构，所以需要 WalkDir()
	filepath.WalkDir(bucketDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() { // 不处理目录，只比较各个目录中的文件
			return nil // 吞掉错误，继续查找。因为反正有错误的结果也是找不到文件
		}

		// Windows 的文件名不区别大小写，直接用等号比较，就会错误地把大小写不同认为是不同的文件
		if strings.EqualFold(d.Name(), manifestFileName) {
			manifestPath = path
			return fs.SkipAll // 找到了，结束查找
		}
		return nil
	})

	return manifestPath // 按逻辑必然有值，所以返回空字符串表示有错误
}

// fillBucketManifest 补全 ManagedApp 的 manifest 及 manifestBackup。
// 定位失败时把状态记录为 NoManifest 并返回 false
func fillBucketManifest(rootPath string, app *ManagedApp) bool {
	if app.Bucket = findBucket(rootPath, app.Name); app.Bucket == "" {
		app.Status = NoManifest
		return false
	}
	if app.Manifest = findManifest(rootPath, app.Name, app.Bucket); app.Manifest == "" {
		app.Status = NoManifest
		return false
	}

	app.ManifestBackup = createManifestBackupName(app.Manifest) // 有 manifest 才确定备份文件名
	return true
}
