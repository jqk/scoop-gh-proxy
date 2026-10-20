# scoop-gh-proxy

scoop-gh-proxy 是一个 Windows 命令行工具，用于加速 Scoop 通过 GitHub 下载软件：

它把 bucket 中 manifest 文件里 `https://github.com` 开头的下载链接，替换为「你自备的 GitHub 代理地址（gh_proxy）+ 原链接」的形式，使 `scoop install` / `scoop update` 的下载流量走代理；完成后可一键还原，manifest 恢复原样。

在修改/还原之外，工具还提供 `update` 与 `install` 命令，把「加前缀 → 下载 → 还原」的完整流程自动化，一条命令即可完成更新或安装。

## 功能一览

| 命令 | 用途 |
| ---- | ---- |
| `scoop-gh set` | 为待更新应用的 manifest 加代理前缀（修改前自动备份） |
| `scoop-gh restore` | 还原 set 的全部修改，并恢复被清空的 scoop proxy |
| `scoop-gh status` | 预览将要 set / restore 的明细，不修改任何文件 |
| `scoop-gh update [app...]` | 更新过期软件（自动处理 manifest 与 proxy，结束后还原）；不带参数时先执行 `scoop update` 更新 scoop 与 buckets |
| `scoop-gh install <bucket/app>` | 安装指定应用（GitHub 应用在保护罩内安装） |
| `scoop-gh list` | 列出已安装应用，按更新时间升序（最久未更新的在最前） |
| `scoop-gh help` / `version` | 显示帮助 / 版本 |

## 特性

- **只动下载链接**：只修改顶层 `url` 与 `architecture.<架构>.url` 中 `https://github.com` 前缀的 URL；未被修改的字节逐字保留
- **自动备份与还原**：修改前备份为 `<app>-gh-backup.json`，restore / update 结束后自动还原
- **幂等安全**：重复执行 set / restore / update 均安全，已处理的 manifest 自动跳过
- **单应用失败不中断**：update 逐个应用执行，失败记录状态后继续下一个，最后统一汇总
- **流式输出**：scoop 的输出与下载进度实时透传，观感与直接执行 scoop 一致

## 快速开始

```powershell
# 1. 配置 GitHub 代理（必填，替换为你可用的 gh 代理地址）
scoop config set gh_proxy https://你的gh代理地址/

# 2. 一条命令更新全部过期软件（先更新 scoop 与 buckets，再自动加前缀、清 proxy、更新、还原）
scoop-gh update
```

也可以走经典三步：`scoop-gh set` → `scoop update <app>` → `scoop-gh restore`。

详见 [安装](Install.md) 与 [命令手册](Commands.md)。

## 文档导航

| 页面 | 内容 |
| ---- | ---- |
| [安装](Install.md) | 获取与安装、scoop config 前置要求 |
| [命令手册](Commands.md) | 全部命令的语法、流程、输出示例与退出码 |
| [工作原理](Principles.md) | 备份/还原、proxy 保护罩、成败判定、进度显示 |
| [架构](Architecture.md) | 代码分层、文件职责、manifest 流式编辑、设计不变量 |
| [开发](Development.md) | 构建、版本号规则、测试与发布流程 |
| [FAQ](FAQ.md) | 常见问题 |
