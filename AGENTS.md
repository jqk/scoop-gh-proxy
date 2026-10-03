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
- `main.go` — 入口，参数分发（--set / --restore / --status / --help / --version）
- `internal/cli/cli.go` — set/restore/status 的输出（汇总、明细表）
- `internal/cli/output.go` — 彩色输出（info/success/warning/error_）、IsTTY
- `internal/scoop/config.go` — 解析并校验 scoop config；设置与恢复 proxy（setScoopProxy / RestoreScoopProxy）
- `internal/scoop/status.go` — 解析 scoop status -l；OutdatedApp 与状态枚举定义；fillBucketManifest 定位 bucket 与 manifest
- `internal/scoop/runner.go` — set / restore 命令主流程（SetProxyForOutdatedApps / RestoreOutdatedAppManifests）
- `internal/scoop/manifest.go` — manifest 的只读定位与应用（jsontext 流式）
- `internal/scoop/manifest_test.go` — 定位/应用的单测
- `internal/scoop/bucket.go` — 备份/还原 [app].json；install.json 与 manifest 查找
- `internal/scoop/tools.go` — stripAnsi、fileExists

## 关键约定
- Windows only，路径用 `filepath`
- manifest 修改不预定义 struct，用 `encoding/json/jsontext`（json v2 系列）逐 token 流式扫描：
  - `locateManifestEdits` 只读定位，返回 `[]ManifestEdit`（字节区间 + 替换文本）
  - `applyManifestEdits` 在别处应用，仅替换命中区间，其余字节原样保留
- 只处理顶层 `url` 与 `architecture.<架构>.url`（checkver、autoupdate 等不处理）
- URL 值可以是 `string` 或字符串数组，统一处理
- 只改 `https://github.com` 前缀的 URL，已带 gh_proxy 前缀的不重复改
- install.json 解析用 struct + `encoding/json`（v1）
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
- `Skipped` — status 行信息不全（Info/Missing 列非空、版本缺失等）
- `Manifest not found` — bucket 或 manifest 文件找不到
- `Manifest error` — manifest 读取或解析失败
- `Manifest backup exists` — 备份已存在
- `Manifest backup failed` — 备份失败

## 命令行为
- `--set` — 定位 + 备份 + 修改 [app].json，输出 "Manifest to Set: N" + 明细表
- `--restore` — 还原 backup，输出 "Manifest to restore: N" + 明细表（Name + Bucket）
- `--status` — dry-run，先输出 restore 明细，再输出 set 明细，不修改任何文件
- N 为 0 时不输出明细表
- dryRun 标志只存在于命令流程层（SetProxyForOutdatedApps / RestoreOutdatedAppManifests），manifest 层只做只读定位

## 共享逻辑
- `locateManifest` — 只读定位待修改 url 并流转 Status（set 与 status 共用）
- `FindRestoreCommandItems` — 只读扫描 backup（restore 与 status 共用）

## 编译验证
```
go build -o scoop-gh-proxy.exe .
go vet ./...
go test ./...
```
