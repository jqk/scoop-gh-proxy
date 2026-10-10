package scoop

type OutdatedAppStatus string // set / restore / update 命令中 Status 列的枚举值

const (
	Unknown        OutdatedAppStatus = "Unknown"                // 刚刚初始化，应继续执行
	IsGitHub       OutdatedAppStatus = "Is github"              // 下载链接是 github 的，到当前执行阶段仍是成功的，可继续执行
	NotGitHub      OutdatedAppStatus = "Not github"             // 下载链接不是 github 的，终止执行
	Skipped        OutdatedAppStatus = "Skipped"                // 由于信息不全，或 hold 等原因，跳过并终止执行
	NoManifest     OutdatedAppStatus = "Manifest not found"     // manifest 文件不存在，终止执行
	ManifestError  OutdatedAppStatus = "Manifest error"         // manifest 文件存在但读取失败，终止执行
	BackupExists   OutdatedAppStatus = "Manifest backup exists" // manifest 备份已存在，终止执行
	BackupFailed   OutdatedAppStatus = "Manifest backup failed" // manifest 备份失败，终止执行
	ProxySet       OutdatedAppStatus = "Proxy set"              // url 已带 gh_proxy 前缀，无需重复设置，终止执行
	RestoreSuccess OutdatedAppStatus = "Success"                // restore：还原成功
	RestoreFailed  OutdatedAppStatus = "Failed"                 // restore：还原失败
	Updated        OutdatedAppStatus = "Updated"                // --update：捕获到成功标志
	UpdateFailed   OutdatedAppStatus = "Update failed"          // --update：未捕获到成功标志（scoop 出错时退出码常为 0，不能靠退出码兜底）
)

// OutdatedApp 对应 scoop status -l 中一行 app 记录，还附加有本程序使用的属性。
// restore 流程的记录也复用本类型，仅填充其中用到的字段
type OutdatedApp struct {
	Name             string            // scoop status -l 返回信息行：应用名
	Installed        string            // scoop status -l 返回信息行：已安装版本
	Latest           string            // scoop status -l 返回信息行：最新版本
	Missing          string            // scoop status -l 返回信息行：缺失的依赖
	Info             string            // scoop status -l 返回信息行：其它信息
	Bucket           string            // 本程序的属性：桶名
	Manifest         string            // 本程序的属性：manifest 文件名
	ManifestBackup   string            // 本程序的属性：manifest 备份文件名
	Status           OutdatedAppStatus // 本程序的属性：状态值
	OriginalManifest []byte            // 本程序的属性：manifest 原始内容
	Edits            []ManifestEdit    // 本程序的属性：manifest 中待应用的 url 修改清单
}
