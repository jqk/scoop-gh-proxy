# 工作原理

工具的所有命令都围绕同一套核心机制：**manifest 加前缀 + proxy 保护罩 + 备份还原**。手动三步（`set` → `scoop update` → `restore`）与自动命令（`update` / `install`）只是这套机制的不同编排方式。

## manifest 的修改与还原

### 定位

1. 从 `apps\<应用>\current\install.json`（或 `scoop-install.json`）读取该应用来自哪个 bucket
2. 在 `buckets\<bucket>\` 下查找 manifest：优先 `<bucket>\bucket\<应用>.json`（支持多级子目录），备选 `<bucket>\<应用>.json`，文件名大小写不敏感

### 分析

manifest 是 JSON 文件，工具只关心其中的下载 URL：

```text
manifest
├── url                        ← 顶层 url
└── architecture
      ├── 64bit  └─ url        ← 各架构的 url
      ├── 32bit  └─ url
      └── arm64  └─ url
```

- URL 值可以是字符串或字符串数组，统一处理
- 只修改 `https://github.com` 开头的 URL：在前面加上 gh_proxy 地址，形如 `https://代理/https://github.com/...`
- 所有候选位置都已是「gh_proxy + 原链接」形式时，状态记为 `Proxy set`，不再重复修改
- `checkver`、`autoupdate` 等其它成员不处理

### 备份与还原

- 修改前：把 manifest **改名**为 `<应用>-gh-backup.json`，再写入加前缀后的内容。改名即备份——原始内容一个字节都不会丢
- 还原时：删除修改版，把备份改名回原文件名
- 备份已存在时跳过（状态 `Manifest backup exists`），不覆盖已有备份——这是 set / update 重复执行安全的基础

### 字节级保留

manifest 不预定义 struct，而是用 `encoding/json/jsontext`（Go json v2）逐 token 流式扫描，分两个阶段：

1. **只读定位**：返回编辑清单（每项 = 字节区间 + 替换文本），不改动任何内容
2. **应用**：仅替换命中的 URL 字符串字面量（含引号），其余字节原样复制

因此修改后的 manifest 与原文件相比，只有命中的 URL 行变化，缩进、key 顺序、转义形式、文件末尾换行全部保持原样。

## proxy 保护罩

### 为什么要清空 proxy

scoop 的全局 `proxy` 作用于所有下载，而 gh_proxy 前缀只应作用于 GitHub 链接；两者叠加会冲突。所以下载 GitHub 应用期间需要临时清空 `proxy`，结束后恢复。

### 机制

- **清空**（`ClearScoopProxy`）：`proxy` 非空时，先把它的值备份到 `gh_scoop_proxy_backup`，再 `scoop config rm proxy`
- **恢复**（`RestoreScoopProxy`）：仅当 `gh_scoop_proxy_backup` 非空且 `proxy` 为空时恢复。幂等；程序运行期间用户手动设置的其它非空 proxy 不会被覆盖
- 使用范围：
  - `update`：Proxied 组整体一个保护罩——组开始清空，组结束恢复
  - `install`：`Is github` 应用安装期间清空，结束后恢复
  - `set` / `restore`：set 结尾清空、restore 结尾恢复（经典三步工作流对应）

### manifest 保护罩

`update` 的 Proxied 组与 `install` 的 `Is github` 应用共用同一个逐应用执行引擎：

1. 把 gh_proxy 前缀写入该应用的 manifest（写入前先备份）
2. 流式执行 `scoop update / install`
3. **无论成败都还原 manifest**——不会留下半修改状态
4. 还原失败优先展示为 `Restore failed`（manifest 仍处于已修改状态，可再执行 `restore` 补救）

### 中断自愈

update / install 期间异常中断（断电、强杀）可能遗留备份文件与被清空的 proxy。下次执行 `update` 开工前会自动扫描并还原这些遗留（复用 restore 主流程），保证每次都从干净状态开始；也可以随时手动执行 `restore` 清理。

## 成败判定

**只看成功标志，不看退出码。** scoop 出错时退出码经常为 0，且存在静默失败（如架构不支持时不输出 ERROR 也返回 0），退出码不可靠。

- 成功标志（大小写不敏感子串，出现在流程末尾，是最终定论）：
  - `was installed successfully`（安装/更新完成）
  - `Latest versions for all apps are installed`（已是最新）
- 命中任一 → `Updated` / `Installed`；未见成功标志一律判失败 → `Update failed` / `Install failed`
- 失败标志（`download failed`、`hash check failed`、`installation aborted`、`is not valid`、`couldn't find manifest`、`doesn't support current architecture`）**不参与成败判定**，只用于捕获最后一行失败原因，在结果行后提示

## 流式输出与下载进度

scoop 子进程的输出实时透传显示，观感与直接执行 scoop 一致：

- 终端下字节原样透传，scoop 进度行自带的 `\r` 原地覆盖刷新
- 非终端（重定向到文件、GUI、mintty / VSCode 等管道型终端）下行归一 + 重绘行单行刷新：整段下载只占一行，重定向到文件时以最后状态为准

下载进度显示分两种来源（取决于 `aria2-enabled`）：

- **aria2 启用**：进度由 scoop 自带输出，透传即得
- **aria2 关闭（默认）**：工具轮询 scoop 缓存目录（`<root_path>\cache`）中正在增长的 `*.download` 文件自绘进度条；只跟踪本次目标 URL 对应的文件，其它 scoop 进程的并发下载不会串台；下载成功后文件被改名挪走，工具据此补画 100% 帧（下载中断不虚报 100%）

## 进程安全

- **Windows Job Object**：scoop 子进程绑定到 Job Object（KILL_ON_JOB_CLOSE），本程序退出（含异常、被强杀）时整个子进程树一并终止，不留孤儿 powershell 进程
- **无超时**：安装包大小与下载速度不可推测，固定超时会误杀正在进行的下载，因此不设超时；子进程真挂死时需手动 Ctrl+C 终止（Job Object 负责收尾）

## 分层与显示分离

核心库（`internal/scoop`）只产生结构化数据与 Progress 事件，**不生成任何展示文本**；所有渲染（彩色输出、表格、进度条）在命令行层（`internal/cli`）完成。终端检测决定是否启用颜色；将来 GUI 化时传入自己的 writer 即可复用核心库。
