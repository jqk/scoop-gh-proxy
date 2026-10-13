package scoop

import (
	"bytes"
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
			d.feed(c.line) // nil-safe：不应 panic
			d = &downloadProgress{}
			d.feed(c.line)
			if d.total != c.total {
				t.Errorf("total = %d, want %d", d.total, c.total)
			}
		})
	}
}

func TestNilSafeDownloadProgress(t *testing.T) {
	var d *downloadProgress
	d.feed("whatever")
	if stop := d.start(); stop == nil {
		t.Error("start 应返回停止函数")
	} else {
		stop()
	}
	stop := d.start()
	stop()
	stop() // 幂等
}

// TestDownloadProgressTickEmits tick 应把 *.download 文件大小与解析出的总量组装成 Progress 事件。
// 两拍语义：首拍只记基线，文件增长后的下一拍才上报
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
	d.mu.Unlock()

	d.tick() // cache 目录为空：不应有事件
	if len(got) != 0 {
		t.Fatalf("无下载文件时不应上报，实际 %v", got)
	}

	file := filepath.Join(cacheDir, "uv#0.13.0#a48fd9a.zip.download")
	resize := func(n int) {
		if err := os.WriteFile(file, make([]byte, n), 0644); err != nil {
			t.Fatal(err)
		}
	}
	bypass := func() { // 绕过限频
		d.mu.Lock()
		d.lastAt = time.Now().Add(-time.Hour)
		d.mu.Unlock()
	}
	resize(420)
	bypass()
	d.tick() // 首拍只记基线：不应有事件
	if len(got) != 0 {
		t.Fatalf("首拍只记基线不应上报，实际 %v", got)
	}

	resize(500)
	bypass()
	d.tick()
	if len(got) != 1 {
		t.Fatalf("期望 1 个事件，实际 %v", got)
	}
	want := Progress{File: "uv#0.13.0#a48fd9a.zip", Downloaded: 500, Total: 1000}
	if got[0] != want {
		t.Errorf("事件不符: got %+v, want %+v", got[0], want)
	}

	// 整百分比未变（509 仍是 50%）时即便绕过限频也不重复上报
	resize(509)
	bypass()
	d.tick()
	if len(got) != 1 {
		t.Errorf("整百分比未变不应重复上报，实际 %v", got)
	}
}

// TestDownloadProgressTickIgnoresStaleDownload 陈旧 .download（上次中断下载遗留、
// 大小不再变化）永不上报。回归：曾在 scoop 输出首行前把遗留半成品当成正在下载
// 误报一帧（total 未知、lastAt 零值使首拍立即放行），下载完成后同文件还会顶替进来错报
func TestDownloadProgressTickIgnoresStaleDownload(t *testing.T) {
	root := t.TempDir()
	cacheDir := filepath.Join(root, "cache")
	if err := os.Mkdir(cacheDir, 0755); err != nil {
		t.Fatal(err)
	}
	var got []Progress
	d := newDownloadProgress(func(p Progress) { got = append(got, p) }, root)
	file := filepath.Join(cacheDir, "xpipe#24.6#2c75734.zip.download")
	if err := os.WriteFile(file, make([]byte, 1<<20), 0644); err != nil {
		t.Fatal(err)
	}

	tick := func() {
		d.mu.Lock()
		d.lastAt = time.Now().Add(-time.Hour) // 即便限频形同虚设
		d.mu.Unlock()
		d.tick()
	}

	// 现场一：scoop 尚未输出（total 未知）的启动窗口
	for range 3 {
		tick()
	}
	if len(got) != 0 {
		t.Fatalf("total 未知时陈旧文件不应上报，实际 %v", got)
	}

	// 现场二：Downloading 行已解析（total 已知）后，真实下载完成、陈旧文件顶替进来
	d.mu.Lock()
	d.total = 35 * 1024 * 1024
	d.mu.Unlock()
	for range 3 {
		tick()
	}
	if len(got) != 0 {
		t.Fatalf("total 已知时陈旧文件不应上报，实际 %v", got)
	}
}

// TestDownloadProgressComplete 下载收尾的补报：曾增长的 .download 消失（scoop 成功后
// 改名挪走）时 complete 补发满额事件；文件保留（下载中断）不虚报；补报一次后清账
func TestDownloadProgressComplete(t *testing.T) {
	root := t.TempDir()
	cacheDir := filepath.Join(root, "cache")
	if err := os.Mkdir(cacheDir, 0755); err != nil {
		t.Fatal(err)
	}
	var got []Progress
	d := newDownloadProgress(func(p Progress) { got = append(got, p) }, root)
	d.mu.Lock()
	d.total = 1000
	d.mu.Unlock()

	var dNil *downloadProgress
	dNil.complete() // nil-safe：不应 panic
	d.complete()    // 未跟踪任何下载：no-op
	if len(got) != 0 {
		t.Fatalf("未跟踪下载时不应补报，实际 %v", got)
	}

	file := filepath.Join(cacheDir, "uv#0.13.0#a48fd9a.zip.download")
	write := func(n int) {
		if err := os.WriteFile(file, make([]byte, n), 0644); err != nil {
			t.Fatal(err)
		}
	}
	bypass := func() { // 绕过限频
		d.mu.Lock()
		d.lastAt = time.Now().Add(-time.Hour)
		d.mu.Unlock()
	}

	write(420)
	bypass()
	d.tick() // 首拍只记基线
	write(900)
	bypass()
	d.tick() // 增长 → 上报 90%
	if len(got) != 1 {
		t.Fatalf("期望 1 个事件，实际 %v", got)
	}

	// 下载中断：部分文件保留在原处，complete 不虚报 100%
	d.complete()
	if len(got) != 1 {
		t.Fatalf("临时文件还在时不应补报，实际 %v", got)
	}

	// 下载完成：.download 改名挪走，complete 补报 100%
	if err := os.Remove(file); err != nil {
		t.Fatal(err)
	}
	d.complete()
	if len(got) != 2 {
		t.Fatalf("期望补报后 2 个事件，实际 %v", got)
	}
	want := Progress{File: "uv#0.13.0#a48fd9a.zip", Downloaded: 1000, Total: 1000}
	if got[1] != want {
		t.Errorf("收尾事件不符: got %+v, want %+v", got[1], want)
	}

	// 补报一次后清账：重复 complete 不再补报
	d.complete()
	if len(got) != 2 {
		t.Fatalf("收尾只补报一次，实际 %v", got)
	}
}

// TestDownloadProgressCompleteAfterStaleReplaces 真实下载完成改名后、下一行输出前，
// 陈旧 .download 顶替进 tick 的跟踪（lastName 被覆盖），complete 仍应针对曾增长的
// 真实文件（active）补报 100%
func TestDownloadProgressCompleteAfterStaleReplaces(t *testing.T) {
	root := t.TempDir()
	cacheDir := filepath.Join(root, "cache")
	if err := os.Mkdir(cacheDir, 0755); err != nil {
		t.Fatal(err)
	}
	var got []Progress
	d := newDownloadProgress(func(p Progress) { got = append(got, p) }, root)
	d.mu.Lock()
	d.total = 1000
	d.mu.Unlock()

	stale := filepath.Join(cacheDir, "stale#1.0#deadbeef.zip.download")
	if err := os.WriteFile(stale, make([]byte, 1<<20), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(stale, time.Now().Add(-time.Hour), time.Now().Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(cacheDir, "uv#0.13.0#a48fd9a.zip.download")
	if err := os.WriteFile(file, make([]byte, 100), 0644); err != nil {
		t.Fatal(err)
	}

	d.tick() // 真实文件较新被选中：首拍基线
	if err := os.WriteFile(file, make([]byte, 900), 0644); err != nil {
		t.Fatal(err)
	}
	d.tick() // 增长 → active = 真实文件
	if len(got) != 1 {
		t.Fatalf("期望 1 个事件，实际 %v", got)
	}

	if err := os.Remove(file); err != nil { // 下载完成：改名挪走
		t.Fatal(err)
	}
	d.tick() // 陈旧文件顶替进跟踪：只记基线、不上报
	if len(got) != 1 {
		t.Fatalf("陈旧文件顶替不应上报，实际 %v", got)
	}
	d.complete()
	if len(got) != 2 {
		t.Fatalf("陈旧文件顶替后仍应补报真实文件的 100%%，实际 %v", got)
	}
	want := Progress{File: "uv#0.13.0#a48fd9a.zip", Downloaded: 1000, Total: 1000}
	if got[1] != want {
		t.Errorf("收尾事件不符: got %+v, want %+v", got[1], want)
	}
}

// TestCompletingWriter 流式输出写入前先补报收尾（100% 那帧先于触发它的下一行
// 输出画上），再透传字节；WriterIsConsole 转发内层显示目标的终端判定
func TestCompletingWriter(t *testing.T) {
	root := t.TempDir()

	var order []string
	d := newDownloadProgress(func(Progress) { order = append(order, "progress") }, root)
	d.mu.Lock()
	d.total = 1000
	d.active = "uv.zip" // 曾增长且已收尾的状态；增长语义由上面的测试覆盖
	d.mu.Unlock()      // cache 下无 uv.zip.download：文件已消失 = 下载完成

	hook := &hookWriter{onWrite: func() { order = append(order, "write") }}
	cw := &completingWriter{w: hook, dp: d}
	if _, err := cw.Write([]byte("Checking hash of uv.zip... ")); err != nil {
		t.Fatal(err)
	}
	if len(order) != 2 || order[0] != "progress" || order[1] != "write" {
		t.Errorf("应先补报再透传，实际顺序 %v", order)
	}
	if got := hook.buf.String(); got != "Checking hash of uv.zip... " {
		t.Errorf("透传内容不符: %q", got)
	}

	// 收尾已清账：后续写入仅透传
	if _, err := cw.Write([]byte("OK.\n")); err != nil {
		t.Fatal(err)
	}
	if len(order) != 3 || order[2] != "write" {
		t.Errorf("收尾只补报一次，实际顺序 %v", order)
	}

	// 终端判定转发：包装层不改变内层声明
	if !(&completingWriter{w: &consoleFake{console: true}}).WriterIsConsole() {
		t.Error("内层为终端时包装层应判定为终端")
	}
	if (&completingWriter{w: &consoleFake{}}).WriterIsConsole() {
		t.Error("内层非终端时包装层应判定为非终端")
	}

	// dp 为 nil（aria2 启用时的形态）时 Write 不 panic、照常透传
	if _, err := (&completingWriter{w: hook}).Write([]byte("x")); err != nil {
		t.Fatal(err)
	}
}

type hookWriter struct {
	buf     bytes.Buffer
	onWrite func()
}

func (h *hookWriter) Write(p []byte) (int, error) {
	h.onWrite()
	return h.buf.Write(p)
}

type consoleFake struct{ console bool }

func (c *consoleFake) Write(p []byte) (int, error) { return len(p), nil }
func (c *consoleFake) WriterIsConsole() bool      { return c.console }
