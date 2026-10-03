# 修改 scoop bucket manifest 文件中 url 的内容的工具

## 一、 功能

使用 github 专用代理地址，替换 scoop bucket manifest 文件中 url 的内容，以加速下载。

## 二、 基础技术选型

- Go 当前最新版 Go
- OS Windows only
- CLI Go 标准库 flag 或专门 CLI 库
- JSON 标准库。manifest 修改用 `encoding/json/jsontext`（Go 1.27 的 json v2 系列），install.json 用 `encoding/json`
- 文件 标准库 os / filepath
- 执行 Scoop os/exec
- 输出 os.Stdout / os.Stderr
- 彩色输出 github.com/fatih/color
- 日志文件 不需要
- 配置文件 不需要
- 数据库 不需要
- Git 程序本身不直接依赖 Git 操作
- GUI 第一版不做

尽量使用标准库 + 一个颜色库。

## 三、 CLI 设计

```bash
scoop-gh-proxy --set
scoop-gh-proxy --restore
scoop-gh-proxy --status

# 如果没有参数，就相当于 help
scoop-gh-proxy --help
scoop-gh-proxy --version
```

对于 `set` 和 `restore` 命令，基本执行顺序是：

- 获取 scoop config
- 执行 set 或 restore 逻辑

对于 `status` 命令，基本执行顺序是：

- 先执行 restore 命令
- 再执行 set 命令
- 但两者均不执行实际修改操作

## 四、 功能

### 4.1 set

- 清除 scoop config 中的 proxy 配置，避免和 github 专用代理冲突。
- 设置 github 专用代理。
- 此后可执行 `scoop update` 命令。
- 执行完 `scoop update` 命令后，请执行 `restore` 命令恢复被修改的 scoop config。

```text
执行 scoop config 获取并检查配置信息
        ↓
执行 scoop status -l 命令获取软件项状态
        ↓
如果有需要更新的 app，针对所有需更新的 app 执行
        ↓
从 scoop\apps 中找到需要更新的 App
        ↓
在其 current 目录中定位并分析 install.json，得到 bucket
        ↓
进入 scoop\buckets\[前面得到的 bucket]\bucket 目录。

备份 [app].json 为 [app]-gh-backup.json。如果备份文件已存在，则报警，跳过当前软件
        ↓
修改 GitHub URL。修改一级元素 url 或三级元素 architecture.xxx.url。
只修改起始为 https://github.com 的 url。
将 gh_proxy 添加到该 url 之前成为组合 url。
至此，针对某个 app 的修改结束
        ↓
如果 scoop config 的 proxy 不为空，执行 scoop config rm proxy
```

以下为 `install.json` 的内容：

```json
{
    "bucket": "extras",
    "architecture": "64bit"
}
```

`scoop\buckets\[bucket]` 下，一般有 `bucket` 目录，保存所有 app 的 manifest 文件。但也有少数 bucket 没有 `bucket` 目录，而是直接在仓库目录下保存所有 app 的 manifest 文件。

以下是 `scoop status -l` 的输出示例：

```text
Name    Installed Version Latest Version Missing Dependencies Info
----    ----------------- -------------- -------------------- ----
alma    0.4.150           0.4.151
cmirror 0.1.2                                                 Deprecated, Manifest removed
git     2.55.0.5          2.56.0
uv      0.12.19           0.12.20
```

- 只处理只有 `Name、Installed Version、Latest Version` 的行，其它行跳过。
- manifest 中，只要有一个 `github.com` 的 url，则认定义该 manifest 需修改。
- 所有 url 都不需要修改，则跳过该 manifest。

假设 `alma` 下载链接不以 `https://github.com` 开始，则应输出以下结果。

```text
Manifest to Set: 4

App Name    Installed Version Latest Version Bucket Name Status
--------    ----------------- -------------- ----------- ------
alma        0.4.150           0.4.151        ajqk        Not github
cmirror     0.1.2                                        Skipped
git         2.55.0.5          2.56.0         main        Is github
uv          0.12.19           0.12.20        extras      Is github
```

如果 `Manifest to Set: 0`，则无后续明细输出。

Status 列的取值：

- `Is github` — 存在待修改的 GitHub URL
- `Proxy set` — url 已带代理前缀（重复执行 `set` 时显示，无需处理）
- `Not github` — 无 GitHub URL
- `Skipped` — 信息不全或被 hold 等，跳过

### 4.2 restore 命令

- 在执行完 `set` 命令及 `scoop update` 命令后，执行本命令，恢复被修改的 scoop config。

```text
执行 scoop config 获取并检查配置信息
        ↓
遍历 scoop\buckets，找到有 [app]-gh-backup.json 的软件项
        ↓
删除 [app].json
        ↓
将 [app]-gh-backup.json 更名为 [app].json
至此，针对某个 app 的修改结束
        ↓
如果 scoop config 的 gh_scoop_proxy_backup 不为空，
执行 scoop config proxy gh_scoop_proxy_backup的实际值
```

输出内容：

```text
Manifest to restore: 3

App Name    Bucket Name Status
--------    ----------- ------
alma        lemon       Success
git         main        Skipped
uv          extras      Failed
```

如果 `Manifest to restore: 0`，则无后续明细输出。

### 4.3 status 命令

先执行 restore 命令，后执行 set 命令，只不过不真正修改任何文件，只是输出要变更的明细。输出内容参见这两个命令。

## 五、 获取并校验 scoop config 信息

通过执行 `scoop config` 命令，获取所需配置：

- root_path
- proxy
- gh_scoop_proxy_backup
- gh_proxy

前两个参数是 scoop 使用的，后两个参数是本程序专用的。

以下检查，均只针对 `set` 和 `restore` 命令。无论如何，不修改 `scoop config` 文件。

### 5.1 root_path

scoop 的安装目录。必须存在且不为空字符串，且目录存在，否则报错退出。

### 5.2 proxy

scoop 使用的代理信息。可以不存在或为空。

### 5.3 gh_proxy

必须存在且不为空字符串，否则报错退出。

如果尾部没有 `/`，则加上。

### 5.4 gh_scoop_proxy_backup

- 如果 `proxy` 不存在或为空，本字段应亦不存在或为空。否则给出警告，但继续执行。
- 如果 `proxy` 存在且不空，本字段必须存在且不为空字符串。否则给出警告，但继续执行。

## 六、 技术细节

### 6.1 JSON

不要预定义完整 Manifest Struct。程序只关心 Manifest 中的 url 字段。

```text
manifest
├── url
└── architecture
      ├── 64bit
      │    └── url
      ├── 32bit
      │    └── url
      └── arm64
           └── url
```

注意 `url` 对应的值可以是地址字符串数组。

使用 `encoding/json/jsontext` 逐 token 流式处理，分两个阶段：

```go
// 只读定位：返回字节区间 + 替换文本的编辑清单，不改动任何内容
edits, err := locateManifestEdits(data, ghProxy)

// 在其它函数应用：仅替换命中的字符串字面量，其余字节原样保留
out := applyManifestEdits(data, edits)
```

因此处理后的 manifest 与原文件相比，只有命中的 url 行发生变化，
缩进、key 顺序、转义形式、文件末尾换行等全部保持原样。

### 6.2 彩色输出

- INFO：普通
- SUCCESS：绿色
- WARNING：黄色
- ERROR：红色

### 6.3 程序组织结构

核心功能与命令行交互分层，核心库不输出、不退出，将来可供图形化界面复用。

```text
scoop-gh-proxy/
│
├── go.mod / go.sum
├── README.md / AGENTS.md
├── main.go                 # 程序入口
├── build.bat               # 编译脚本
│
└── internal/
    ├── cli/                # 命令行交互层（彩色输出、明细表、退出码）
    │   ├── cli.go
    │   └── output.go
    └── scoop/              # 核心库（只返回数据与错误）
        ├── config.go       # scoop config 信息处理
        ├── status.go       # scoop status -l 信息处理
        ├── runner.go       # set 命令主流程
        ├── manifest.go     # manifest 文件处理
        ├── bucket.go       # manifest 文件备份及还原处理
        └── tools.go
```
