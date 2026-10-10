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

## 七、 进一步自动化

### 7.1 基本思路

1. 现在已经实现了 `--set` `--restore` `--status` 三个基本功能。在此 3 个功能的基础上，需要进一步自动化 scoop 软件更新
2. 理想化的执行步骤：

   - 执行 `scoop update` 命令，更新本地桶
   - scoop status -l，这个可以用已经实现的 --status 功能
   - 拿到 apps 后，将其分为三组：
     - `skipped` 或有错误的，这些不处理，直接跳过，当然要显示一下
     - `NotGithub`：这些直接逐条执行 `scoop update <app_name>`
     - `IsGithub`：
       - 先备份 `scoop config` 的 `proxy`，然后清除之。这个逻辑在当前代码已经实现了，使用即可。
         - 逐条执行：为该软件的设置 gh_proxy，`scoop update <app_name>`，恢复该软件的 manifest。
         - 这个在当前软件中逻辑可复用。
       - 恢复 `scoop config` 的 `proxy`，这个逻辑在当前代码已经实现了，使用即可。
3. `scoop update` 会更新 scoop 自身以及各个 bucket。执行结果基本如下：

   - `Scoop was updated successfully!`，说明更新都成功了。每一步完成后接着执行下一步。
   - 网络有问题，例如下载失败。出现这样的问题就停止并显示信息。
   - git 状态有问题，例如有某些本地文件改动了，与远程不同步。当前的任务跳过，继续执行下一任务。

### 7.2 执行计划（新增 `--update` 命令）

在 7.1 基本思路的基础上，确定的执行方案如下。

#### 7.2.1 命令入口

```bash
scoop-gh-proxy --update
```

#### 7.2.2 总体流程

不执行 `scoop update`（更新 scoop 自身与所有 bucket），也不执行 `scoop update *`；只根据 `scoop status -l` 的结果分组后，逐个执行 `scoop update <app_name>`。

```text
获取并校验 scoop config，失败则退出（退出码 1）
        ↓
清理遗留状态：
扫描遗留的 [app]-gh-backup.json 备份与被备份清空的 proxy，
自动还原并输出警告明细，随后重新获取 scoop config 刷新快照，
保证 --update 总是从干净状态开始
        ↓
执行 scoop status -l（不透传原始输出，按本工具的明细表格式显示），
将 outdated apps 分为三组：
  - Skipped / Manifest not found / Manifest error 等：显示明细，跳过
  - Not github：逐个执行 scoop update <app_name>
  - Is github / Proxy set：进入 proxy 保护罩阶段
        ↓
Not github 组：逐个流式执行 scoop update <app_name>（透传显示），
单个失败只记录状态，继续下一个，直到全部执行完
        ↓
Is github 组（proxy 保护罩）：
  1. setScoopProxy：备份 proxy 到 gh_scoop_proxy_backup 并清空
  2. 逐个 app：
     - Is github：修改该 app 的 manifest（GitHub URL 加 gh_proxy 前缀）
     - 流式执行 scoop update <app_name>（透传显示）
     - 还原该 app 的 manifest（无论成败）
     - Proxy set：URL 已带前缀、无备份，直接更新，保持原状并警告
     - 失败（未捕获成功标志）：记录 Failed，不中断，继续下一个
  3. 组结束：restoreScoopProxy 恢复 proxy（任何返回路径都保证先恢复）
        ↓
输出汇总：成功 / 失败 / 跳过计数 + 明细表
```

#### 7.2.3 流式执行与实时扫描（scoop update <app_name>）

`scoop update <app_name>` 采用「流式透传 + 实时逐行扫描」的执行模式，替代现有 runScoop 的「全量捕获后处理」：

- 透传：子进程 stdout/stderr 原样实时打印（保留颜色与进度条动画），stdin 接管，用户观感等同直接执行 scoop 命令；工具自身只在前后加少量说明行
- 扫描：读取侧按行缓冲（\n 切分，兼容 \r），每行去 ANSI 后交给 `classifyUpdateLine` 纯函数判定。错误标记集中在一处定义（大小写不敏感子串匹配），扫描命中即判定该 app 失败：`unable to access`、`could not resolve host`、`failed to connect`、`download failed`、`timed out`、`would be overwritten by merge`、`your local changes`、`not a git repository`、`detected dubious ownership`
- 进程异常退出（非零退出码）同样判定该 app 失败；未命中错误标记且正常退出才算更新成功
- 检测到错误行时不提前杀进程：让 scoop 自己的收尾/重试逻辑走完，结束后统一判定该 app 成败
- 无超时：安装包大小与下载速度不可推测，固定超时会误杀正在进行的下载。代价是子进程真挂死时需手动终止；本程序退出（含异常）时由 Job Object 保证子进程树不残留
- 进程树终止：scoop.cmd 会派生 powershell 子进程，普通 Kill 会留下孤儿进程；用 Job Object（KILL_ON_JOB_CLOSE）绑定子进程，本程序退出时整树结束

`scoop status -l` 保持现有模式：runScoop 捕获输出、解析后按本工具的明细表显示，不做透传。

#### 7.2.4 错误处理策略

- `scoop update <app_name>` 有错误 → 不中断整体流程：还原该 app 的 manifest（Is github 组）、记录 Failed、继续下一个 app_name，直到都执行完
- 所有失败在最终汇总中统一显示
- Proxy set 状态的 app（URL 已带前缀、无备份）：直接更新，保持原状并警告

#### 7.2.5 代码改动

| 文件 | 改动 |
| ---- | ---- |
| `internal/scoop/tools.go` | 新增 runScoopStream：流式透传 + 逐行扫描。现有 runScoop 保持不动（scoop config / status -l 仍用） |
| `internal/scoop/exec_kill.go`（新增） | Job Object 进程树终止（golang.org/x/sys/windows 转为直接依赖） |
| `internal/scoop/update_test.go`（新增） | classifyUpdateLine 表驱动单测 |
| `internal/scoop/update.go`（新增） | PrepareUpdate：遗留清理 + status 分组（UpdatePlan）；UpdatePlainApp / UpdateProxiedApp：逐个更新；BeginProxyPhase / EndProxyPhase：proxy 保护罩；classifyUpdateLine 纯函数 + 错误标记表 |
| `internal/scoop/runner.go` | 抽取「定位 bucket + 分析 manifest」门控为 prepareAppManifest，批量 set 与 --update 共用；批量行为不变 |
| 复用不改签名 | setProxiedManifest / restoreProxiedManifest / setScoopProxy / restoreScoopProxy / getOutdatedApps / findProxiedManifests |
| `internal/cli/update.go`（新增） | RunUpdate：os.Stdout 作为 progress writer 传入核心层；输出遗留还原警告、分组明细、逐 app 进度行、最终汇总；错误输出 + 退出码。printRestoreTable 抽取自 cli.go 供遗留警告复用 |
| `main.go` | --update 分发；更新 printUsage |
| `README.md` 第七节、`AGENTS.md` | 实现完成后同步最终设计 |

分层约定不破坏：核心层不直接打印——透传目标由注入的 `io.Writer` 决定（CLI 传 os.Stdout，将来 GUI 传自己的 writer）；核心层不 os.Exit，退出码由 cli 层决定。

#### 7.2.6 退出码

- 0：流程正常走完（含个别 app 更新失败——失败只体现在汇总与明细表中，沿用「单 app 失败不影响退出码」的现有约定）
- 1：配置错误、`scoop status -l` 无法执行或解析失败

#### 7.2.7 测试与验证

- `classifyUpdateLine` 表驱动单测：命中与不命中错误标记的样例（取自真实 scoop 输出）
- `go build` / `go vet` / `go test` 全绿
- 手动验证：
  - 正常全流程
  - 单个 app 更新失败，验证「还原 manifest → 继续下一个 → 直到全部执行完 → 最终汇总」
  - 重复执行验证幂等
