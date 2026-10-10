package scoop

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Progress 一次下载进度的快照（aria2 关闭时由核心轮询 cache 的 *.download 产生）。
// 核心层只产出结构化事件，渲染由调用方（cli / 将来的 GUI）负责
type Progress struct {
	File       string // 正在下载的文件名（cache 中的 *.download，已去后缀）
	Downloaded int64  // 已下载字节数
	Total      int64  // 总字节数；0 = 未知（服务器未返回 Content-Length）
}

// downloadProgress 为 aria2 关闭时的 scoop 默认下载器轮询下载进度。
//
// 背景：scoop 检测到 stdout 被重定向即关闭自带进度条（download.ps1 的
// [console]::IsOutputRedirected 判断），只输出一行 "Downloading <url> (<总量>)..."
// 后静默，管道侧拿不到任何进度字节。好在默认下载器边下边写 cache 目录的
// <name>.download 临时文件（完成后改名），总量又能从那行解析——轮询临时文件
// 大小即可得到进度，以 Progress 事件回调给调用方：
//   - Feed：接到扫描行时解析 "Downloading <url> (15.0 MB)..." 得到本次总量
//   - Start/Stop：轮询 cache 下最新的 *.download（200ms），整百分比变化时发一次事件
//
// aria2 启用时构造方传 nil（aria2 自带 \r 进度流，经透传展示即可）；
// 所有方法 nil-safe，调用点无需判断
type downloadProgress struct {
	onProgress func(Progress)
	cacheDir   string

	mu       sync.Mutex
	total    int64     // 总字节数，0 = 未知
	lastPct  int       // 上次上报的整百分比
	lastAt   time.Time // 上次上报时间（限频）
	lastName string    // 上一拍选中的下载文件名
	lastSize int64     // 上一拍选中时的大小
}

// "Downloading <url> (15.0 MB)..."（download.ps1:160）。总量为 scoop filesize
// 格式：1024 进制、一位小数，单位 B/KB/MB/GB；数值可能带千分位逗号
var downloadingLineRe = regexp.MustCompile(`^Downloading \S+ \(([\d.,]+)\s*(B|KB|MB|GB)\)\.\.\.$`)

func newDownloadProgress(onProgress func(Progress), rootPath string) *downloadProgress {
	return &downloadProgress{onProgress: onProgress, cacheDir: filepath.Join(rootPath, "cache")}
}

// newDownloadProgressFor 按配置决定是否启用进度轮询：aria2 启用或无回调时返回 nil
func newDownloadProgressFor(cfg *ScoopConfig, onProgress func(Progress)) *downloadProgress {
	if cfg == nil || cfg.Aria2Enabled || onProgress == nil {
		return nil
	}
	return newDownloadProgress(onProgress, cfg.RootPath)
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
	if !ok || d.onProgress == nil {
		return
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	// 只跟随正在增长的文件：currentDownload 按 mtime 盲选，上次中断下载遗留的
	// 陈旧 .download（不再写入）会被误当成正在下载而上报，故同名且变大才继续
	growing := name == d.lastName && size > d.lastSize
	d.lastName, d.lastSize = name, size
	if !growing {
		return
	}
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
	d.onProgress(Progress{File: name, Downloaded: size, Total: d.total})
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
