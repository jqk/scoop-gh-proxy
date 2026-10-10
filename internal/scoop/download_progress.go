package scoop

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// downloadProgress 为 aria2 关闭时的 scoop 默认下载器自绘下载进度。
//
// 背景：scoop 检测到 stdout 被重定向即关闭自带进度条（download.ps1 的
// [console]::IsOutputRedirected 判断），只输出一行 "Downloading <url> (<总量>)..."
// 后静默，管道侧拿不到任何进度字节。好在默认下载器边下边写 cache 目录的
// <name>.download 临时文件（完成后改名），总量又能从那行解析——轮询临时文件
// 大小即可自行渲染进度：
//   - Feed：接到扫描行时解析 "Downloading <url> (15.0 MB)..." 得到本次总量
//   - Start/Stop：轮询 cache 下最新的 *.download（200ms），整百分比变化时输出一行
//
// aria2 启用时构造方传 nil（aria2 自带 \r 进度流，经 streamPipe 展示即可）；
// 所有方法 nil-safe，调用点无需判断
type downloadProgress struct {
	w        io.Writer
	cacheDir string

	mu      sync.Mutex
	total   int64     // 总字节数，0 = 未知（退化为只显示已下载量）
	lastPct int       // 上次输出的整百分比
	lastAt  time.Time // 上次输出时间（限频）
}

// "Downloading <url> (15.0 MB)..."（download.ps1:160）。总量为 scoop filesize
// 格式：1024 进制、一位小数，单位 B/KB/MB/GB；数值可能带千分位逗号
var downloadingLineRe = regexp.MustCompile(`^Downloading \S+ \(([\d.,]+)\s*(B|KB|MB|GB)\)\.\.\.$`)

func newDownloadProgress(w io.Writer, rootPath string) *downloadProgress {
	return &downloadProgress{w: w, cacheDir: filepath.Join(rootPath, "cache")}
}

// newDownloadProgressFor 按配置决定是否启用自绘进度：aria2 启用时返回 nil
func newDownloadProgressFor(cfg *ScoopConfig, w io.Writer) *downloadProgress {
	if cfg == nil || cfg.Aria2Enabled {
		return nil
	}
	return newDownloadProgress(w, cfg.RootPath)
}

// Feed 解析 scoop 输出行，捕获下载总量（每次新下载都会重新输出该行）。nil-safe
func (d *downloadProgress) Feed(line string) {
	if d == nil {
		return
	}
	m := downloadingLineRe.FindStringSubmatch(strings.TrimSpace(line))
	if m == nil {
		return
	}
	num, err := strconv.ParseFloat(strings.ReplaceAll(m[1], ",", ""), 64)
	if err != nil {
		return
	}
	var unit float64
	switch m[2] {
	case "KB":
		unit = 1024
	case "MB":
		unit = 1024 * 1024
	case "GB":
		unit = 1024 * 1024 * 1024
	default:
		unit = 1
	}

	d.mu.Lock()
	d.total = int64(num * unit)
	d.lastPct = 0
	d.mu.Unlock()
}

// Start 启动轮询协程，返回停止函数（幂等）。nil-safe
func (d *downloadProgress) Start() (stop func()) {
	if d == nil {
		return func() {}
	}
	done := make(chan struct{})
	go d.loop(done)
	var once sync.Once
	return func() { once.Do(func() { close(done) }) }
}

func (d *downloadProgress) loop(done <-chan struct{}) {
	t := time.NewTicker(200 * time.Millisecond)
	defer t.Stop()
	for {
		select {
		case <-done:
			return
		case <-t.C:
			d.tick()
		}
	}
}

func (d *downloadProgress) tick() {
	name, size, ok := currentDownload(d.cacheDir)
	if !ok {
		return
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	now := time.Now()
	if d.total > 0 {
		pct := int(size * 100 / d.total)
		if pct == d.lastPct || now.Sub(d.lastAt) < 900*time.Millisecond {
			return
		}
		d.lastPct, d.lastAt = pct, now
	} else {
		// 总量未知（如服务器未返回 Content-Length）：每 2 秒报一次已下载量
		if now.Sub(d.lastAt) < 2*time.Second {
			return
		}
		d.lastAt = now
	}
	fmt.Fprint(d.w, renderDownloadLine(name, size, d.total)+"\n")
}

// currentDownload 找 cache 目录中正在写入的 *.download 临时文件（取最新修改的那个）
func currentDownload(dir string) (name string, size int64, ok bool) {
	matches, err := filepath.Glob(filepath.Join(dir, "*.download"))
	if err != nil || len(matches) == 0 {
		return "", 0, false
	}
	var best string
	var bestTime time.Time
	for _, m := range matches {
		if st, err := os.Stat(m); err == nil && st.ModTime().After(bestTime) {
			best, bestTime, size = m, st.ModTime(), st.Size()
		}
	}
	if best == "" {
		return "", 0, false
	}
	return strings.TrimSuffix(filepath.Base(best), ".download"), size, true
}

// renderDownloadLine 渲染一行进度（样式仿 scoop 自带进度条）：
//
//	uv-x86_64-pc-windows-msvc.zip (6.3 MB/15.0 MB) [===========>                   ]  42%
func renderDownloadLine(name string, size, total int64) string {
	const barW = 30
	pct := 0
	if total > 0 {
		pct = int(size * 100 / total)
		if pct > 100 {
			pct = 100
		}
	}
	filled := pct * barW / 100
	bar := strings.Repeat("=", filled)
	if filled > 0 && filled < barW {
		bar += ">"
	}
	bar += strings.Repeat(" ", barW-len(bar))

	if total > 0 {
		return fmt.Sprintf("%s (%s/%s) [%s] %3d%%", name, humanSize(size), humanSize(total), bar, pct)
	}
	return fmt.Sprintf("%s (%s) [%s]", name, humanSize(size), bar)
}

// humanSize scoop filesize 的 Go 版（core.ps1：1024 进制，一位小数）
func humanSize(n int64) string {
	const kb, mb, gb = 1 << 10, 1 << 20, 1 << 30
	switch {
	case n > gb:
		return fmt.Sprintf("%.1f GB", float64(n)/gb)
	case n > mb:
		return fmt.Sprintf("%.1f MB", float64(n)/mb)
	case n > kb:
		return fmt.Sprintf("%.1f KB", float64(n)/kb)
	default:
		return fmt.Sprintf("%d B", n)
	}
}
