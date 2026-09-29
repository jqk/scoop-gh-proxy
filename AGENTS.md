# AGENTS.md

## 项目概述
Windows Go CLI 工具，用于修改 scoop bucket 中 `[app].json` 的 GitHub URL（加 gh_proxy 前缀），并支持还原。

## 构建
```
go build -o scoop-gh-proxy.exe .
# 或 Windows: build.bat
```

## 文件结构
- `main.go` — 入口，flag 分发（--set / --reset / --status / --help / --version）
- `cli.go` — set/reset/status 业务逻辑
- `scoop_config.go` — 解析并校验 scoop config
- `scoop_status.go` — 解析 scoop status -l
- `manifest.go` — 读写 [app].json、修补 GitHub URL
- `backup.go` — 备份/还原 [app].json
- `output.go` — 彩色输出（info/success/warning/error_）

## 关键约定
- Windows only，路径用 `filepath`
- JSON 用 `map[string]any`，不预定义 struct
- URL 值可以是 `string` 或 `[]any`，都要处理
- 只改 `https://github.com` 前缀的 URL，已带 gh_proxy 前缀的不重复改
- bucket 布局：优先 `buckets\<b>\bucket\<app>.json`，备选 `buckets\<b>\<app>.json`
- install.json 位于 `apps\<name>\current\install.json`

## 校验规则（scoop_config.go）
- `root_path` 必填且目录存在 → 否则退出
- `gh_proxy` 必填，尾部补 `/` → 否则退出
- `proxy` 可为空
- `proxy` 与 `go_backup_for_scoop_proxy` 需成对，不匹配只 warn

## 结果状态值
- `Skipped` — 该行有 Info 列，或备份已存在，或 manifest 找不到
- `Not github` — manifest 无 GitHub URL
- 实际 gh_proxy 值 — 已修改

## 命令行为
- `--set` — 备份 + 修改 [app].json，输出 "Manifest to Set: N" + 明细表
- `--reset` — 还原 backup，输出 "Manifest to Reset: N" + 明细表（Name + Bucket）
- `--status` — dry-run，先输出 reset 明细，再输出 set 明细，不修改任何文件
- N 为 0 时不输出明细表

## 共享逻辑
- `classifyAppForSet` — 只读分类（set 与 status 共用）
- `collectResetItems` — 只读扫描 backup（reset 与 status 共用）

## 编译验证
```
go build -o scoop-gh-proxy.exe .
go vet ./...
```
