package scoop

import "testing"

func TestClassifyUpdateLine(t *testing.T) {
	tests := []struct {
		name string
		line string
		want bool
	}{
		// 确定性失败（scoop error 助手 / git / abort / 异常 / 进程运行中）
		{"空行", "", false},
		{"scoop error 助手", "ERROR Download failed!", true},
		{"管理员权限不足（pre_uninstall 拼接行）", "Running pre_uninstall script... ERROR clash-verge-rev requires admin rights to update", true},
		{"git 无法访问远端", "fatal: unable to access 'https://github.com/git-for-windows/git/': Failed to connect to 127.0.0.1 port 10809", true},
		{"域名解析失败", "fatal: Could not resolve host: github.com", true},
		{"大小写不敏感", "FATAL: Unable to access 'https://github.com/': Could not resolve host", true},
		{"本地改动与远端冲突", "error: Your local changes to the following files would be overwritten by merge:", true},
		{"本地文件已改动", "error: your local changes would be lost", true},
		{"目录不是 git 仓库", "fatal: not a git repository (or any of the parent directories): .git", true},
		{"目录属主可疑", "fatal: detected dubious ownership in repository at 'E:/Scoop/buckets/extras'", true},
		{"程序仍在运行", "Running process detected, skip updating.", true},
		{"下载失败 new_issue_msg", "Please try again or create a new issue by using the following link and paste your console output:", true},
		{"API 限流提示", "Please try again later or configure your API token using 'scoop config gh_token <your token>'.", true},
		{"PowerShell 异常", "Exception calling \"DownloadFile\" with \"2\" argument(s): \"The remote server returned an error: (404) Not Found.\"", true},

		// aria2 失败重试的瞬时噪声：不是终态，不能命中（成功与否由 updateVerdict 决定）
		{"aria2 SSL 握手失败日志", "Download:   -> [SocketCore.cc:1019] errorCode=1 SSL/TLS handshake failure: Error: 由于吊销服务器已脱机，吊销功能无法检查", false},
		{"aria2 结果表 ERR 行", "Download: fa35c7|ERR |       0B/s|E:/Scoop/cache/uv#0.13.0#a48fd9a.zip", false},
		{"aria2 状态图例", "Download: (ERR):error occurred.", false},
		{"aria2 断点续传提示", "Download: aria2 will resume download if the transfer is restarted.", false},
		{"aria2 日志提示", "Download: If there are any errors, then see the log file. See '-l' option in help/man page for detail", false},
		{"WARN 下载失败（将 Fallback 重试）", "WARN  Download failed! (Error 1) An unknown error occurred", false},
		{"WARN 失败 URL", "WARN  https://github.com/astral-sh/uv/releases/download/0.13.0/uv-x86_64-pc-windows-msvc.zip", false},
		{"WARN aria2 命令行", "WARN  & 'E:\\Scoop\\apps\\aria2\\current\\aria2c.exe' --input-file='E:\\Scoop\\cache\\uv.txt' --user-agent='Scoop'", false},

		// 普通输出
		{"Fallback 提示", "Fallback to default downloader...", false},
		{"普通进度", "Installing 'git' (2.56.0) [64bit] from github.com", false},
		{"下载进度", "Downloading git 34.5%", false},
		{"hash 校验通过", "Checking hash of Clash.Verge_2.5.8_x64-setup.exe... OK.", false},
		{"scoop 自身更新成功", "Scoop was updated successfully!", false},
		{"app 更新成功", "Successfully updated 'git'.", false},
		{"SourceForge 提示（warn 语境）", "SourceForge.net is known for causing hash validation fails. Please try again before opening a ticket.", false},
		{"超时句（瞬态，终态由成功标志决定）", "The operation has timed out.", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := classifyUpdateLine(tt.line); got != tt.want {
				t.Errorf("classifyUpdateLine(%q) = %v, want %v", tt.line, got, tt.want)
			}
		})
	}
}

// TestAria2FallbackScenario 端到端回归：aria2 下载失败 → WARN → Fallback →
// 默认下载器成功 → 安装成功。中途的瞬时噪声不得改变最终判定（成功标志优先）
func TestAria2FallbackScenario(t *testing.T) {
	output := []string{
		"Updating 'uv' (0.12.0 -> 0.13.0)",
		"Downloading new version",
		"Starting download with aria2...",
		"Download:   -> [SocketCore.cc:1019] errorCode=1 SSL/TLS handshake failure: Error: 由于吊销服务器已脱机，吊销功能无法检查",
		"Download: Download Results:",
		"Download: fa35c7|ERR |       0B/s|E:/Scoop/cache/uv#0.13.0#a48fd9a.zip",
		"Download: (ERR):error occurred.",
		"Download: aria2 will resume download if the transfer is restarted.",
		"Download: If there are any errors, then see the log file. See '-l' option in help/man page for detail",
		"WARN  Download failed! (Error 1) An unknown error occurred",
		"WARN  https://github.com/astral-sh/uv/releases/download/0.13.0/uv-x86_64-pc-windows-msvc.zip",
		"Fallback to default downloader...",
		"uv-x86_64-pc-windows-msvc.zip (15.0 MB) [================================================>]",
		"Checking hash of uv-x86_64-pc-windows-msvc.zip... OK.",
		"'uv' (0.13.0) was installed successfully!",
	}

	var errLines []string
	successSeen := false
	for _, line := range output {
		if classifyUpdateLine(line) {
			errLines = append(errLines, line)
		}
		if matchUpdateSuccess(line) {
			successSeen = true
		}
	}
	if len(errLines) != 0 {
		t.Errorf("瞬时噪声不应命中错误分类: %v", errLines)
	}
	if !successSeen {
		t.Error("应捕获到成功标志")
	}
	if got := updateVerdict(successSeen); got != Updated {
		t.Errorf("updateVerdict = %v, want Updated", got)
	}
}

func TestMatchUpdateSuccess(t *testing.T) {
	tests := []struct {
		name string
		line string
		want bool
	}{
		{"安装成功", "'dbx' (0.6.38) was installed successfully!", true},
		{"安装成功（notes 前的成功行）", "'uv' (0.13.0) was installed successfully!", true},
		{"已是最新版本", "Latest versions for all apps are installed! For more information try 'scoop status'", true},
		{"notes 标题不算成功", "Notes", false},
		{"notes 内容不算成功", "Scoop persists data since version 0.11.16. You may need to move data by yourself.", false},
		{"单 app 最新版本行不算成功（其后必有 Latest versions 行）", "dbx: 0.6.38 (latest version)", false},
		{"普通进度", "Checking hash of Clash.Verge_2.5.8_x64-setup.exe... OK.", false},
		{"卸载成功不算安装成功", "'git' (2.56.0) was uninstalled successfully.", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := matchUpdateSuccess(tt.line); got != tt.want {
				t.Errorf("matchUpdateSuccess(%q) = %v, want %v", tt.line, got, tt.want)
			}
		})
	}
}

func TestUpdateVerdict(t *testing.T) {
	if got := updateVerdict(true); got != Updated {
		t.Errorf("updateVerdict(true) = %v, want Updated", got)
	}
	if got := updateVerdict(false); got != UpdateFailed {
		t.Errorf("updateVerdict(false) = %v, want UpdateFailed", got)
	}
}
