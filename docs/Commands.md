# 命令手册

程序名为 `scoop-gh.exe`，以下命令示例均写作 `scoop-gh`。

通用约定：

- 每条命令的输出以**程序信息头**开始（版权行 + 名称版本行），以一个空行结束
- scoop config 校验失败（`root_path` 或 `gh_proxy` 缺失，见[安装](Install.md)）时，所有命令报错退出码 1（`list` 不读取 config，无此校验）
- 下方示例中省略程序信息头；`（…）` 表示略去的 scoop 流式输出；实际运行时状态行带颜色（成功绿色、失败红色、Skipped 黄色）
- 非终端输出（重定向到文件、管道）自动去色，进度行以单行形式刷新

## set

为待更新应用的 manifest 加 gh_proxy 前缀。语法：

```text
scoop-gh set
```

执行流程：

1. 读取并校验 scoop config
2. 执行 `scoop status -l` 获取过期应用列表
3. 对每个应用：从 `apps\<应用>\current\install.json` 找到 bucket → 定位 manifest → 分析其中的 GitHub URL
4. 状态为 `Is github` 的应用：把 manifest 备份为 `<应用>-gh-backup.json`（备份已存在则跳过），写入加前缀后的内容
5. 有修改时：把 scoop 的 `proxy` 备份到 `gh_scoop_proxy_backup` 并清空，避免与 gh_proxy 前缀冲突

输出示例：

```text
Manifest to Set: 3

App Name  Installed Version  Latest Version  Bucket Name  Status
--------  -----------------  --------------  -----------  ----------
alma      0.4.150            0.4.151         ajqk         Not github
git       2.55.0.5           2.56.0          main         Is github
uv        0.12.19            0.12.20         extras       Is github
```

数量为 0 时只输出 `Manifest to Set: 0`，无明细表。

执行完 set 后即可运行 `scoop update <应用>` 下载，完成后执行 `restore` 还原。日常更新更推荐直接使用 [`update`](#update-app) 命令，无需手动三步。

退出码：0 = 正常走完（个别应用的备份/写入失败只记录在明细表）；1 = 配置错误。

## restore

还原 set 的全部修改。语法：

```text
scoop-gh restore
```

执行流程：

1. 读取并校验 scoop config
2. 扫描 buckets 下所有 `<应用>-gh-backup.json`，对每个备份：删除已修改的 manifest，把备份改名回原文件
3. `gh_scoop_proxy_backup` 非空且 `proxy` 为空时，把备份值恢复到 `proxy`

输出示例：

```text
Manifest to restore: 2

App Name  Bucket Name  Status
--------  -----------  ------
git       main         Success
uv        extras       Success
```

数量为 0 时只输出 `Manifest to restore: 0`，无明细表。

幂等性：重复执行安全——无备份的应用不出现；proxy 已恢复时跳过。本程序运行期间用户手动设置的其它非空 proxy 不会被覆盖。

退出码：0 = 正常走完（个别还原失败记录在明细表）；1 = 配置错误。

## status

预览将要 restore / set 的明细，**不修改任何文件**。语法：

```text
scoop-gh status
```

依次 dry-run 执行 restore 与 set：先输出 restore 明细（两列表格，无 Status 列），空行分隔后输出 set 明细。全新状态下的输出示例：

```text
Manifest to restore: 0

Manifest to Set: 2

App Name  Installed Version  Latest Version  Bucket Name  Status
--------  -----------------  --------------  -----------  ----------
git       2.55.0.5           2.56.0          main         Is github
uv        0.12.19            0.12.20         extras       Is github
```

已执行过 set 的状态下，set 部分会显示 `Proxy set`（URL 已带前缀），`Manifest to Set` 计数为 0。

退出码：0 = 正常走完；1 = 配置错误。

## update [app...]

更新过期软件，整个「加前缀 → 下载 → 还原」流程自动化。语法：

```text
scoop-gh update          # 先执行 scoop update 更新 scoop 与 buckets，再全量更新
scoop-gh update *        # 直接全量更新（不执行 scoop update）
scoop-gh update git uv   # 点名更新（一个或多个）
```

执行流程：

0. 仅 `update`（不带参数）执行本步：流式执行 `scoop update` 更新 scoop 自身与 buckets。失败不中断——个别 bucket 更新失败（如网络原因）只影响新版本的可见性，输出黄色警告后按当前 buckets 状态继续。`update *` 与点名更新跳过此步
1. **清理遗留**：自动还原上次运行遗留的备份与 proxy（异常中断自愈），有遗留时输出黄色警告与还原明细
2. 执行 `scoop status -l`，按名称字母升序排列（忽略大小写）后分析 manifest、分组：
   - `Is github` → **Proxied 组**（第一更新组）
   - `Not github` → **Not github 组**（第二更新组）
   - `Skipped`、`Proxy set`、`Manifest not found` 等 → 仅显示，不更新
3. Proxied 组：清空 scoop proxy（备份到 `gh_scoop_proxy_backup`）→ 逐个更新（每个应用写入 gh_proxy 前缀 → `scoop update <应用>` → 无论成败还原 manifest）→ 组结束恢复 proxy
4. 两组之间以横线分隔（某组为空时不画）
5. Not github 组：不处理 manifest 与 proxy，逐个直接更新；`[n/m]` 序号与第一组连续编号
6. 输出汇总

单应用失败不中断：失败只记录状态，继续下一个，直到全部执行完。

完整输出示例（scoop 流式输出以 `（…）` 略去）：

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

App Name  Installed Version  Latest Version  Bucket Name  Status
--------  -----------------  --------------  -----------  ----------
7zip      24.08              24.09           main         Updated
curl      8.10.1             8.11.0          main         Updated
```

### 点名规则

- 点名匹配**不区分大小写**；重复点名静默去重（`update git GIT git` 只更新一次），无任何输出
- 不在 outdated 列表中的点名（已最新或未安装）逐行提示（stderr），不进明细表与汇总：

  ```text
  以下 app 不在 outdated 列表中，没有可用的更新: 1
   1. curl
  ```

- 点名中混入 `*`（如 `update git uv * curl`）：提示「参数中有 * 号，忽略指定的 app 名称，按 update * 处理」，按全量执行；仅 `update *` 不提示

### 成败判定

以 scoop 输出中的**成功标志**为最终定论：`…was installed successfully` 或 `Latest versions for all apps are installed` → `Updated`；未见成功标志一律判 `Update failed`。不看 scoop 退出码（scoop 出错时退出码常为 0）。失败的错误行已在流式输出中直接显示。详见[工作原理](Principles.md)。

### 退出码

- 0 = 全部执行完毕（个别应用更新失败不影响退出码，失败体现在结果行与汇总中；`scoop update` 步骤失败同样不影响退出码，仅警告）
- 1 = 配置错误、`scoop status -l` 无法执行或解析失败、遗留清理失败、proxy 清空失败

## install &lt;bucket/app&gt;

安装指定应用。语法：

```text
scoop-gh install <bucket/app>   # bucket 可省略
scoop-gh install main/7zip      # 显式指定桶
scoop-gh install 7zip           # 省略桶名，在所有桶中逐个查找
```

执行流程：

1. 解析参数（`<bucket/app>`；省略桶时在 `buckets` 下逐桶查找 manifest，取第一个命中的桶并输出桶名；显式指定时先校验该桶目录存在）
2. 已安装预检：已安装的应用报错退出（scoop 对已安装应用只警告不安装，无法判定成败），如需更新请执行 `update <应用>`
3. 分析 manifest 的下载 URL，按状态处理：
   - `Not github`：不处理 manifest 与 proxy，直接流式执行 `scoop install <bucket>/<app>`
   - `Is github`：保护罩安装——备份并清空 proxy → gh_proxy 前缀写入 manifest → 流式安装 → 无论成败还原 manifest → 恢复 proxy
   - 其它状态（`Proxy set`、`Manifest error` 等）：报错退出

输出示例（安装成功，以省略桶名的 `scoop-gh install 7zip` 为例；`main/7zip: …` 行中的 app 参数始终是显式桶形式）：

```text
已定位到 bucket: main
main/7zip: Is github，安装期间临时应用 gh_proxy 前缀，结束后还原
（scoop install main/7zip 的流式输出与下载进度…）
7zip: Installed
```

安装失败时，失败原因行（scoop 输出中最后命中的失败标志）跟在结果行后：

```text
7zip: Install failed
失败原因: ERROR Download failed! ...
```

安装使用 `bucket/app` 显式桶请求，保证安装的正是本程序定位并修改过的那份 manifest。成败判定与 update 相同（只看成功标志，结果状态为 `Installed` / `Install failed`；manifest 还原失败优先展示 `Restore failed`）。

### 退出码

- 0 = 安装成功（`Installed`）
- 1 = 参数/配置错误、应用已安装、bucket 或应用不存在、状态不允许、proxy 清空失败、安装未成功或 manifest 还原失败

## list

执行 `scoop list`，按更新时间升序输出已安装应用。语法：

```text
scoop-gh list
```

与 update / install 的流式透传不同：全部输出收集完成后统一解析、排序、再输出。不读取 scoop config，不修改任何文件。

执行流程：

1. 执行 `scoop list`（全量捕获输出）
2. 解析表格（Name / Version / Source / Updated / Info），按 Updated 升序排列
3. 输出计数与明细表

Updated 是该应用最近一次安装/更新的时间（scoop 取 install.json 的最后写入时间）。升序排列后最久未更新的应用在最前——每天执行 update 的前提下，长期不动的行意味着上游已许久没有发布新版本，可考虑 `scoop uninstall` 移除。

输出示例：

```text
Installed apps: 3

App Name  Version  Bucket Name  Updated              Info
--------  -------  -----------  -------------------  ------------------
aria2     1.37.0-1 main         2024-06-23 22:01:29
cmirror   0.1.2    ajqk         2025-12-17 06:39:32  Deprecated package
7zip      26.04    main         2026-10-08 07:22:54
```

退出码：0 = 正常列出；1 = `scoop list` 执行或解析失败。

## help / version

```text
scoop-gh help       # 程序信息头 + 用法
scoop-gh version    # 程序信息头（版本在第二行）
```

用法文本：

```text
用法:
  scoop-gh set                   向待更新应用 manifest 中的 github 下载链接插入代理地址
  scoop-gh restore               还原 set 命令的修改
  scoop-gh status                预览将要 set/restore 的明细
  scoop-gh update [app...]       更新所有过期软件；不带参数时先更新 scoop 与 buckets
  scoop-gh install <bucket/app>  安装指定的应用, bucket 可省略
  scoop-gh list                  列出已安装应用, 按更新时间升序

  scoop-gh help                  显示帮助，详细信息参见 README.md
  scoop-gh version               显示版本
```

- 不带参数运行 = 显示 help，退出码 1
- 未知命令 = 红色提示「未知命令: …」+ help，退出码 1

## Status 取值

明细表 Status 列与结果行的取值（`ManagedAppStatus`）：

| 状态值 | 含义 | 出现场景 |
| ---- | ---- | ---- |
| `Is github` | 存在待修改的 GitHub URL | set / status / update 明细表 |
| `Not github` | 无待修改的 GitHub URL | 各明细表；update 第二组；install 直接安装 |
| `Proxy set` | URL 已带 gh_proxy 前缀，无需修改 | 重复 set；update 中跳过不更新 |
| `Skipped` | status 行信息不全（Info / Missing 非空、版本缺失等，如被 hold） | 各明细表，不处理 |
| `Manifest not found` | bucket 或 manifest 文件找不到 | 各明细表 |
| `Manifest error` | manifest 读取或解析失败 | 各明细表 |
| `Manifest backup exists` | 备份已存在，不重复处理 | 重复 set（幂等保护） |
| `Manifest backup failed` | 备份失败 | set 明细表 |
| `Success` | restore 还原成功 | restore 明细表 |
| `Failed` | restore 还原失败 | restore 明细表 |
| `Updated` / `Update failed` | update 成功 / 失败 | update 结果行与汇总表 |
| `Installed` / `Install failed` | install 成功 / 失败 | install 结果行 |

颜色：失败类（`Failed`、`Update failed`、`Install failed` 等）红色，`Skipped` 黄色，`Not github` / `Proxy set` 白色，其余绿色。
