# AGENTS.md

## 项目概述
Windows Go CLI 工具，用于修改 scoop bucket 中 `[app].json` 的 GitHub URL（加 gh_proxy 前缀），并支持还原。

## 构建
```
go build -o scoop-gh-proxy.exe .
# 或 Windows: build.bat
```

## 分层
- `internal/scoop` — 核心库：只接收参数、返回数据与 error，不打印、不 os.Exit。将来 GUI 与 internal/cli 平级，共用核心
- `internal/cli` — 命令行交互层：彩色输出、明细表、退出码只在这里
- 依赖方向单向：main → internal/cli → internal/scoop，反向禁止

## 文件结构
- `main.go` — 入口，参数分发（--set / --restore / --status / --update / --help / --version）
- `internal/cli/cli.go` — set/restore/status 的输出（汇总、明细表、printRestoreTable）
- `internal/cli/update.go` — RunUpdate：--update 的输出（遗留警告、分组明细、进度行、汇总、退出码）；proxy 保护罩在此层（PrepareUpdate 成功后 defer 注册 RestoreScoopProxy）
- `internal/cli/output.go` — 彩色输出（info/success/warning/error_）、IsTTY
- `internal/scoop/config.go` — 解析并校验 scoop config；设置与恢复 proxy（setScoopProxy / restoreScoopProxy，成功后同步更新 ScoopConfig 内存值）
- `internal/scoop/status.go` — 解析 scoop status -l（纯解析，不做文件查找）
- `internal/scoop/outdated_app.go` — OutdatedApp 与状态枚举定义（set / restore / update 共用）
- `internal/scoop/runner.go` — set / restore 命令主流程（SetProxyForOutdatedApps / RestoreOutdatedAppManifests）；prepareAppManifest 为 set 与 update 共用的定位分析门控
- `internal/scoop/update.go` — --update 的核心：PrepareUpdate 准备与分组、UpdateProxiedApp 逐个更新（UpdatePlainApp 为 Not github 组预留，暂无调用方）、classifyUpdateLine 与错误标记表
- `internal/scoop/exec_kill.go` — Windows Job Object 进程树终止（x/sys/windows）
- `internal/scoop/manifest.go` — manifest 的只读定位与应用（jsontext 流式）
- `internal/scoop/manifest_test.go` — 定位/应用的单测
- `internal/scoop/update_test.go` — classifyUpdateLine 的单测
- `internal/scoop/bucket.go` — 备份/还原 [app].json；install.json 与 manifest 查找
- `internal/scoop/tools.go` — stripAnsi、fileExists、runScoop（短超时全量捕获）、runScoopStream（流式透传 + 逐行扫描）

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
- `Updated` — --update：scoop update <app> 成功
- `Update failed` — --update：更新失败（输出命中错误标记、超时或进程异常退出）

## 设计不变量（幂等性，重构或 GUI 化时不得破坏）
- set 重复执行安全：已处理的 manifest 跳过（Proxy set / Manifest backup exists），不重复修改、不覆盖已有备份
- restore 重复执行安全：无备份的 app 无操作；proxy 已恢复（Proxy == GhScoopProxyBackup）时跳过
- proxy 设置与恢复条件对称：set 仅在 Proxy 非空时备份并清空；restore 仅在 GhScoopProxyBackup 非空且与当前 Proxy 不同时恢复
- manifest 中未被修改的字节逐字节保留：applyManifestEdits 只做命中区间的替换
- --update 单个 app 失败不中断：错误输出命中标记、超时或非零退出都只记录状态，继续下一个，直到全部执行完
- --update 的 proxy 保护罩：PrepareUpdate 成功后 cli 层立即 defer RestoreScoopProxy，函数任何返回路径（含 panic）退出时统一恢复；RestoreScoopProxy 幂等（无备份或已恢复时为 no-op），故单调用点即可，无需防重标志
- --update 开工前先清理遗留：自动还原上次运行遗留的备份与 proxy，保证从干净状态开始（restoreScoopProxy 同步内存 cfg，无需二次 GetScoopConfig）
- --update 的 Is github app 无论更新成败都还原 manifest；还原失败优先展示为 Failed（manifest 仍处于已修改状态，可再执行 --restore）

## 命令行为
- `--set` — 定位 + 备份 + 修改 [app].json，输出 "Manifest to Set: N" + 明细表
- `--restore` — 还原 backup，输出 "Manifest to restore: N" + 明细表（Name + Bucket）
- `--status` — dry-run，先输出 restore 明细，再输出 set 明细，不修改任何文件
- `--update` — 不执行 scoop update（不更新 scoop 自身与桶）。流程：清理遗留 → status -l 分组（三组：Skipped / Not github 保留待用不更新；只更新 Is github 与 Proxy set 组）→ setScoopProxy 清空 proxy（备份到 gh_scoop_proxy_backup）→ 逐个 scoop update <app>（runScoopStream 流式透传，appUpdateTimeout=10 分钟超时）→ restoreScoopProxy 恢复 proxy → 汇总。单 app 失败不影响退出码；退出码 1 = 配置错误或 status -l 无法执行/解析失败
- N 为 0 时不输出明细表
- dryRun 标志只存在于命令流程层（SetProxyForOutdatedApps / RestoreOutdatedAppManifests），manifest 层只做只读定位

## 共享逻辑
- manifest 的只读定位与 backup 的只读扫描均为纯查询，set、restore、status、update 四条命令共用同一实现；
  是否落盘仅由命令层决定（set/update 的 dryRun 或落盘步骤），核心层不打印、不退出、不调用 os.Exit
- prepareAppManifest（定位 bucket + 分析 manifest）为 SetProxyForOutdatedApps 与 PrepareUpdate 共用
- --update 的流式输出由注入的 io.Writer 决定（CLI 传 os.Stdout，将来 GUI 可传自己的 writer）；错误标记表集中在 update.go 的 updateErrorMarkers

## 编译验证
```
go build -o scoop-gh-proxy.exe .
go vet ./...
go test ./...
```
