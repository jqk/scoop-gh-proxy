# 开发

## 环境要求

- Go（版本随 `go.mod`），Git（版本号推导需要）
- 依赖全部为纯 Go，交叉编译无需 CGO。直接依赖仅两个：`github.com/fatih/color`（彩色输出）、`golang.org/x/sys`（Windows Job Object）

## 构建

### build.bat（正式构建）

```bat
build.bat             :: 双架构构建，输出 dist\amd64\ 与 dist\arm64\（各含 scoop-gh.exe 与 README.md）
build.bat --version   :: 仅预览目标版本号，不构建（-v 同义）
```

- 版本号与提交日期经 `-ldflags -X` 注入 `internal/cli` 包的 `version` / `commitDate` 变量
- 注意 `.bat` 文件必须保持 ASCII（cmd 按 ANSI 代码页解析，UTF-8 中文注释会导致解析错乱）

### 直接 go build（快速验证）

```bat
go build -o dist\scoop-gh.exe .
```

单架构跟随宿主机；版本为 `dev`、无提交日期；输出到 `dist\`，勿在项目根目录留下 exe。

## 版本号规则

从 git tag 推导：**最近 tag 的 patch 号 + 其后的提交次数**。

- 例：tag `v0.1.0` 之后有 5 个提交 → 版本 `0.1.5`
- 无任何 tag 时以 `0.0.0` 为基数，累计全部提交数
- 提交日期取版本对应代码（HEAD）的提交日期：`git log -1 --format=%cs`（YYYY-MM-DD），显示在程序信息头第二行

## 校验

```bat
go build ./...
go vet ./...
go test ./...
```

测试覆盖：manifest 定位/应用、feed 解析与进度收尾补报、成功/失败标志匹配、排序与点名匹配、install 定位、流式行处理（行归一、重绘去重、终端透传，含逐字节喂入的最恶劣块边界）、cli 层表格渲染与参数解析。

## 发布（GitHub Actions）

推送 `v*` 格式 tag 触发 `.github/workflows/release.yml`：

1. 校验 tag 必须匹配 `^v[0-9]+\.[0-9]+\.[0-9]+$`，否则失败不发布（Actions 的 tag 过滤是 glob，正则校验在步骤内做）
2. ubuntu runner 交叉编译 windows amd64 / arm64（矩阵策略，`CGO_ENABLED=0`）
3. 版本取自 tag（去 `v` 前缀），日期取 tag 指向提交的提交日期，同样经 `-ldflags -X` 注入
4. 打包 `scoop-gh-<版本>-<架构>.zip`（内含 `scoop-gh.exe` 与 `README.md`）
5. 经 `softprops/action-gh-release@v2` 发布，自动生成 Release notes

## 代码约定

- Windows only，路径一律用 `filepath`
- 分层单向：`main → internal/cli → internal/scoop`；核心层不打印、不退出、不生成展示文本，进度为结构化 `Progress` 事件回调
- manifest 修改不预定义 struct，用 `encoding/json/jsontext` 流式定位 + 字节区间替换
- scoop 成功/失败标志集中在 `markers.go`，取自 scoop 源码与真实输出，精确匹配句子或行首前缀
- 更详细的设计约束见[架构](Architecture.md)
