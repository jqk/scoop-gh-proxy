# AGENTS.md

## 项目概述
Windows Go CLI 工具，用于修改 scoop bucket 中 `[app].json` 的 GitHub URL（加 gh_proxy 前缀），并支持还原。

## 构建
```
build.bat             # 输出 dist\amd64\ 与 dist\arm64\ 下的 scoop-gh.exe（各含 README.md），注入版本号与构建时间
build.bat --version   # 仅预览目标版本号，不构建
go build -o dist\scoop-gh.exe .   # 直接构建（不经 build.bat，单架构跟随宿主机）：Version 为 dev、无构建时间；输出到 dist，勿在项目根目录留 exe
```

版本号规则：从 Git tag 计算——最近 tag 的 patch 号 + 其后提交次数（如 tag v0.1.0 之后 5 个提交 → 0.1.5；无 tag 时以 0.0.0 为基数、累计全部提交数）。build.bat 通过 `-ldflags -X` 注入 `internal/cli` 的 Version 与 BuildTime

GitHub 自动发布（.github/workflows/release.yml）：推送 `v*` tag 触发（ubuntu runner 交叉编译 windows amd64/arm64），步骤内正则校验 tag 必须为 `v1.0.0` 格式否则失败；版本取自 tag（去 v 前缀）、构建时间为 runner 当前时间，经同样的 `-ldflags -X` 注入；发布产物为 `scoop-gh-<版本>-<架构>.zip`（内含 scoop-gh.exe 与 README.md），经 softprops/action-gh-release@v2 发布

## 分层
- `internal/scoop` — 核心库：只接收参数、返回数据与 error，不打印、不 os.Exit。将来 GUI 与 internal/cli 平级，共用核心
- `internal/cli` — 命令行交互层：彩色输出、明细表在这里；Run* 命令入口返回退出码、不直接 os.Exit，由 main 统一退出
- 依赖方向单向：main → internal/cli → internal/scoop，反向禁止

## 文件结构
- `main.go` — 入口，参数分发（--set / --restore / --status / --update / --help / --version），按 cli.Run* 返回的退出码统一 os.Exit
- `internal/cli/cli.go` — 全部 Run* 命令入口（RunSet / RunRestore / RunStatus / RunUpdate / RunHelp / RunVersion），返回退出码；--update 的 proxy 保护罩在此层（PrepareUpdate 成功后 defer 注册 RestoreScoopProxy）
- `internal/cli/printer.go` — 全部输出：用法（printUsage）、汇总与明细表（printSetSummary / printRestoreSummary / printSetTable / printRestoreTable / printAppOutcome / printUpdateSummary）
- `internal/cli/output.go` — 彩色输出（info/success/warning/error_/caution/fail）、IsTTY；caution/fail 走 stdout
- `internal/scoop/config.go` — 解析并校验 scoop config；设置与恢复 proxy（SetScoopProxy / RestoreScoopProxy，成功后同步更新 ScoopConfig 内存值）；读取 aria2-enabled（Aria2Enabled，控制 --update 的下载进度来源：aria2 用自带 \r 进度流，默认下载器用 downloadProgress 自绘）
- `internal/scoop/status.go` — 解析 scoop status -l（纯解析，不做文件查找）
- `internal/scoop/outdated_app.go` — OutdatedApp 与状态枚举定义（set / restore / update 共用）
- `internal/scoop/runner.go` — set / restore 命令主流程（SetProxyForOutdatedApps / RestoreOutdatedAppManifests）；prepareAppManifest 为 set 与 update 共用的定位分析门控
- `internal/scoop/update.go` — --update 的核心：PrepareUpdate 准备与分组（遗留清理复用 RestoreOutdatedAppManifests）、UpdateProxiedApp / UpdatePlainApp 逐个更新（UpdatePlainApp 为 Not github 组预留，暂无调用方）、matchUpdateSuccess 成功标志表 + updateVerdict（以成功标志为最终定论）
- `internal/scoop/download_progress.go` — downloadProgress：aria2 关闭时自绘下载进度（scoop 检测到输出重定向会关闭自带进度条；轮询 cache 的 *.download 临时文件大小，总量从 "Downloading <url> (<size>)..." 行解析；方法 nil-safe，aria2 启用时传 nil）
- `internal/scoop/exec_kill.go` — Windows Job Object（x/sys/windows）：KILL_ON_JOB_CLOSE 保证本程序退出（含异常）时子进程树一并终止，防孤儿
- `internal/scoop/manifest.go` — manifest 的只读定位与应用（jsontext 流式）
- `internal/scoop/manifest_test.go` — 定位/应用的单测
- `internal/scoop/download_progress_test.go` — Feed 解析 / renderDownloadLine / humanSize 的单测
- `internal/scoop/update_test.go` — matchUpdateSuccess / updateVerdict 的单测
- `internal/cli/printer_test.go` — printTable 的单测
- `internal/scoop/bucket.go` — 备份/还原 [app].json；install.json 与 manifest 查找；fillBucketManifest 定位补全
- `internal/scoop/tools.go` — stripAnsi、fileExists、runScoop（短超时全量捕获）、runScoopStream（流式透传 + 逐行扫描，无超时，observe 回调供调用方捕获行；命中成功标志置位，错误行不收集、随透传直接显示；显示侧经 lineFilter 只输出有内容的行：分隔符折叠（\r+ 及其后至多一个 \n）、空行与纯空白行丢弃、行尾空白裁剪、行首缩进保留，对齐真实终端观感）

## 关键约定
- Windows only，路径用 `filepath`
- manifest 修改不预定义 struct，用 `encoding/json/jsontext`（json v2 系列）逐 token 流式扫描：
  - `locateManifestEdits` 只读定位，返回 `[]ManifestEdit`（字节区间 + 替换文本）
  - `applyManifestEdits` 在别处应用，仅替换命中区间，其余字节原样保留
- 只处理顶层 `url` 与 `architecture.<架构>.url`（checkver、autoupdate 等不处理）
- URL 值可以是 `string` 或字符串数组，统一处理
- 只改 `https://github.com` 前缀的 URL，已带 gh_proxy 前缀的不重复改
- install.json 解析用 struct + `encoding/json/v2`（与 manifest 的 jsontext 同为 v2 系列）
- bucket 布局：优先 `buckets\<b>\bucket\<app>.json`，备选 `buckets\<b>\<app>.json`
- install.json 位于 `apps\<name>\current\install.json` 或 `scoop-install.json`

## 校验规则（internal/scoop/config.go）
- `root_path` 必填且目录存在 → 否则退出
- `gh_proxy` 必填，尾部补 `/` → 否则退出
- `proxy` 可为空

## 结果状态值（OutdatedAppStatus）
- `Unknown` — 初始，待处理
- `Is github` — 存在待修改的 GitHub URL
- `Not github` — 无待修改的 URL
- `Proxy set` — url 已带 gh_proxy 前缀（重复执行 set 时的正常状态，无需修改）
- `Success` — restore 还原成功
- `Failed` — restore 还原失败
- `Skipped` — status 行信息不全（Info/Missing 列非空、版本缺失等）
- `Manifest not found` — bucket 或 manifest 文件找不到
- `Manifest error` — manifest 读取或解析失败
- `Manifest backup exists` — 备份已存在
- `Manifest backup failed` — 备份失败
- `Updated` — --update：捕获到成功标志（"was installed successfully" 或 "Latest versions for all apps are installed"）
- `Update failed` — --update：未捕获到成功标志（scoop 出错时退出码常为 0，不能靠退出码兜底；错误行随透传直接显示）

## 设计不变量（幂等性，重构或 GUI 化时不得破坏）
- set 重复执行安全：已处理的 manifest 跳过（Proxy set / Manifest backup exists），不重复修改、不覆盖已有备份
- restore 重复执行安全：无备份的 app 无操作；proxy 已恢复（Proxy == GhScoopProxyBackup）时跳过
- proxy 设置与恢复条件对称：set 仅在 Proxy 非空时备份并清空；restore 仅在 GhScoopProxyBackup 非空且 Proxy 为空时恢复（本程序运行期间用户手动设置的其它非空 proxy 不被覆盖）
- manifest 中未被修改的字节逐字节保留：applyManifestEdits 只做命中区间的替换
- --update 单个 app 失败不中断：失败只记录状态，继续下一个，直到全部执行完
- --update 的成败判定以成功标志为准（updateVerdict）：成功标志（was installed successfully / Latest versions for all apps are installed）由 scoop 在流程末尾打印，是最终定论；未见成功标志一律判失败。错误行不另行收集——随透传直接显示，成败只看成功标志；成功标志表集中在 update.go，取自 scoop 源码（e:\scoop\apps\scoop）与真实输出
- --update 的 proxy 保护罩：PrepareUpdate 成功后 cli 层立即 defer RestoreScoopProxy，函数任何返回路径（含 panic）退出时统一恢复；RestoreScoopProxy 幂等（无备份或已恢复时为 no-op），故单调用点即可，无需防重标志。RunUpdate 以返回退出码代替 os.Exit，defer 因此也覆盖 SetScoopProxy 失败路径
- --update 的逐 app 提示行（[n/m] 表头、Proxy set 警告、更新结果）与 scoop 流式输出同写 stdout（caution/fail），保证先后顺序不被 stderr 合并打乱；error_/warning 走 stderr，仅用于不与流式输出交错的独立消息（配置错误、收尾恢复失败等）
- --update 开工前先清理遗留：自动还原上次运行遗留的备份与 proxy（复用 RestoreOutdatedAppManifests），保证从干净状态开始（恢复的 proxy 已同步到内存 cfg，无需二次 GetScoopConfig）
- --update 的 Is github app 无论更新成败都还原 manifest；还原失败优先展示为 Failed（manifest 仍处于已修改状态，可再执行 --restore）

## 命令行为
- `--set` — 定位 + 备份 + 修改 [app].json，输出 "Manifest to Set: N" + 明细表
- `--restore` — 还原 backup，输出 "Manifest to restore: N" + 明细表（Name + Bucket）
- `--status` — dry-run，先输出 restore 明细，再输出 set 明细，不修改任何文件
- `--update` — 不执行 scoop update（不更新 scoop 自身与桶）。流程：清理遗留 → status -l 分组（三组：Skipped / Not github 保留待用不更新；只更新 Is github 与 Proxy set 组）→ setScoopProxy 清空 proxy（备份到 gh_scoop_proxy_backup）→ 逐个 scoop update <app>（runScoopStream 流式透传，无超时：下载时长不可推测，挂死时需手动终止）→ restoreScoopProxy 恢复 proxy → 汇总。单 app 失败不影响退出码；退出码 1 = 配置错误、status -l 无法执行/解析失败或 proxy 清空失败
- N 为 0 时不输出明细表
- dryRun 标志只存在于命令流程层（SetProxyForOutdatedApps / RestoreOutdatedAppManifests），manifest 层只做只读定位

## 共享逻辑
- manifest 的只读定位与 backup 的只读扫描均为纯查询，set、restore、status、update 四条命令共用同一实现；
  是否落盘仅由命令层决定（set/update 的 dryRun 或落盘步骤），核心层不打印、不退出、不调用 os.Exit
- prepareAppManifest（定位 bucket + 分析 manifest）为 SetProxyForOutdatedApps 与 PrepareUpdate 共用
- --update 的流式输出由注入的 io.Writer 决定（CLI 传 os.Stdout，将来 GUI 可传自己的 writer）；成功标志表集中在 update.go 的 updateSuccessMarkers

## 编译验证
```
go build ./...
go vet ./...
go test ./...
```
