package scoop

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"sync"

	"golang.org/x/sys/windows"
)

// runScoop 执行 scoop 子命令，返回合并 stdout/stderr 的输出（已去除 ANSI 转义码）。
// 统一收口短命令（config / status -l）的执行。
// 无超时：scoop 子命令的耗时不可控（磁盘慢、杀毒扫描等），超时误杀只会得到半截输出；
// 挂死时由用户手动终止
func runScoop(args ...string) (string, error) {
	out, err := exec.Command("scoop", args...).CombinedOutput()
	return stripAnsi(string(out)), err
}

// runScoopStream 流式执行 scoop 子命令，供长时间运行的 scoop update 使用：
//   - 子进程 stdout/stderr 实时写入 w（保留颜色），stdin 接管自 os.Stdin。
//     显示侧按目标分两种模式（writerIsConsole 判定）：终端原样透传，scoop 进度行
//     自带的 \r 前缀由终端原地覆盖刷新，与 scoop 直跑观感一致；非终端（重定向到
//     文件、GUI 注入的 writer）经 lineFilter 归一——只输出有内容的行（空行与纯
//     空白行丢弃、行尾空白裁剪），进度行的连续重绘仅内容变化时输出
//   - 读取侧按行缓冲（\n、\r 切分），每行去 ANSI 后判定：命中成功标志
//     （was installed successfully / Latest versions ...）置位 successSeen。
//     错误行不另行收集——随透传直接显示在输出中，成败以成功标志为准
//
// 无超时：安装包大小与下载速度不可推测，固定超时会误杀正在进行的下载。代价是子进程
// 真挂死时本函数会一直等待（需手动终止）；本程序退出（含异常）时由 Job Object 的
// KILL_ON_JOB_CLOSE 保证子进程树不残留。
// observe（可为 nil）在每行扫描时回调，供调用方捕获行内容（如解析下载总量）。
// 返回是否见到成功标志、进程等待错误
func runScoopStream(w io.Writer, observe func(string), args ...string) (successSeen bool, runErr error) {
	cmd := exec.Command("scoop", args...)
	cmd.Stdin = os.Stdin // scoop 需要确认时可直接应答

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return false, err
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return false, err
	}

	raw := writerIsConsole(w) // 显示侧模式：终端原样透传（\r 原地重绘生效），否则 lineFilter 归一与重绘去重

	job, err := newJobObject()
	if err != nil {
		job = nil // 没有 Job Object 时失去退出时的孤儿清理，不影响正常流程
	} else {
		defer job.close() // KILL_ON_JOB_CLOSE：本程序退出时子进程树一并终止
	}

	if err := cmd.Start(); err != nil {
		return false, err
	}
	if job != nil {
		_ = job.assign(cmd.Process) // 加入失败只是失去退出时的孤儿清理，不影响正常流程
	}

	var mu sync.Mutex // 保护 w 与 successSeen：两个管道的读取协程并发回调

	onLine := func(line string) {
		if observe != nil {
			observe(line)
		}
		if matchUpdateSuccess(line) {
			mu.Lock()
			successSeen = true
			mu.Unlock()
		}
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_ = streamPipe(w, &mu, stdoutPipe, onLine, raw)
	}()
	go func() {
		defer wg.Done()
		_ = streamPipe(w, &mu, stderrPipe, onLine, raw)
	}()

	wg.Wait()
	runErr = cmd.Wait()

	return successSeen, runErr
}

// writerIsConsole 判断显示目标是否为终端控制台（决定 streamPipe 的显示侧模式）。
// 仅 *os.File 且带控制台模式者算终端；bytes.Buffer、GUI 注入的 writer 等一律视为非终端
func writerIsConsole(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	var mode uint32
	return windows.GetConsoleMode(windows.Handle(f.Fd()), &mode) == nil
}

// lineFilter 显示侧的行过滤器（非终端模式），滤除管道输出中的伪影，对齐 scoop
// 在真实终端中直接运行的观感：
//   - 行归一：\r+ 及其后至多一个 \n 视为一个行结束（\r\r\n 重复行尾常见）；
//     行首 \r（进度行前置回车）不产生换行
//   - 空行与纯空白行不输出（aria2 非终端模式的填充）；行尾空白裁剪；行首缩进保留
//   - 重绘折叠：\r 之后跟新内容视为同一行的状态更新（aria2 进度行由 scoop 以
//     Write-Host "`r..." -NoNewLine 输出，真实终端里原地覆盖、整段下载只占一行）。
//     重定向到文件或 GUI writer 无法覆盖刷新，连续相同的重绘折叠为一行，
//     内容变化时才作为新行输出——不刷屏、不丢状态
type lineFilter struct {
	buf        []byte // 当前未终结的行内容（含行首缩进，行尾空白在终结时裁剪）
	lineOpen   bool   // buf 是否为一条待终结的行
	pendingCR  bool   // 刚读过 \r（可连续），其后续字节决定它是行结束还是重绘开始
	lastRedraw []byte // 最近一次输出的重绘行内容，重绘去重的比较基准
}

// filter 归一一个字节块，返回应写入显示侧的字节
func (f *lineFilter) filter(chunk []byte) []byte {
	out := make([]byte, 0, len(chunk))
	for _, b := range chunk {
		switch {
		case b == '\r':
			f.pendingCR = true
		case b == '\n':
			f.pendingCR = false // \r+ 至多带一个 \n，整个序列是一个行结束
			if f.lineOpen {
				out = append(out, f.finalize(false)...)
			}
		default:
			if f.pendingCR { // \r 后跟新内容：重绘开始，上一行按重绘规则终结
				f.pendingCR = false
				if f.lineOpen {
					out = append(out, f.finalize(true)...)
				}
			}
			if !f.lineOpen {
				f.lineOpen = true
				f.buf = f.buf[:0]
			}
			f.buf = append(f.buf, b)
		}
	}
	return out
}

// finalize 终结当前行，返回应写出的内容（含换行）。redraw 为 true 表示该行已被
// 后续重绘覆盖、参与去重。line 是 f.buf 的子切片，本方法返回前完成全部复制
// （lastRedraw 与返回值均独立于 buf 复用）
func (f *lineFilter) finalize(redraw bool) []byte {
	line := bytes.TrimRight(f.buf, " \t")
	f.lineOpen = false
	f.buf = f.buf[:0]
	if len(bytes.TrimSpace(line)) == 0 {
		return nil // 空行与纯空白行不输出
	}
	if redraw {
		if bytes.Equal(line, f.lastRedraw) {
			return nil // 重绘内容未变化
		}
		f.lastRedraw = append(f.lastRedraw[:0], line...)
	}
	out := make([]byte, 0, len(line)+1)
	out = append(out, line...)
	return append(out, '\n')
}

// finish 返回流结尾未终结行的输出：按重绘处理（末条重绘与已输出内容相同时
// 不重复输出），常规未竟行内容必有差异、不受影响
func (f *lineFilter) finish() []byte {
	f.pendingCR = false
	if !f.lineOpen {
		return nil
	}
	return f.finalize(true)
}

// streamPipe 把 r 的字节实时写入 w（经 mu 加锁，供两个管道协程共用），
// 并把完整行（\n、\r 切分）去 ANSI 后回调 onLine；r 读完后处理剩余的不完整行。
// 扫描判定侧（onLine）吃原始行，不受显示模式影响。
// raw 选择显示侧模式：true 原样透传（显示目标是终端，scoop 进度行的 \r 前缀
// 由终端原地覆盖刷新，与 scoop 直跑观感一致），false 走 lineFilter（非终端，
// 行归一 + 进度重绘去重）
func streamPipe(w io.Writer, mu *sync.Mutex, r io.Reader, onLine func(string), raw bool) error {
	const chunkSize = 4096
	tmp := make([]byte, chunkSize)
	var pending []byte  // 尚未成行的剩余字节（扫描侧）
	var disp lineFilter // 显示侧过滤状态，跨块保持（非终端模式）
	var lastByte byte   // 终端模式最近写出的字节（0 表示尚未写出），流结尾补换行的判断依据

	write := func(p []byte) {
		if w == nil {
			return
		}
		mu.Lock()
		_, _ = w.Write(p)
		mu.Unlock()
	}
	emit := func(line []byte) {
		onLine(stripAnsi(string(line)))
	}

	for {
		n, readErr := r.Read(tmp)
		if n > 0 {
			chunk := tmp[:n]
			if raw {
				write(chunk) // 终端：字节原样透传（保留颜色），重绘交给终端
				lastByte = chunk[len(chunk)-1]
			} else if norm := disp.filter(chunk); len(norm) > 0 {
				write(norm) // 保留颜色，仅归一行分隔符与空白、折叠重绘
			}

			pending = append(pending, chunk...)
			for {
				i := bytes.IndexAny(pending, "\r\n")
				if i < 0 {
					break
				}
				emit(pending[:i])
				if pending[i] == '\r' && i+1 < len(pending) && pending[i+1] == '\n' {
					pending = pending[i+2:] // \r\n 合并为一个行分隔符
				} else {
					pending = pending[i+1:]
				}
			}
		}
		if readErr != nil {
			if len(pending) > 0 {
				emit(pending) // 无行尾的最后一行
			}
			if raw {
				// 流结尾停在半行时补换行，避免本程序后续输出拼接到该行尾部
				if lastByte != 0 && lastByte != '\n' {
					write([]byte{'\n'})
				}
			} else if tail := disp.finish(); len(tail) > 0 {
				write(tail)
			}
			if readErr == io.EOF {
				return nil
			}
			return readErr
		}
	}
}
