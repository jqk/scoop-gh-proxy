package scoop

import (
	"slices"
	"testing"
)

func TestMatchScoopSuccess(t *testing.T) {
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
			if got := matchScoopSuccess(tt.line); got != tt.want {
				t.Errorf("matchScoopSuccess(%q) = %v, want %v", tt.line, got, tt.want)
			}
		})
	}
}

// TestAria2FallbackScenario 端到端回归：aria2 下载失败 → WARN → Fallback →
// 默认下载器成功 → 安装成功。中途的瞬时噪声不得命中成功标志，最终以成功行判定
func TestAria2FallbackScenario(t *testing.T) {
	noise := []string{
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
	}
	for _, line := range noise {
		if matchScoopSuccess(line) {
			t.Errorf("瞬时噪声不应命中成功标志: %q", line)
		}
	}

	successLine := "'uv' (0.13.0) was installed successfully!"
	if !matchScoopSuccess(successLine) {
		t.Error("成功行应命中成功标志")
	}
}

// TestMatchScoopFailure 失败标志行样例取自 scoop 源码（lib\install.ps1 / lib\download.ps1）
// 与真实输出：命中仅用于捕获失败原因，成败判定不受影响
func TestMatchScoopFailure(t *testing.T) {
	tests := []struct {
		name string
		line string
		want bool
	}{
		{"下载失败（aria2 回退关闭）", "ERROR Download failed! (Error 1) An unknown error occurred", true},
		{"下载失败（aria2 回退前的 WARN 瞬时行）", "WARN  Download failed! (Error 1) An unknown error occurred", true},
		{"哈希校验失败", "Hash check failed!", true},
		{"安装器执行失败", "Installation aborted. You might need to run 'scoop uninstall git' before trying again.", true},
		{"URL 不可用", "ERROR URL https://example.com/x.zip is not valid", true},
		{"manifest 缺失", "ERROR Couldn't find manifest for 'git'.", true},
		{"架构不支持（静默失败）", "'git' doesn't support current architecture!", true},
		{"普通安装行", "Installing 'git' (2.56.0) [64bit] from 'main' bucket", false},
		{"下载进度行", "Downloading https://example.com/x.zip (15.0 MB)...", false},
		{"哈希校验通过", "Checking hash of x.zip... OK.", false},
		{"安装成功行", "'uv' (0.13.0) was installed successfully!", false},
		{"已是最新版本行", "Latest versions for all apps are installed! For more information try 'scoop status'", false},
		{"aria2 瞬时错误行", "Download: fa35c7|ERR |       0B/s|E:/Scoop/cache/uv#0.13.0#a48fd9c.zip", false},
		{"回退提示行", "Fallback to default downloader...", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := matchScoopFailure(tt.line); got != tt.want {
				t.Errorf("matchScoopFailure(%q) = %v, want %v", tt.line, got, tt.want)
			}
		})
	}
}

// TestSortAppsByName 名称字母升序（忽略大小写）：大小写混排归并、字母序相同的
// 行按原名称字节序定序（结果确定）
func TestSortAppsByName(t *testing.T) {
	apps := []ManagedApp{
		{Name: "nodejs"},
		{Name: "Alma"},
		{Name: "git"},
		{Name: "7zip"},
		{Name: "WINDOWS-terminal"},
		{Name: "alm"},
		{Name: "Bitwarden"},
	}
	sortAppsByName(apps)

	got := make([]string, 0, len(apps))
	for _, app := range apps {
		got = append(got, app.Name)
	}
	want := []string{"7zip", "alm", "Alma", "Bitwarden", "git", "nodejs", "WINDOWS-terminal"}
	if !slices.Equal(got, want) {
		t.Errorf("sorted = %v, want %v", got, want)
	}
}

// TestPickRequestedApps 点名匹配：大小写不敏感、静默去重（保留首现顺序）、
// 同名 outdated 行只取第一行、未匹配的点名原样归入 noUpdate
func TestPickRequestedApps(t *testing.T) {
	apps := []ManagedApp{
		{Name: "git"},
		{Name: "nodejs"},
		{Name: "Alma"},
	}

	tests := []struct {
		name         string
		requested    []string
		wantPicked   []string // picked 中的 Name 序列；nil 或空均表示应为空
		wantNoUpdate []string // noUpdate 序列；nil 或空均表示应为空
	}{
		{"空参数", nil, nil, nil},
		{"精确匹配", []string{"git"}, []string{"git"}, nil},
		{"大小写不敏感", []string{"GIT"}, []string{"git"}, nil},
		{"保持点名顺序", []string{"Alma", "git"}, []string{"Alma", "git"}, nil},
		{"静默去重（保留首现）", []string{"git", "GIT", "git"}, []string{"git"}, nil},
		{"去重后保持首现顺序", []string{"nodejs", "git", "NODEJS"}, []string{"nodejs", "git"}, nil},
		{"未匹配", []string{"foo"}, nil, []string{"foo"}},
		{"未匹配记录输入原样", []string{"FOO", "Bar"}, nil, []string{"FOO", "Bar"}},
		{"混合", []string{"foo", "nodejs", "bar", "git", "NODEJS"}, []string{"nodejs", "git"}, []string{"foo", "bar"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			picked, noUpdate := pickRequestedApps(apps, tt.requested)

			gotPicked := make([]string, 0, len(picked))
			for _, app := range picked {
				gotPicked = append(gotPicked, app.Name)
			}
			if !slices.Equal(gotPicked, tt.wantPicked) { // slices.Equal 视 nil 与空切片为相同
				t.Errorf("picked = %v, want %v", gotPicked, tt.wantPicked)
			}
			if !slices.Equal(noUpdate, tt.wantNoUpdate) {
				t.Errorf("noUpdate = %v, want %v", noUpdate, tt.wantNoUpdate)
			}
		})
	}
}
