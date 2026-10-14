package scoop

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDownloadProgressFeed(t *testing.T) {
	const uvURL = "https://github.com/astral-sh/uv/releases/download/0.13.0/uv-x86_64-pc-windows-msvc.zip"
	cases := []struct {
		name   string
		line   string
		total  int64
		target string // 期望的轮询目标；空 = 该行不构成目标
	}{
		{"MB", "Downloading " + uvURL + " (15.0 MB)...", 15 * 1024 * 1024, cacheTag(uvURL)},
		{"GB", "Downloading https://example.com/big.7z (1.5 GB)...", int64(1.5 * 1024 * 1024 * 1024), cacheTag("https://example.com/big.7z")},
		{"KB", "Downloading https://example.com/small.zip (3.4 KB)...", 3481, cacheTag("https://example.com/small.zip")}, // 3.4*1024=3481.6，截断
		{"B", "Downloading https://example.com/tiny.bin (512 B)...", 512, cacheTag("https://example.com/tiny.bin")},
		{"千分位逗号", "Downloading https://example.com/a.zip (1,234.5 KB)...", int64(1234.5 * 1024), cacheTag("https://example.com/a.zip")},
		{"总量未知（Content-Length 缺失）", "Downloading https://example.com/x.zip (unknown)...", 0, cacheTag("https://example.com/x.zip")},
		{"无总量括号（Downloading new version）", "Downloading new version", 0, ""},
		{"普通行", "Checking hash of uv.zip... OK.", 0, ""},
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
			if d.target != c.target {
				t.Errorf("target = %q, want %q", d.target, c.target)
			}
		})
	}
}

// TestCacheTag 与 scoop 真实运行的对照：URL 经 core.ps1 cache_path 规则
// （sha256 小写十六进制前 7 位 + 扩展名）落盘的 cache 文件名哈希段，
// 取自 2026-10 实际运行的日志，防算法复刻漂移
func TestCacheTag(t *testing.T) {
	cases := []struct{ url, hash string }{
		{"https://v4.gh-proxy.org/https://github.com/farion1231/cc-switch/releases/download/v4.0.11/CC-Switch-v4.0.11-Windows-Portable.zip", "cd9b730"},
		{"https://v4.gh-proxy.org/https://github.com/pnpm/pnpm/releases/download/v12.13.0/pnpm-win32-x64.zip", "33c4cc0"},
	}
	for _, c := range cases {
		if got := cacheTag(c.url); got != "#"+c.hash {
			t.Errorf("cacheTag(%q) = %q, want #%s", c.url, got, c.hash)
		}
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
	const url = "https://example.com/uv.zip"
	name := "uv#0.13.0#" + strings.TrimPrefix(cacheTag(url), "#") + ".zip"
	var got []Progress
	d := newDownloadProgress(func(p Progress) { got = append(got, p) }, root)
	d.feed("Downloading " + url + " (1000 B)...") // 总量与轮询目标

	d.tick() // cache 目录为空：不应有事件
	if len(got) != 0 {
		t.Fatalf("无下载文件时不应上报，实际 %v", got)
	}

	file := filepath.Join(cacheDir, name+".download")
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
	want := Progress{File: name, Downloaded: 500, Total: 1000}
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

// TestDownloadProgressTickIgnoresConcurrentForeignDownload cache 目录为所有
// scoop 进程共享：另一窗口的 scoop 正在下载其它 app（其 *.download 持续增长、
// mtime 更新），本程序的进度只跟踪自己的 URL。回归：曾按 mtime 盲选把并发
// 下载的文件画进本程序画面，还配着本程序解析到的总量错报百分比
func TestDownloadProgressTickIgnoresConcurrentForeignDownload(t *testing.T) {
	root := t.TempDir()
	cacheDir := filepath.Join(root, "cache")
	if err := os.Mkdir(cacheDir, 0755); err != nil {
		t.Fatal(err)
	}
	const url = "https://example.com/pnpm.zip"
	name := "pnpm#12.13.0#" + strings.TrimPrefix(cacheTag(url), "#") + ".zip"
	foreign := filepath.Join(cacheDir, "firefox#158.0#71b6569.7z.download")
	own := filepath.Join(cacheDir, name+".download")
	var got []Progress
	d := newDownloadProgress(func(p Progress) { got = append(got, p) }, root)
	write := func(path string, n int) {
		if err := os.WriteFile(path, make([]byte, n), 0644); err != nil {
			t.Fatal(err)
		}
	}
	bypass := func() { // 绕过限频
		d.mu.Lock()
		d.lastAt = time.Now().Add(-time.Hour)
		d.mu.Unlock()
	}

	// 未见 Downloading 行（无目标）的窗口：别人的下载独自增长也不得上报
	write(foreign, 10<<20)
	bypass()
	d.tick()
	if len(got) != 0 {
		t.Fatalf("未见 Downloading 行时并发下载不应上报，实际 %v", got)
	}

	// 目标已知后两个文件都在增长（foreign 后写、mtime 更新），只跟踪自己的
	d.feed("Downloading " + url + " (2000 B)...")
	write(own, 100)
	bypass()
	d.tick() // 首拍基线
	write(foreign, 11<<20)
	write(own, 1000)
	bypass()
	d.tick()
	if len(got) != 1 {
		t.Fatalf("期望只上报自己的下载，实际 %v", got)
	}
	want := Progress{File: name, Downloaded: 1000, Total: 2000}
	if got[0] != want {
		t.Errorf("事件不符: got %+v, want %+v", got[0], want)
	}
}

// TestDownloadProgressTickIgnoresStaleDownload 陈旧 .download（上次中断下载遗留、
// 大小不再变化）永不上报——即便其名字与本次下载的目标相同（同 URL 重下）。
// 回归：曾在 scoop 输出首行前把遗留半成品当成正在下载
// 误报一帧（total 未知、lastAt 零值使首拍立即放行），下载完成后同文件还会顶替进来错报
func TestDownloadProgressTickIgnoresStaleDownload(t *testing.T) {
	root := t.TempDir()
	cacheDir := filepath.Join(root, "cache")
	if err := os.Mkdir(cacheDir, 0755); err != nil {
		t.Fatal(err)
	}
	const url = "https://example.com/xpipe.zip"
	stale := filepath.Join(cacheDir, "xpipe#24.6#"+strings.TrimPrefix(cacheTag(url), "#")+".zip.download")
	if err := os.WriteFile(stale, make([]byte, 1<<20), 0644); err != nil {
		t.Fatal(err)
	}
	var got []Progress
	d := newDownloadProgress(func(p Progress) { got = append(got, p) }, root)

	tick := func() {
		d.mu.Lock()
		d.lastAt = time.Now().Add(-time.Hour) // 即便限频形同虚设
		d.mu.Unlock()
		d.tick()
	}

	// 现场一：scoop 尚未输出（无目标）的启动窗口
	for range 3 {
		tick()
	}
	if len(got) != 0 {
		t.Fatalf("无目标时陈旧文件不应上报，实际 %v", got)
	}

	// 现场二：Downloading 行已解析（目标同名、总量已知）后，陈旧文件仍不增长
	d.feed("Downloading " + url + " (35.0 MB)...")
	for range 3 {
		tick()
	}
	if len(got) != 0 {
		t.Fatalf("目标已知时陈旧文件不应上报，实际 %v", got)
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
	const url = "https://example.com/uv.zip"
	name := "uv#0.13.0#" + strings.TrimPrefix(cacheTag(url), "#") + ".zip"
	var got []Progress
	d := newDownloadProgress(func(p Progress) { got = append(got, p) }, root)
	d.feed("Downloading " + url + " (1000 B)...")

	var dNil *downloadProgress
	dNil.complete() // nil-safe：不应 panic
	d.complete()    // 未跟踪任何下载：no-op
	if len(got) != 0 {
		t.Fatalf("未跟踪下载时不应补报，实际 %v", got)
	}

	file := filepath.Join(cacheDir, name+".download")
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
	want := Progress{File: name, Downloaded: 1000, Total: 1000}
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
// cache 里其它 *.download 顶替进 tick 的视野（名字过滤后不匹配目标、只记基线），
// complete 仍应针对曾增长的真实文件（active）补报 100%
func TestDownloadProgressCompleteAfterStaleReplaces(t *testing.T) {
	root := t.TempDir()
	cacheDir := filepath.Join(root, "cache")
	if err := os.Mkdir(cacheDir, 0755); err != nil {
		t.Fatal(err)
	}
	const url = "https://example.com/uv.zip"
	name := "uv#0.13.0#" + strings.TrimPrefix(cacheTag(url), "#") + ".zip"
	var got []Progress
	d := newDownloadProgress(func(p Progress) { got = append(got, p) }, root)
	d.feed("Downloading " + url + " (1000 B)...")

	stale := filepath.Join(cacheDir, "stale#1.0#deadbeef.zip.download")
	if err := os.WriteFile(stale, make([]byte, 1<<20), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(stale, time.Now().Add(-time.Hour), time.Now().Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(cacheDir, name+".download")
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
	d.tick() // 陈旧文件顶替进视野：不匹配目标、不上报
	if len(got) != 1 {
		t.Fatalf("陈旧文件顶替不应上报，实际 %v", got)
	}
	d.complete()
	if len(got) != 2 {
		t.Fatalf("陈旧文件顶替后仍应补报真实文件的 100%%，实际 %v", got)
	}
	want := Progress{File: name, Downloaded: 1000, Total: 1000}
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
	d.mu.Unlock()       // cache 下无 uv.zip.download：文件已消失 = 下载完成

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
func (c *consoleFake) WriterIsConsole() bool       { return c.console }
