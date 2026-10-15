# AGENTS.md

## 项目概述
Windows Go CLI 工具，用于修改 scoop bucket 中 `[app].json` 的 GitHub URL（加 gh_proxy 前缀），并支持还原。

## 构建
```
build.bat             # 输出 dist\amd64\ 与 dist\arm64\ 下的 scoop-gh.exe（各含 README.md），注入版本号与提交日期
build.bat --version   # 仅预览目标版本号，不构建
go build -o dist\scoop-gh.exe .   # 直接构建（不经 build.bat，单架构跟随宿主机）：version 为 dev、无提交日期；输出到 dist，勿在项目根目录留 exe
```

版本号规则：从 Git tag 计算——最近 tag 的 patch 号 + 其后提交次数（如 tag v0.1.0 之后 5 个提交 → 0.1.5；无 tag 时以 0.0.0 为基数、累计全部提交数）。build.bat 通过 `-ldflags -X` 注入 `internal/cli` 的 version 与 commitDate（版本对应代码即 HEAD 的提交日期，YYYY-MM-DD，`git log -1 --format=%cs`）

GitHub 自动发布（.github/workflows/release.yml）：推送 `v*` tag 触发（ubuntu runner 交叉编译 windows amd64/arm64），步骤内正则校验 tag 必须为 `v1.0.0` 格式否则失败；版本取自 tag（去 v 前缀）、日期取 HEAD（即 tag 指向的提交）的提交日期，经同样的 `-ldflags -X` 注入；发布产物为 `scoop-gh-<版本>-<架构>.zip`（内含 scoop-gh.exe 与 README.md），经 softprops/action-gh-release@v2 发布

## 分层
- `internal/scoop` — 核心库：只接收参数、返回数据与 error，不打印、不 os.Exit、不生成展示文本（进度经 Progress 结构化事件回调，渲染由调用方负责）。将来 GUI 与 internal/cli 平级，共用核心
- `internal/cli` — 命令行交互层：彩色输出、明细表在这里；Run* 命令入口返回退出码、不直接 os.Exit，由 main 统一退出
- 依赖方向单向：main → internal/cli → internal/scoop，反向禁止

## 文件结构
- `main.go` — 入口，子命令分发（set / restore / status / update [app...] / install <bucket/app> / help / version，不带 -- 前缀），按 cli.Run* 返回的退出码统一 os.Exit
- `internal/cli/cli.go` — 全部 Run* 命令入口（RunSet / RunRestore / RunStatus / RunUpdate / RunInstall / RunHelp / RunVersion，RunSet/RunRestore 无参数——dryRun 只在内部供 status 复用），返回退出码；输出以程序信息头开始（printBanner）、空行结尾（defer fmt.Println）；update 与 install 的 proxy 保护罩在此层（PrepareUpdate / LocateInstallApp 成功后进入更新或安装，结束处显式调用 RestoreScoopProxy）；RunUpdate 接收 update 之后的参数（requestedApps 归一化：任一参数为 * 即全量更新，既有 * 又点名时返回 starMixed 交调用方提示），更新分两组先后执行——Proxied 组（Is github，proxy 保护罩内）在前、Not github 组（不处理 manifest 与 proxy）在后，Proxy set 状态跳过（归入 Skipped 组，本次不处理），两组 [n/m] 连续编号、均非空时中间以横线分隔（printGroupSeparator），逐 app 更新块抽为 runUpdateApp 共用；RunInstall 经 parseInstallArg 解析 <bucket/app>（bucket 可省略）
- `internal/cli/printer.go` — 程序信息头与表格：version / commitDate（构建时 -ldflags -X 注入，-X 按包路径定位、与所在文件无关）、bannerText / printBanner（版权行 + 名称版本行 + 空行；日期为注入的提交日期，直接构建时省略、版权年份取当前年）、printUsage、汇总与明细表（printSetSummary / printRestoreSummary / printSetTable / printRestoreTable / printAppOutcome / printUpdateSummary（覆盖 Proxied 与 Not github 两组，顺序与执行一致）/ printGroupSeparator（update 两组操作间的分隔横线，横线前一空行自带、后一空行由下一组首个 app 块的空行承担）/ printTable / pickColor——与 scoop 状态常量编译期绑定）
- `internal/cli/progress.go` — 下载进度渲染（progressPrinter / renderDownloadLine / humanSize，消费 scoop.Progress 事件）。progressPrinter 兼作流式输出的透传 writer（构造时传 os.Stdout，update / install 每个 app 新建一个），进度刷新与流式透传经 mu 串行：进度行始终 \r 原地重绘一行（终端与管道型终端渲染一致，与 scoop aria2 进度同观感；非终端内容未变不重写），新行更短补空格覆盖残留，Write（后续 scoop 输出）与 finishLine（流程结束，cli 显式调用）先补 \n 终结进度行（与核心层 lineFilter 同语义双实现，分层所需）
- `internal/cli/output.go` — 彩色输出（info/success/warning/errorf/caution/fail）、IsTTY（委托 scoop.IsConsole，全程序同一终端判定）；caution/fail 走 stdout
- `internal/scoop/managed_app.go` — ManagedApp 与 ManagedAppStatus 状态枚举定义（set / restore / status / update / install 共用）
- `internal/scoop/config.go` — 解析并校验 scoop config；设置与恢复 proxy（ClearScoopProxy / RestoreScoopProxy，成功后同步更新 ScoopConfig 内存值）；读取 aria2-enabled（Aria2Enabled，控制 update 的下载进度来源：aria2 用自带 \r 进度流，默认下载器用 downloadProgress 自绘）
- `internal/scoop/status.go` — 执行并解析 scoop status -l（fetchOutdatedApps / findColumnStarts / parseStatusApp，纯解析，不做文件查找）
- `internal/scoop/set.go` — set 命令主流程（SetProxyForOutdatedApps）
- `internal/scoop/restore.go` — restore 命令主流程（RestoreOutdatedAppManifests）
- `internal/scoop/markers.go` — scoop 输出的成败标志：scoopSuccessMarkers 成功标志表 + matchScoopSuccess（update 与 install 共用，"was installed successfully" 出自 install.ps1）+ scoopFailureMarkers 失败标志表 + matchScoopFailure（仅用于捕获失败原因行，不影响判定），均取自 scoop 源码与真实输出
- `internal/scoop/update.go` — update 的核心：PrepareUpdate 准备与分组（遗留清理复用 RestoreOutdatedAppManifests；requested 点名过滤在 manifest 分析之前，pickRequestedApps 大小写不敏感匹配 + 静默去重，未匹配进 plan.NoUpdate；分组前 sortAppsByName 按名称字母升序排列（忽略大小写，字母序相同按原名称字节序定序），明细表与两组执行顺序一致）、UpdateProxiedApp 逐个更新 Proxied 组 = Is github（参数：透传 writer + Progress 进度回调）、UpdatePlainApp 逐个更新 Not github 组（不处理 manifest 与 proxy，直接执行）
- `internal/scoop/proxied_cmd.go` — update 与 install 共用的逐 app 执行引擎：runResult（成功标志 + 失败原因行记录到 app.Failure）、runScoopForApp（流式执行样板：启动进度采样、进度轮询启用时以 completingWriter 包装透传 writer（下载收尾补报 100%）、逐行扫描标志）、proxiedAppCmd（manifest 保护罩：前缀写入 → 执行 scoop <cmd> <spec> → 无论成败还原；成功/失败结果状态由调用方以参数传入，update 传 Updated/Update failed、install 传 Installed/Install failed）、runPlainApp（无 manifest/proxy 处理的直接执行：update 与 install 的 Not github 共用）
- `internal/scoop/install.go` — install 的核心：LocateInstallApp 定位与分析 bucket manifest（显式桶先校验 buckets\<bucket> 目录存在；省略桶时逐桶查找取第一个命中并回填 Bucket；已安装预检用 findBucket——scoop 对已安装 app 的 install 只警告且无成功标志；analyzeManifest 复用，分析错误记录在 Status）、InstallApp 保护罩内安装（scoop install <bucket>/<app>，仅 IsGitHub）、InstallPlainApp 直接安装 Not github（不处理 manifest 与 proxy，仍用 installSpec 显式桶请求）、installSpec（显式桶请求，保证安装的就是被修改的那份 manifest）
- `internal/scoop/download_progress.go` — downloadProgress：aria2 关闭时轮询 cache 的 *.download 文件产生 Progress 结构化事件（轮询目标与总量均从 "Downloading <url> (<size>)..." 行经 feed 解析：URL 按 scoop cache_path 规则（core.ps1，sha256 小写十六进制前 7 位）算出哈希片段、只跟踪名字匹配的 *.download——cache 目录全局共享，其它 scoop 进程的并发下载（同样在增长、mtime 更新）不得串台进画面；未见 Downloading 行（目标为空）时不跟踪；总量无法解析则记 0 仅按已下载量上报；渲染由调用方负责，核心层不生成展示文本；方法 nil-safe，aria2 启用或无回调时传 nil）。complete 收尾补报：曾观察到增长的 .download（active 字段，陈旧半成品永不增长、顶替进 tick 跟踪也不影响收尾对象）消失——scoop 下载成功后将其改名挪走，失败则保留——即补发一次满额事件（100% 那帧，下载中断不虚报），由 completingWriter 在流式输出写入前触发：纯轮询会因 900ms 限频与改名到下一行输出间不足一个轮询周期而漏掉，进度条停在 9x% 就被下一行顶掉。completingWriter：显示目标的 writer 包装，Write 前调 complete 再透传（100% 先于触发它的下一行输出画上），WriterIsConsole 转发内层 consoleAware 判定，仅 runScoopForApp 在进度轮询启用时包装
- `internal/scoop/job_object.go` — Windows Job Object（x/sys/windows）：KILL_ON_JOB_CLOSE 保证本程序退出（含异常）时子进程树一并终止，防孤儿
- `internal/scoop/manifest.go` — manifest 的只读定位与应用（prepareAppManifest 定位分析门控（set 与 update 共用）/ analyzeManifest / locateManifestEdits / applyManifestEdits，jsontext 流式；扫描器在 manifest_scanner.go）
- `internal/scoop/manifest_scanner.go` — manifestScanner：jsontext 流式扫描 manifest 的状态与方法（scanArchitecture / scanArchBlock / scanURLValue / scanURLElement / readMemberName）
- `internal/scoop/run_scoop.go` — scoop 命令执行：runScoop（全量捕获，无超时）、runScoopStream（流式透传 + 逐行扫描，无超时，observe 回调供调用方捕获行；命中成功标志（matchScoopSuccess，定义在 markers.go）置位，错误行不收集、随透传直接显示；writerIsConsole 判定显示目标——实现 consoleAware 可选接口（WriterIsConsole）的包装器以其声明为准，IsConsole 为导出形式供 cli 复用同一判定——终端与非终端两种显示模式）、streamPipe 与 lineFilter（显示侧两种模式：终端——CLI 传 os.Stdout 的常规场景——字节原样透传，scoop 进度行自带的 \r 前缀由终端原地覆盖刷新、与 scoop 直跑观感一致，流结尾停在半行时补换行防本程序后续输出拼接；非终端——重定向到文件、GUI 注入的 writer、不被 GetConsoleMode 识别为控制台的管道型终端（mintty / VSCode 终端等）——行归一（\r+ 及其后至多一个 \n 为一个行结束、空行与纯空白行丢弃、行尾空白裁剪、行首缩进保留），且重绘行单行刷新：\r 后跟新内容视为同一显示行的状态更新（aria2 进度行经 scoop Write-Host "`r..." -NoNewLine 输出），内容变化时以 \r 重写该行（更短补空格覆盖残留）、内容相同跳过；该行保持未落定，被换行终结的行覆写落定（行首 \r 落在未换行的重绘行上，与终端一致）或流结束时补 \n——整段下载只占一行，管道型终端据此同样单行刷新，重定向到文件时一行以最后状态为准）
- `internal/scoop/manifest_test.go` — 定位/应用的单测
- `internal/scoop/download_progress_test.go` — feed 解析 / tick 事件 / complete 收尾补报（含中断不虚报、陈旧文件顶替）/ completingWriter 写入顺序与终端判定转发 / nil-safe 的单测
- `internal/scoop/update_test.go` — matchScoopSuccess / matchScoopFailure / sortAppsByName / pickRequestedApps 的单测
- `internal/scoop/install_test.go` — LocateInstallApp（t.TempDir 构造桶）/ installSpec 的单测
- `internal/scoop/run_scoop_test.go` — streamPipe / lineFilter 的单测（行归一、重绘去重、终端透传、onLine 行切分；byteReader 逐字节喂入制造最恶劣块边界）
- `internal/cli/printer_test.go` — bannerText / printTable / renderDownloadLine / humanSize / progressPrinter（终端重绘与终结、非终端逐行去重）的单测
- `internal/cli/cli_test.go` — requestedApps / parseInstallArg 的单测
- `internal/scoop/backup.go` — manifest 备份的落盘与还原：setProxiedManifest（备份 + 应用编辑写回）/ restoreProxiedManifest / findProxiedManifests（只读扫描所有待还原的备份）/ createManifestBackupName / backupSuffix
- `internal/scoop/bucket.go` — 纯查找：install.json 与 manifest 查找（findBucket / findManifest）、fillBucketManifest 定位补全
- `internal/scoop/tools.go` — stripAnsi、fileExists

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

## 结果状态值（ManagedAppStatus）
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
- `Updated` — update：捕获到成功标志（"was installed successfully" 或 "Latest versions for all apps are installed"）
- `Update failed` — update：未捕获到成功标志（scoop 出错时退出码常为 0，不能靠退出码兜底；错误行随透传直接显示）
- `Install failed` — install：未捕获到成功标志（失败时 scoop 输出 ERROR 行并经 abort 以退出码 1 结束，或静默失败退出码 0，如架构不支持——成败只看成功标志；失败原因行在结果行后提示）
- `Installed` — install：捕获到成功标志

## 设计不变量（幂等性，重构或 GUI 化时不得破坏）
- set 重复执行安全：已处理的 manifest 跳过（Proxy set / Manifest backup exists），不重复修改、不覆盖已有备份
- restore 重复执行安全：无备份的 app 无操作；proxy 已恢复（Proxy == GhScoopProxyBackup）时跳过
- proxy 设置与恢复条件对称：set 仅在 Proxy 非空时备份并清空；restore 仅在 GhScoopProxyBackup 非空且 Proxy 为空时恢复（本程序运行期间用户手动设置的其它非空 proxy 不被覆盖）
- manifest 中未被修改的字节逐字节保留：applyManifestEdits 只做命中区间的替换
- update 单个 app 失败不中断：失败只记录状态，继续下一个，直到全部执行完
- update / install 的成败判定以成功标志为准：成功标志（was installed successfully / Latest versions for all apps are installed）由 scoop 在流程末尾打印，是最终定论；未见成功标志一律判失败（proxiedAppCmd 按调用方传入的成功/失败状态常量记入 app.Status）。错误行不另行收集——随透传直接显示，成败只看成功标志；成功/失败标志表集中在 markers.go（scoopSuccessMarkers / scoopFailureMarkers），取自 scoop 源码（e:\scoop\apps\scoop）与真实输出。scoopFailureMarkers（download failed / hash check failed / installation aborted / is not valid / couldn't find manifest / doesn't support current architecture）命中的失败原因行记录在 app.Failure 供输出层提示，不影响判定
- update 的 proxy 保护罩：ClearScoopProxy 成功后进入更新循环，组结束处显式调用 RestoreScoopProxy 恢复；所有提前 return 都发生在 proxy 清空之前，无需 defer。RestoreScoopProxy 幂等（无备份或已恢复时为 no-op）；panic 等异常路径不恢复，遗留状态由下次运行开工前的遗留清理自愈。Not github 组在保护罩之外更新（RestoreScoopProxy 之后执行，不处理 manifest 与 proxy）
- update 的 Not github 组与 install 的 Not github 直接执行共用 runPlainApp：不写 manifest、不动 proxy，成败同样只看成功标志（结果状态与保护罩路径一致：Updated / Update failed、Installed / Install failed）
- update 的逐 app 提示行（[n/m] 表头、更新结果）与 scoop 流式输出同写 stdout（caution/fail），保证先后顺序不被 stderr 合并打乱；errorf/warning 走 stderr，仅用于不与流式输出交错的独立消息（配置错误、收尾恢复失败、点名未匹配提示等）
- update 开工前先清理遗留：自动还原上次运行遗留的备份与 proxy（复用 RestoreOutdatedAppManifests），保证从干净状态开始（恢复的 proxy 已同步到内存 cfg，无需二次 GetScoopConfig）
- update 的 Is github app 无论更新成败都还原 manifest；还原失败优先展示为 Failed（manifest 仍处于已修改状态，可再执行 restore）
- update <app...> 点名静默去重（大小写不敏感，保留首现顺序，无任何输出）；未点中的 app 仅提示（plan.NoUpdate）不进更新流程；点名过滤发生在 prepareAppManifest 分析之前，未点名的行不做 manifest 分析
- install 与 update 的 manifest 保护罩共用同一实现（proxiedAppCmd）：gh_proxy 前缀写入 → 执行 scoop install/update <spec> → 无论成败还原 manifest，还原失败优先展示为 RestoreFailed；install 的所有提前 return 都发生在 ClearScoopProxy 之前；install 以 installSpec（bucket/app 显式桶请求）保证安装的正是被修改的那份 manifest

## 命令行为
- `set` — 定位 + 备份 + 修改 [app].json，输出 "Manifest to Set: N" + 明细表
- `restore` — 还原 backup，输出 "Manifest to restore: N" + 明细表（Name + Bucket）
- `status` — dry-run，先输出 restore 明细，再输出 set 明细，不修改任何文件
- `update` / `update *` — 全量更新（两者完全等价，`*` 由 cli 层 requestedApps 归一化为空参数；点名中混有 * 时先提示「参数中有 * 号，忽略指定的 app 名称，按 update * 处理」再按全量执行）。不执行 scoop update（不更新 scoop 自身与桶）。流程：清理遗留 → status -l 解析后按名称字母升序排列（忽略大小写，点名与全量一致）→ 分组（Skipped / Proxy set 不更新——Proxy set 已带前缀且无备份、无法安全还原，本次跳过；更新 Is github 与 Not github 两组，两组各自保持排序后的顺序）→ 第一组 Proxied（Is github）：ClearScoopProxy 清空 proxy（备份到 gh_scoop_proxy_backup）→ 逐个 scoop update <app>（runScoopStream 流式透传，无超时：下载时长不可推测，挂死时需手动终止）→ RestoreScoopProxy 恢复 proxy → 第二组 Not github：不处理 manifest 与 proxy，逐个 scoop update <app> 直接更新（[n/m] 与第一组连续编号）→ 汇总（覆盖两组）。两组均非空时两组操作之间以一行横线分隔（横线前后各一空行；某组数量为零时不画横线）。单 app 失败不影响退出码；退出码 1 = 配置错误、status -l 无法执行/解析失败或 proxy 清空失败
- `update <app...>` — 点名更新（一个或多个）：先 status -l，再用 pickRequestedApps 把点名与 outdated 列表对比（不区分大小写、静默去重保留首现顺序、同名行只取第一行）。匹配到的行替换 plan.All，进入与全量 update 完全相同的分组与更新流程（Skipped / Proxy set 不更新，先更新 Proxied 组 = Is github，再更新 Not github 组）；未匹配的点名进 plan.NoUpdate，仅以 warning 提示（stderr：标题行带数量，明细逐行两位右对齐序号 `%2d.`，1-9 前补空格），不进明细表与汇总。退出码语义同上
- `install <bucket/app>` — 单 app 安装，bucket 可省略（省略时在 buckets 下逐桶查找，取第一个命中并输出桶名；显式指定时先校验 buckets\<bucket> 目录存在，不存在报错）。流程：定位 manifest（复用 findManifest，找不到报错）→ 已安装预检（报错退出）→ analyzeManifest 分析状态。Not github：不处理 manifest 与 proxy，直接流式执行 scoop install <bucket>/<app>（成败同样以成功标志判定）；Is github：保护罩内安装（ClearScoopProxy → gh_proxy 前缀写入 manifest → 流式执行 scoop install <bucket>/<app> → 无论成败还原 manifest → RestoreScoopProxy）；其它状态（ProxySet / ManifestError 等）报错退出。成败以成功标志为准（Installed / Install failed，还原失败优先展示 RestoreFailed）；退出码 0 = 安装成功（Installed），1 = 参数/配置错误、已安装、bucket 或 app 不存在、状态不允许、proxy 清空失败、安装未成功
- N 为 0 时不输出明细表
- dryRun 标志只存在于命令流程层（SetProxyForOutdatedApps / RestoreOutdatedAppManifests），manifest 层只做只读定位

## 共享逻辑
- manifest 的只读定位与 backup 的只读扫描均为纯查询，set、restore、status、update 四条命令共用同一实现；
  是否落盘仅由命令层决定（set/update 的 dryRun 或落盘步骤），核心层不打印、不退出、不调用 os.Exit
- prepareAppManifest（定位 bucket + 分析 manifest）为 SetProxyForOutdatedApps 与 PrepareUpdate 共用
- update / install 的流式输出由注入的 io.Writer 决定（CLI 传包装 os.Stdout 的 progressPrinter——兼渲染下载进度，实现 consoleAware 保留终端透传模式；将来 GUI 可传自己的 writer）；显示侧模式由 writerIsConsole 判定——终端原样透传（\r 原地重绘生效），非终端（重定向、GUI、管道型终端）行归一 + 重绘行单行刷新（内容变化时 \r 重写同一行、内容未变跳过）；下载进度行始终 \r 原地重绘一行（与 scoop aria2 进度同观感，非终端内容未变不重写）；成功/失败标志表集中在 markers.go；manifest 保护罩（proxiedAppCmd）与流式执行（runScoopForApp）为两者共用

## 编译验证
```
go build ./...
go vet ./...
go test ./...
```
