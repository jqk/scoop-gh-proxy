# 架构

## 分层

依赖方向单向：`main → internal/cli → internal/scoop`，反向禁止。

| 层 | 职责 | 约束 |
| ---- | ---- | ---- |
| `main.go` | 子命令分发，按 `cli.Run*` 返回的退出码统一 `os.Exit` | 全程序仅此一处退出调用 |
| `internal/cli` | 命令行交互层：彩色输出、明细表、下载进度渲染 | `Run*` 入口返回退出码；proxy 保护罩的调用编排在此层 |
| `internal/scoop` | 核心库：只接收参数、返回数据与 error | 不打印、不 `os.Exit`、不生成展示文本；进度经 `Progress` 结构化事件回调，渲染由调用方负责 |

分层的目的：核心功能与命令行交互解耦，将来 GUI 与 `internal/cli` 平级，共用核心库（update / install 的流式输出由注入的 `io.Writer` 决定，CLI 传包装 `os.Stdout` 的进度渲染器，GUI 可传自己的 writer）。

## 文件职责

```text
scoop-gh-proxy/
├── main.go                       # 入口：子命令分发（set/restore/status/update/install/list/help/version）
├── build.bat                     # 双架构构建，git tag 推导版本并 -ldflags 注入
├── internal/cli/
│   ├── cli.go                    # 全部 Run* 命令入口；update/install 的 proxy 保护罩编排
│   ├── printer.go                # 程序信息头、用法文本、汇总与明细表
│   ├── output.go                 # 彩色输出分级（info/success/warning/errorf/caution/fail）、IsTTY
│   └── progress.go               # 下载进度渲染（消费核心层 Progress 事件）
└── internal/scoop/
    ├── managed_app.go            # ManagedApp 与状态枚举（所有命令共用）
    ├── config.go                 # scoop config 读取校验；proxy 备份/清空/恢复
    ├── status.go                 # 执行并解析 scoop status -l
    ├── set.go                    # set 主流程
    ├── restore.go                # restore 主流程（update 遗留清理复用）
    ├── update.go                 # update 核心：裸 update 的 scoop update 步骤、准备与分组、逐组逐个更新
    ├── install.go                # install 的定位分析与安装入口
    ├── list.go                   # list 核心：scoop list 全量捕获、表格解析、按更新时间升序
    ├── proxied_cmd.go            # update/install 共用的逐 app 执行引擎（manifest 保护罩、流式执行）
    ├── markers.go                # scoop 成功/失败输出标志表（成败判定与失败原因捕获）
    ├── manifest.go               # manifest 只读定位与编辑应用（jsontext）
    ├── manifest_scanner.go       # manifest 流式扫描状态机
    ├── backup.go                 # manifest 备份落盘/还原/只读扫描
    ├── bucket.go                 # install.json 与 manifest 的纯查找
    ├── run_scoop.go              # scoop 命令执行：全量捕获 / 流式透传 + 逐行扫描
    ├── download_progress.go      # aria2 关闭时的下载进度轮询（结构化事件）
    ├── job_object.go             # Windows Job Object，防孤儿进程
    └── tools.go                  # stripAnsi、fileExists 等工具函数
```

各文件均有对应单测：`manifest_test.go`、`download_progress_test.go`、`update_test.go`、`install_test.go`、`list_test.go`、`run_scoop_test.go`（cli 层：`printer_test.go`、`cli_test.go`）。

## manifest 流式编辑

manifest 不预定义 struct，用 `encoding/json/jsontext`（json v2 系列）逐 token 流式处理，分两个阶段：

```go
// 只读定位：返回字节区间 + 替换文本的编辑清单，不改动任何内容
edits, err := locateManifestEdits(data, ghProxy)

// 在别处应用：仅替换命中的字符串字面量，其余字节原样保留
out := applyManifestEdits(data, edits)
```

- `locateManifestEdits` 只看顶层 `url` 与 `architecture.<架构>.url` 两个成员，其它成员整段跳过；同时报告是否存在已带前缀的 URL（`Proxy set` 判定）
- `applyManifestEdits` 按字节区间替换，未被修改的字节逐字节保留
- install.json 用 struct + `encoding/json/v2` 解析（与 manifest 的 jsontext 同为 v2 系列）

## 关键设计不变量

重构或 GUI 化时不得破坏：

- **幂等性**：set 重复执行安全（`Proxy set` / `Manifest backup exists` 跳过，不重复修改、不覆盖已有备份）；restore 重复执行安全（无备份无操作，proxy 已恢复时跳过）
- **proxy 条件对称**：清空仅在 `proxy` 非空时执行（备份到 `gh_scoop_proxy_backup`）；恢复仅在备份非空且 `proxy` 为空时执行——用户手动设置的其它 proxy 不被覆盖
- **未修改字节逐字保留**：`applyManifestEdits` 只做命中区间替换
- **单应用失败不中断**：update 逐个执行，失败只记录状态，直到全部执行完
- **成败只看成功标志**：成功标志（`was installed successfully` / `Latest versions for all apps are installed`）由 scoop 在流程末尾打印，是最终定论；失败标志只用于提示原因，不影响判定
- **保护罩边界**：update / install 中所有提前 return 都发生在 proxy 清空之前，无需 defer；`RestoreScoopProxy` 幂等；异常路径的遗留状态由下次 update 开工前的遗留清理自愈
- **install 显式桶请求**：以 `bucket/app` 形式执行 `scoop install`，保证安装的正是被定位并修改过的那份 manifest
- **点名静默去重**：update 点名不区分大小写、去重无输出；未点中的 app 仅提示不进更新流程

## 输出分级与流向

| 函数 | 颜色 | 流向 | 用途 |
| ---- | ---- | ---- | ---- |
| `info` | 无 | stdout | 普通信息 |
| `success` | 绿 | stdout | 成功结果 |
| `caution` | 黄 | stdout | update 逐应用提示（与流式输出同流保序） |
| `fail` | 红 | stdout | 失败结果 / 失败原因（与流式输出同流保序） |
| `warning` | 黄 | stderr | 独立警告（遗留清理、点名未匹配等） |
| `errorf` | 红 | stderr | 独立错误（配置错误等，不与流式输出交错） |

`caution` / `fail` 特意走 stdout：update 的逐应用提示行与 scoop 流式输出同写 stdout，保证重定向合并 stderr 时先后顺序不乱。

颜色经 `fatih/color` 输出，非终端（`IsTTY` 判定）自动去色。
