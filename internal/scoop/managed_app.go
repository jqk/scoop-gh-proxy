package scoop

// ManagedAppStatus set / restore / update / install 命令中 Status 列的枚举值
type ManagedAppStatus string

const (
	Unknown        ManagedAppStatus = "Unknown"                // 刚刚初始化，应继续执行
	IsGitHub       ManagedAppStatus = "Is github"              // 下载链接是 github 的，到当前执行阶段仍是成功的，可继续执行
	NotGitHub      ManagedAppStatus = "Not github"             // 下载链接不是 github 的，终止执行
	Skipped        ManagedAppStatus = "Skipped"                // 由于信息不全，或 hold 等原因，跳过并终止执行
	NoManifest     ManagedAppStatus = "Manifest not found"     // manifest 文件不存在，终止执行
	ManifestError  ManagedAppStatus = "Manifest error"         // manifest 文件存在但读取失败，终止执行
	BackupExists   ManagedAppStatus = "Manifest backup exists" // manifest 备份已存在，终止执行
	BackupFailed   ManagedAppStatus = "Manifest backup failed" // manifest 备份失败，终止执行
	ProxySet       ManagedAppStatus = "Proxy set"              // url 已带 gh_proxy 前缀，无需重复设置，终止执行
	RestoreSuccess ManagedAppStatus = "Success"                // restore：还原成功
	RestoreFailed  ManagedAppStatus = "Failed"                 // restore：还原失败
	Updated        ManagedAppStatus = "Updated"                // update：捕获到成功标志
	UpdateFailed   ManagedAppStatus = "Update failed"          // update / install：未捕获到成功标志（scoop 出错时退出码常为 0，不能靠退出码兜底）
	InstallFailed  ManagedAppStatus = "Install failed"         // install：未捕获到成功标志（失败时 scoop 输出 ERROR 行并经 abort 以退出码 1 结束，或静默失败退出码 0，如架构不支持——成败只看成功标志）
	Installed      ManagedAppStatus = "Installed"              // install：捕获到成功标志
)

// ManagedApp 一条由本程序接管的 app 记录：update 组（源自 scoop status -l 的一行）
// 与 install（单个定位结果）共用；restore 扫描备份时也复用，仅填充其中用到的字段
type ManagedApp struct {
	Name             string           // scoop status -l 返回信息行：应用名
	Installed        string           // scoop status -l 返回信息行：已安装版本
	Latest           string           // scoop status -l 返回信息行：最新版本
	Missing          string           // scoop status -l 返回信息行：缺失的依赖
	Info             string           // scoop status -l 返回信息行：其它信息
	Bucket           string           // 本程序的属性：桶名
	Manifest         string           // 本程序的属性：manifest 文件名
	ManifestBackup   string           // 本程序的属性：manifest 备份文件名
	Status           ManagedAppStatus // 本程序的属性：状态值
	Failure          string           // 本程序的属性：scoop 输出中最后命中的失败标志行（空 = 未命中），失败时供输出层提示原因
	OriginalManifest []byte           // 本程序的属性：manifest 原始内容
	Edits            []ManifestEdit   // 本程序的属性：manifest 中待应用的 url 修改清单
}
