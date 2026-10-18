# scoop-gh-proxy

Windows 命令行工具，为 [Scoop](https://scoop.sh) 加速 GitHub 下载：把 bucket manifest 中 `https://github.com` 开头的下载链接替换为「你自备的 GitHub 代理地址（gh_proxy）+ 原链接」的形式，使下载流量走代理；并可一键还原。`update` / `install` 命令把「加前缀 → 下载 → 还原」全流程自动化。

## 功能特性

- `set` / `restore` / `status` — 手动修改、还原、预览（不落盘）manifest 中的 GitHub 下载链接
- `update [app...]` — 一条命令更新全部或指定过期软件：自动加前缀、清空 scoop proxy（备份恢复）、逐个流式更新、无论成败还原；不带参数时先执行 `scoop update` 更新 scoop 自身与 buckets
- `install <bucket/app>` — 安装指定应用，GitHub 应用在保护罩内安装，bucket 可省略
- 只修改顶层 `url` 与 `architecture.<架构>.url` 中 `https://github.com` 前缀的 URL，manifest 其余字节逐字保留
- 修改前自动备份为 `<app>-gh-backup.json`；set / restore / update 全部幂等，支持中断自愈
- scoop 输出与下载进度实时透传；单应用失败不中断，成败以 scoop 成功标志判定（不依赖退出码）

## 安装

**下载 Release（推荐）**：从 [Releases](https://github.com/jqk/scoop-gh-proxy/releases) 下载对应架构的 zip（`scoop-gh-<版本>-amd64.zip` / `-arm64.zip`），解压后把 `scoop-gh.exe` 放入 PATH 中的目录。

**源码构建**（需要 Go 与 Git）：

```bat
git clone https://github.com/jqk/scoop-gh-proxy.git
cd scoop-gh-proxy
build.bat
```

产物在 `dist\amd64\` 与 `dist\arm64\`（各含 `scoop-gh.exe` 与 `README.md`）；`build.bat --version` 仅预览版本号。

## 配置

唯一必填项：在 scoop config 中设置 gh_proxy 代理地址（尾部斜杠可省略，程序自动补齐）：

```powershell
scoop config set gh_proxy https://你的gh代理地址/
```

- `gh_proxy` 是本工具的自定义配置键，scoop 自身不使用，不影响其它功能
- `root_path` 必须指向存在的目录（scoop 正常安装自带）
- `proxy` 可为空：update / install 运行期间会被临时清空（自动备份到 `gh_scoop_proxy_backup`，结束后恢复）

## 命令速查

| 命令 | 用途 |
| ---- | ---- |
| `scoop-gh set` | 为待更新应用的 manifest 加代理前缀（自动备份） |
| `scoop-gh restore` | 还原 set 的修改，并恢复被清空的 proxy |
| `scoop-gh status` | 预览将要 set / restore 的明细，不修改任何文件 |
| `scoop-gh update [app...]` | 更新过期软件；不带参数时先执行 `scoop update` 更新 scoop 与 buckets 再全量更新，`update *` 直接全量更新，点名时只更新指定的 app |
| `scoop-gh install <bucket/app>` | 安装指定应用，bucket 可省略（逐桶查找） |
| `scoop-gh help` | 显示帮助 |
| `scoop-gh version` | 显示版本 |

## 快速上手

日常更新——一条命令完成（先执行 `scoop update` 更新 scoop 与 buckets，再更新过期应用，含还原，无需手动收尾；只想更新应用、跳过 scoop 更新时用 `scoop-gh update *`）：

```powershell
scoop-gh update
```

经典三步——自己控制 scoop 命令时使用：

```powershell
scoop-gh set
scoop update <应用>
scoop-gh restore
```

安装新应用：

```powershell
scoop-gh install main/7zip   # 或省略桶名: scoop-gh install 7zip
```

update 输出概览（分组更新：先 Is github 组走保护罩，后 Not github 组直接更新，`[n/m]` 连续编号，最后汇总）：

```text
Apps outdated: 3, to update: 2

App Name  Installed Version  Latest Version  Bucket Name  Status
--------  -----------------  --------------  -----------  ----------
7zip      24.08              24.09           main         Is github
curl      8.10.1             8.11.0          main         Not github
cmirror   0.1.2                                           Skipped

[1/2] 7zip
（scoop update 7zip 的流式输出与下载进度…）
7zip: Updated

----------------------------------------------------------------

[2/2] curl
（scoop update curl 的流式输出…）
curl: Updated

Update Summary: 2 updated, 0 failed
```

## 退出码

| 命令 | 0 | 1 |
| ---- | ---- | ---- |
| set / restore / status | 正常走完（个别应用失败记录在明细表） | 配置错误 |
| update | 全部执行完毕（个别应用失败不影响退出码，见汇总） | 配置错误、`scoop status -l` 失败、遗留清理或 proxy 清空失败 |
| install | 安装成功 | 参数/配置错误、已安装、bucket 或 app 不存在、状态不允许、安装未成功 |
| help / version | 正常 | —（不带参数运行或未知命令时为 1） |

## 文档

完整文档在本仓库 [docs/](https://github.com/jqk/scoop-gh-proxy/blob/main/docs/Home.md) 目录（README 随 Release zip 发布，故使用绝对链接）：

- [安装](https://github.com/jqk/scoop-gh-proxy/blob/main/docs/Install.md) — 获取与安装、scoop config 校验规则
- [命令手册](https://github.com/jqk/scoop-gh-proxy/blob/main/docs/Commands.md) — 全部命令的语法、流程、输出示例、状态值与退出码
- [工作原理](https://github.com/jqk/scoop-gh-proxy/blob/main/docs/Principles.md) — 备份/还原、proxy 保护罩、成败判定、下载进度
- [架构](https://github.com/jqk/scoop-gh-proxy/blob/main/docs/Architecture.md) — 代码分层、文件职责、设计不变量
- [开发](https://github.com/jqk/scoop-gh-proxy/blob/main/docs/Development.md) — 构建、版本号规则、测试与发布流程
- [FAQ](https://github.com/jqk/scoop-gh-proxy/blob/main/docs/FAQ.md) — 常见问题
