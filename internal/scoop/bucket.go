package scoop

import (
	"encoding/json/v2"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const backupSuffix = "-gh-backup.json" // manifest 备份文件的后缀。格式为 <app>-gh-backup.json

// setProxiedManifest 备份原始 manifest，再把应用了代理前缀的新内容写入 manifest 文件
func setProxiedManifest(app *OutdatedApp) error {
	if app.Status != IsGitHub {
		return nil // 不是 github 的，跳过
	}
	if fileExists(app.ManifestBackup) {
		app.Status = BackupExists
		return nil // 备份文件已存在，跳过
	}

	if err := os.Rename(app.Manifest, app.ManifestBackup); err != nil { // 备份
		app.Status = ManifestError
		return err
	}

	// 此处应用 locateManifest 定位出的修改：只替换命中的 url 字面量，其余字节原样保留
	out := applyManifestEdits(app.OriginalManifest, app.Edits)

	return os.WriteFile(app.Manifest, out, 0644) // 创建添加 github 代理后的文件
}

// restoreProxiedManifest 将备份文件恢复为正式文件
func restoreProxiedManifest(app *OutdatedApp) error {
	if err := os.Remove(app.Manifest); err != nil && !os.IsNotExist(err) {
		app.Status = RestoreFailed
		return err
	}

	if err := os.Rename(app.ManifestBackup, app.Manifest); err != nil { // 恢复
		app.Status = RestoreFailed
		return err
	}

	app.Status = RestoreSuccess
	return nil
}

// findProxiedManifests 只读扫描，收集 buckets 下所有已备份（待还原）的 app
func findProxiedManifests(rootPath string) ([]OutdatedApp, error) {
	// scoop 所有 bucket 在此目录下，即 <rootPath>\buckets\，如 E:\Scoop\buckets
	scoopBucketsRoot := filepath.Join(rootPath, "buckets")
	var apps []OutdatedApp
	var bucketName string // 此处声明是为了能在闭包中引用

	appendProxiedApp := func(fileName, manifestDir string) { // 定义闭包是为了少传参
		// manifest 文件名是 <appName> + <backupSuffix>
		if appName, ok := strings.CutSuffix(fileName, backupSuffix); ok {
			apps = append(apps, OutdatedApp{
				Name:           appName,
				Bucket:         bucketName,
				Manifest:       filepath.Join(manifestDir, appName+".json"),
				ManifestBackup: filepath.Join(manifestDir, appName+backupSuffix),
				Status:         Unknown, // 初始化，待还原时更新
			})
		}
	}

	// 获取 scoopBucketsRoot 下的子目录和文件列表
	// 先用两轮 ReadDir()，再 WalkDir() 获取所有 mainfest 进行比较
	// 不全使用 WalkDir() 是为了避免进入 .git 等无关的目录
	bucketList, err := os.ReadDir(scoopBucketsRoot)
	if err != nil {
		return apps, err
	}

	for _, budgetEntry := range bucketList { // 第一轮循环，列出如 E:\Scoop\buckets 下的所有桶
		if !budgetEntry.IsDir() {
			continue // scoopBucketsRoot 下每个子目录就是一个 bucket，所以跳过非目录
		}

		bucketName = budgetEntry.Name()                          // 子目录名就是桶的名称，如 extras
		bucketDir := filepath.Join(scoopBucketsRoot, bucketName) // 如 E:\Scoop\buckets\extras

		bucketItems, err := os.ReadDir(bucketDir) // 获取桶内的子目录和文件列表。少数桶内的 bucket 目录是多层的
		if err != nil {
			continue // 出错不中断，继续处理下一个桶。简化一下没必要的错误处理
		}

		for _, bucketItem := range bucketItems { // 第二轮循环，列出桶中的文件名和目录名
			bucketItemName := bucketItem.Name() // 暂存以避免多次调用 Name()

			if bucketItem.IsDir() { // 优先处理目录，因为大多数桶都是 buckets\<bucket>\bucket\<app>.json 这样的结构
				if bucketItemName == "bucket" { // 过滤掉无关的目录
					dir := filepath.Join(bucketDir, bucketItemName) // 如 E:\Scoop\buckets\extras\bucket

					// 使用 WalkDir() 是因为有少数桶的 bucket 子目录是多层目录。注意 path 是包含路径的完整文件名
					filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
						if err == nil && !d.IsDir() {
							appendProxiedApp(d.Name(), filepath.Dir(path))
						}
						return nil
					})
				}
			} else {
				// 此处已是文件。少数桶将 manifest 直接放在桶的目录下。此时，桶的路径就是 manifest 的路径
				appendProxiedApp(bucketItemName, bucketDir)
			}
		}
	}

	return apps, nil
}

// createManifestBackupName 根据 manifest 文件名，创建其备份文件名
func createManifestBackupName(name string) string {
	return strings.TrimSuffix(name, ".json") + backupSuffix
}

// findBucket 读取 apps\<name>\current\install.json 获取 bucket 名
func findBucket(rootPath, appName string) string {
	installPath := filepath.Join(rootPath, "apps", appName, "current")
	installFile := "" // 安装信息文件名，初始时以空字符串表示安装信息文件不存在

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
