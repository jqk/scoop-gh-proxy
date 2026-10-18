# FAQ

### update 运行期间为什么清空了我的 scoop proxy？

scoop 的全局 `proxy` 作用于所有下载，与 gh_proxy 前缀叠加会冲突，所以下载 GitHub 应用期间临时清空。你的原值已自动备份到 `gh_scoop_proxy_backup`，Proxied 组结束即恢复；若程序被中断没来得及恢复，下次 `update` 开工时会自动还原，或随时手动执行 `scoop-gh restore`。

### 更新/安装中途断电、Ctrl+C、强杀，会留下什么？

可能遗留两类状态：`<app>-gh-backup.json` 备份文件与被清空的 proxy。两者都会在下次 `update` 开工前自动还原（带黄色警告明细），也可随时手动 `scoop-gh restore`。scoop 子进程由 Windows Job Object 绑定，本程序退出（含异常）时整个子进程树一并终止，不会残留孤儿 powershell 进程。

### 某个 app 更新失败了，为什么退出码还是 0？

设计如此：update 逐个应用执行，单个失败只记录状态并继续下一个，直到全部执行完。退出码 1 仅表示流程级失败（配置错误、`scoop status -l` 失败、proxy 清空失败等）。个别应用的失败看每个应用的结果行（红色 `Update failed`）与汇总的 failed 计数。

### 为什么成败不看 scoop 的退出码？

scoop 出错时退出码经常为 0，且存在静默失败（如架构不支持时不输出 ERROR 也返回 0），退出码不可靠。本工具只认 scoop 在流程末尾打印的成功标志行（`was installed successfully` / `Latest versions for all apps are installed`），未见成功标志一律判失败。

### Proxy set 状态的 app 为什么被跳过？想重新处理怎么办？

该应用的 manifest URL 已带 gh_proxy 前缀，且没有本工具的备份文件，无法安全还原，所以 update 不碰它（下载本身不受影响，仍走前缀代理）。想摘掉前缀重新处理：手动编辑该 manifest 删除前缀，或从桶的 git 仓库还原该文件（如 `git -C "<scoop根目录>\buckets\<桶名>" checkout -- <manifest路径>`），之后 set / update 会把它当作 `Is github` 重新处理。

### 重复执行 set / restore / update 安全吗？

安全，全部幂等：已处理的 manifest 自动跳过（`Proxy set`、`Manifest backup exists`），不重复修改、不覆盖已有备份；restore 对无备份的 app 无操作；proxy 的备份/恢复条件对称，不会覆盖程序运行期间你手动设置的其它 proxy。

### 下载进度条为什么有时有、有时没有？

进度显示取决于 `aria2-enabled`：启用时进度由 scoop 的 aria2 自带输出，透传即得；关闭（默认）时由本工具轮询缓存目录的 `*.download` 文件自绘。另外，输出重定向到文件或管道时（非终端），进度以单行文本刷新，整段下载只占一行。

### 哪些 URL 会被修改？

只修改 manifest 顶层 `url` 与 `architecture.<64bit|32bit|arm64>.url` 中以 `https://github.com` 开头的 URL（值为字符串或字符串数组均支持）；已带 gh_proxy 前缀的跳过；`checkver`、`autoupdate` 等其它成员不动。没有 GitHub 下载链接的应用（`Not github`）不修改 manifest 也不动 proxy，update / install 直接执行。

### update 与「set → scoop update → restore」是什么关系？该用哪个？

日常更新用 `update`——一条命令自动完成「更新 scoop 与 buckets → 加前缀 → 清 proxy → 逐个更新 → 还原」，无需手动收尾。经典三步适合只想预先改好链接、自己控制 scoop 命令的场景。注意：不带参数的 `update` 会先执行一次 `scoop update`（更新 scoop 自身与桶）；`update *` 与点名更新、以及经典三步都不执行 `scoop update`，需要时先手动执行一次。

### status 输出的两个计数是什么意思？

`Manifest to restore: N` 是当前存在的遗留备份数（0 = 干净状态）；`Manifest to Set: N` 是现在执行 set 会产生修改的应用数（都已是 `Proxy set`、`Not github` 或信息不全时为 0）。status 是 dry-run，不修改任何文件。

### 支持非 GitHub 下载的应用吗？

支持。`Not github` 的应用不修改 manifest、不动 proxy，update / install 直接流式执行，成败同样以成功标志判定，结果同样计入明细与汇总。
