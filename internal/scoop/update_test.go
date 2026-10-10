package scoop

import "testing"

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
		if matchUpdateSuccess(line) {
			t.Errorf("瞬时噪声不应命中成功标志: %q", line)
		}
	}

	successLine := "'uv' (0.13.0) was installed successfully!"
	if !matchUpdateSuccess(successLine) {
		t.Error("成功行应命中成功标志")
	}
	if got := updateVerdict(matchUpdateSuccess(successLine)); got != Updated {
		t.Errorf("updateVerdict = %v, want Updated", got)
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
