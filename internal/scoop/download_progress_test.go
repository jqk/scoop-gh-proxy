package scoop

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDownloadProgressFeed(t *testing.T) {
	cases := []struct {
		name  string
		line  string
		total int64
	}{
		{"MB", "Downloading https://github.com/astral-sh/uv/releases/download/0.13.0/uv-x86_64-pc-windows-msvc.zip (15.0 MB)...", 15 * 1024 * 1024},
		{"GB", "Downloading https://example.com/big.7z (1.5 GB)...", int64(1.5 * 1024 * 1024 * 1024)},
		{"KB", "Downloading https://example.com/small.zip (3.4 KB)...", 3481}, // 3.4*1024=3481.6，截断
		{"B", "Downloading https://example.com/tiny.bin (512 B)...", 512},
		{"千分位逗号", "Downloading https://example.com/a.zip (1,234.5 KB)...", int64(1234.5 * 1024)},
		{"无总量括号（Downloading new version）", "Downloading new version", 0},
		{"普通行", "Checking hash of uv.zip... OK.", 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var d *downloadProgress
			d.Feed(c.line) // nil-safe：不应 panic
			d = &downloadProgress{}
			d.Feed(c.line)
			if d.total != c.total {
				t.Errorf("total = %d, want %d", d.total, c.total)
			}
		})
	}
}

func TestNilSafeDownloadProgress(t *testing.T) {
	var d *downloadProgress
	d.Feed("whatever")
	if stop := d.Start(); stop == nil {
		t.Error("Start 应返回停止函数")
	} else {
		stop()
	}
	stop := d.Start()
	stop()
	stop() // 幂等
}

// TestDownloadProgressTickEmits tick 应把 *.download 文件大小与解析出的总量组装成 Progress 事件
func TestDownloadProgressTickEmits(t *testing.T) {
	root := t.TempDir()
	cacheDir := filepath.Join(root, "cache") // 轮询目录是 rootPath 下的 cache 子目录
	if err := os.Mkdir(cacheDir, 0755); err != nil {
		t.Fatal(err)
	}
	var got []Progress
	d := newDownloadProgress(func(p Progress) { got = append(got, p) }, root)
	d.mu.Lock()
	d.total = 1000
	d.lastAt = time.Now().Add(-time.Hour) // 绕过限频
	d.mu.Unlock()

	d.tick() // cache 目录为空：不应有事件
	if len(got) != 0 {
		t.Fatalf("无下载文件时不应上报，实际 %v", got)
	}

	file := filepath.Join(cacheDir, "uv#0.13.0#a48fd9a.zip.download")
	if err := os.WriteFile(file, make([]byte, 420), 0644); err != nil {
		t.Fatal(err)
	}
	d.mu.Lock()
	d.lastAt = time.Now().Add(-time.Hour)
	d.mu.Unlock()

	d.tick()
	if len(got) != 1 {
		t.Fatalf("期望 1 个事件，实际 %v", got)
	}
	want := Progress{File: "uv#0.13.0#a48fd9a.zip", Downloaded: 420, Total: 1000}
	if got[0] != want {
		t.Errorf("事件不符: got %+v, want %+v", got[0], want)
	}

	// 百分比未变（42%）时即便绕过限频也不重复上报
	d.mu.Lock()
	d.lastAt = time.Now().Add(-time.Hour)
	d.mu.Unlock()
	d.tick()
	if len(got) != 1 {
		t.Errorf("整百分比未变不应重复上报，实际 %v", got)
	}
}
