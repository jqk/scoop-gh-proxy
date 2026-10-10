package scoop

import (
	"strings"
	"testing"
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

func TestRenderDownloadLine(t *testing.T) {
	total := int64(15 * 1024 * 1024)
	got := renderDownloadLine("uv.zip", total/2, total) // 50%
	if !strings.Contains(got, "uv.zip (7.5 MB/15.0 MB) [") {
		t.Errorf("50%% 行前缀不符: %q", got)
	}
	if !strings.HasSuffix(got, "]  50%") {
		t.Errorf("50%% 行结尾不符: %q", got)
	}
	bar := got[strings.Index(got, "[")+1 : strings.Index(got, "]")]
	if !strings.Contains(bar, ">") || len(bar) != 30 {
		t.Errorf("进度条形态不符: %q", bar)
	}

	got = renderDownloadLine("uv.zip", total, total) // 100%
	if !strings.HasSuffix(got, "] 100%") || strings.Contains(got, ">") {
		t.Errorf("100%% 行不符: %q", got)
	}

	got = renderDownloadLine("uv.zip", 2048, 0) // 总量未知
	if !strings.HasPrefix(got, "uv.zip (2.0 KB) [") {
		t.Errorf("未知总量行不符: %q", got)
	}
}

func TestHumanSize(t *testing.T) {
	cases := []struct {
		in   int64
		want string
	}{
		{512, "512 B"},
		{1024, "1024 B"}, // scoop filesize 为严格大于：恰好 1KB 仍显示 B
		{1025, "1.0 KB"},
		{15 * 1024 * 1024, "15.0 MB"},
		{int64(1.5 * 1024 * 1024 * 1024), "1.5 GB"},
	}
	for _, c := range cases {
		if got := humanSize(c.in); got != c.want {
			t.Errorf("humanSize(%d) = %q, want %q", c.in, got, c.want)
		}
	}
}
