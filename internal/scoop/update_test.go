package scoop

import "testing"

func TestClassifyUpdateLine(t *testing.T) {
	tests := []struct {
		name string
		line string
		want bool
	}{
		{"空行", "", false},
		{"普通进度", "Installing 'git' (2.56.0) [64bit] from github.com", false},
		{"下载进度", "Downloading git 34.5%", false},
		{"scoop 自身更新成功", "Scoop was updated successfully!", false},
		{"app 更新成功", "Successfully updated 'git'.", false},
		{"git 无法访问远端", "fatal: unable to access 'https://github.com/git-for-windows/git/': Failed to connect to 127.0.0.1 port 10809", true},
		{"域名解析失败", "fatal: Could not resolve host: github.com", true},
		{"下载失败", "ERROR Download failed!", true},
		{"连接/下载超时", "The operation has timed out.", true},
		{"大小写不敏感", "FATAL: Unable to access 'https://github.com/': Could not resolve host", true},
		{"本地改动与远端冲突", "error: Your local changes to the following files would be overwritten by merge:", true},
		{"本地文件已改动", "error: your local changes would be lost", true},
		{"目录不是 git 仓库", "fatal: not a git repository (or any of the parent directories): .git", true},
		{"目录属主可疑", "fatal: detected dubious ownership in repository at 'E:/Scoop/buckets/extras'", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := classifyUpdateLine(tt.line); got != tt.want {
				t.Errorf("classifyUpdateLine(%q) = %v, want %v", tt.line, got, tt.want)
			}
		})
	}
}
