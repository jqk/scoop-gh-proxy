package scoop

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
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
//   - feed：解析 "Downloading <url> (15.0 MB)..."，记录总量并以 URL 锁定轮询目标
//   - start/stop：轮询 cache 下属于目标的 *.download（200ms），整百分比变化时发一次事件
//
// cache 目录为所有 scoop 进程共享：另一窗口的 scoop 同样在其中增长临时文件，
// 不加判别会把别人的下载串台画进本程序的画面（还配着错误的总量）。scoop 的
// cache 文件名含 URL 哈希（core.ps1 cache_path），按 "#"+哈希片段匹配文件名
// 即可只跟踪本次下载（见 cacheTag）；未见 Downloading 行（目标为空）时不跟踪
//
// aria2 启用时构造方传 nil（aria2 自带 \r 进度流，经透传展示即可）；
// 所有方法 nil-safe，调用点无需判断
type downloadProgress struct {
	onProgress func(Progress)
	cacheDir   string

	mu       sync.Mutex
	total    int64     // 总字节数，0 = 未知
	target   string    // 轮询目标：URL 的 cache 文件名判别片段（"#"+sha256 前 7 位），空 = 未见 Downloading 行
	lastPct  int       // 上次上报的整百分比
	lastAt   time.Time // 上次上报时间（限频）
	lastName string    // 上一拍选中的下载文件名
	lastSize int64     // 上一拍选中时的大小
	active   string    // 曾观察到增长的文件名（区别于按 mtime 盲选的 lastName），complete 的收尾对象
}

// "Downloading <url> (15.0 MB)..."（download.ps1:160）。总量为 scoop filesize
// 格式：1024 进制、一位小数，单位 B/KB/MB/GB；数值可能带千分位逗号
var downloadingLineRe = regexp.MustCompile(`^Downloading (\S+) \((.*)\)\.\.\.$`)

// 括号内的总量文本（downloadingLineRe 的第二捕获组）；不匹配（服务器未返回
// Content-Length 等）则 total 记 0，仅按已下载量上报
var downloadingSizeRe = regexp.MustCompile(`^([\d.,]+)\s*(B|KB|MB|GB)$`)

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

// feed 解析 scoop 输出行：从 "Downloading <url> (<总量>)..." 捕获本次下载的
// URL（锁定轮询目标，见 cacheTag）与总量（每次新下载都会重新输出该行）。
// nil-safe
func (d *downloadProgress) feed(line string) {
	if d == nil {
		return
	}
	m := downloadingLineRe.FindStringSubmatch(strings.TrimSpace(line))
	if m == nil {
		return
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	d.target = cacheTag(m[1])
	d.total = 0
	d.lastPct = 0
	if sm := downloadingSizeRe.FindStringSubmatch(m[2]); sm != nil {
		if total, err := parseFileSize(sm[1], sm[2]); err == nil {
			d.total = total
		}
	}
}

// parseFileSize 解析 scoop filesize 的数值与单位（1024 进制）
func parseFileSize(numStr, unit string) (int64, error) {
	num, err := strconv.ParseFloat(strings.ReplaceAll(numStr, ",", ""), 64)
	if err != nil {
		return 0, err
	}
	var scale float64
	switch unit {
	case "KB":
		scale = 1024
	case "MB":
		scale = 1024 * 1024
	case "GB":
		scale = 1024 * 1024 * 1024
	default:
		scale = 1
	}
	return int64(num * scale), nil
}

// cacheTag 计算 URL 的 cache 文件名判别片段。scoop 命名 cache 文件为
// "<app>#<version>#<sha256(url) 前 7 位><扩展名>"（core.ps1 cache_path，下载
// 期间加 .download 后缀），app#version 无从得知也不需要——"#"+哈希片段在文件
// 名中唯一定位属于该 URL 的临时文件，把共享 cache 目录中其它 scoop 进程的
// 并发下载排除在外
func cacheTag(url string) string {
	sum := sha256.Sum256([]byte(url))
	return "#" + hex.EncodeToString(sum[:])[:7]
}

// start 启动轮询协程，返回停止函数（幂等）。nil-safe
func (d *downloadProgress) start() (stop func()) {
	if d == nil {
		return func() {}
	}
	done := make(chan struct{})
	go d.loop(done)
	return sync.OnceFunc(func() { close(done) })
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
	d.mu.Lock()
	target := d.target
	d.mu.Unlock()
	name, size, ok := currentDownload(d.cacheDir, target)
	if !ok || d.onProgress == nil {
		return
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	// 只跟随正在增长的文件：名字匹配目标的陈旧 .download（上次中断下载遗留、
	// 不再写入）会被误当成正在下载而上报，故同名且变大才继续
	growing := name == d.lastName && size > d.lastSize
	d.lastName, d.lastSize = name, size
	if !growing {
		return
	}
	d.active = name // 增长的才是在下载的文件：陈旧文件永不增长，不会进入 active
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

// complete 补报下载收尾的 100%：曾观察到增长的 *.download 已消失时（下载成功后
// scoop 将其改名挪走）补发一次满额事件。下载中断则部分文件保留在原处，不虚报。
// 轮询最后一拍常落在 900ms 限频窗口内、或错过改名瞬间（改名到下一行输出之间
// 往往不足一个轮询周期），故收尾由 completingWriter 在流式输出写入前显式触发。
// nil-safe
func (d *downloadProgress) complete() {
	if d == nil {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.active == "" || d.total <= 0 {
		return
	}
	if fileExists(filepath.Join(d.cacheDir, d.active+".download")) {
		return // 临时文件还在：仍在下载或已中断，保持最后的真实进度
	}
	name := d.active
	d.active = ""
	d.onProgress(Progress{File: name, Downloaded: d.total, Total: d.total})
}

// currentDownload 找 cache 目录中属于 target（URL 哈希片段，见 cacheTag）且
// 最新修改的 *.download 临时文件。名字不含片段的一律无视——其它 scoop 进程的
// 并发下载即便正在增长、mtime 更新也不属于本程序；target 为空（尚未见到
// Downloading 行）时无匹配
func currentDownload(dir, target string) (name string, size int64, ok bool) {
	matches, err := filepath.Glob(filepath.Join(dir, "*.download"))
	if err != nil || len(matches) == 0 {
		return "", 0, false
	}
	var best string
	var bestTime time.Time
	for _, m := range matches {
		if target == "" || !strings.Contains(filepath.Base(m), target) {
			continue
		}
		if st, err := os.Stat(m); err == nil && st.ModTime().After(bestTime) {
			best, bestTime, size = m, st.ModTime(), st.Size()
		}
	}
	if best == "" {
		return "", 0, false
	}
	return strings.TrimSuffix(filepath.Base(best), ".download"), size, true
}

// completingWriter 包装显示目标：流式输出写入前给 downloadProgress 一次收尾
// 机会——100% 那帧必须先于触发它的下一行输出（如 "Checking hash of..."）画上，
// 否则进度行被透传字节终结时停在 9x%。仅 runScoopForApp 在进度轮询启用时包装
type completingWriter struct {
	w  io.Writer
	dp *downloadProgress
}

func (c *completingWriter) Write(p []byte) (int, error) {
	c.dp.complete()
	return c.w.Write(p)
}

// WriterIsConsole 转发内层显示目标的终端判定（progressPrinter 等包装器实现了
// consoleAware，透传模式不因本包装而退化）
func (c *completingWriter) WriterIsConsole() bool {
	return writerIsConsole(c.w)
}
