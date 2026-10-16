# 安装

## 前置条件

- Windows（amd64 或 arm64）
- 已安装 [Scoop](https://scoop.sh)
- 一个可用的 GitHub 代理服务，支持以 URL 前缀形式使用，即 `https://<代理地址>/https://github.com/...`

## 配置 gh_proxy（必填）

所有命令启动时都会读取并校验 scoop config，`gh_proxy` 不存在或为空会直接报错退出（退出码 1）：

```powershell
scoop config set gh_proxy https://你的gh代理地址/
```

- 尾部斜杠可省略，程序会自动补齐
- `gh_proxy` 是本工具使用的自定义配置键，scoop 自身不认识它，不影响 scoop 其它功能

scoop config 校验规则：

| 配置键 | 要求 |
| ---- | ---- |
| `root_path` | 必填，且对应目录必须存在，否则报错退出（正常安装的 scoop 自带） |
| `gh_proxy` | 必填非空，否则报错退出；尾部自动补 `/` |
| `proxy` | 可为空。update / install 执行期间会被临时清空（备份到 `gh_scoop_proxy_backup`，结束后自动恢复） |
| `gh_scoop_proxy_backup` | 本工具自动管理，无需手动设置 |
| `aria2-enabled` | 可选，仅影响下载进度的显示方式，见[工作原理](Principles.md) |

## 获取程序

### 方式一：下载 Release（推荐）

从 [Releases](https://github.com/jqk/scoop-gh-proxy/releases) 下载对应架构的 zip：

- `scoop-gh-<版本>-amd64.zip`（64 位）
- `scoop-gh-<版本>-arm64.zip`（ARM64）

解压得到 `scoop-gh.exe` 与 `README.md`，把 exe 放入 PATH 中的目录即可。

### 方式二：源码构建

需要 Go 与 Git：

```bat
git clone https://github.com/jqk/scoop-gh-proxy.git
cd scoop-gh-proxy
build.bat
```

`build.bat` 构建双架构（amd64 / arm64），输出到 `dist\amd64\` 与 `dist\arm64\`（各含 `scoop-gh.exe` 与 `README.md`），版本号从 git tag 推导。

- `build.bat --version` 仅预览目标版本号，不构建
- 也可以直接 `go build -o dist\scoop-gh.exe .`：单架构跟随宿主机，版本号显示为 `dev`

构建细节与版本号规则见[开发](Development.md)。

## 验证

```powershell
scoop-gh version
```

输出程序信息头（版权行 + 名称版本行）即安装成功：

```text
Copyright (c) 1999-2026 Not a dream Co., Ltd.
scoop-gh-proxy 0.7.6, 2026-10-10

```

版本行格式为 `scoop-gh-proxy <版本>[, <提交日期>]`；直接 `go build` 构建时版本为 `dev` 且无日期。
