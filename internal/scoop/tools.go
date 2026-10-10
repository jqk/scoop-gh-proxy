package scoop

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"regexp"
	"sync"
	"time"
)

// ansiRe 匹配输出中的 ANSI 颜色转义码（如 \x1b[32;1m、\x1b[0m）
var ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// stripAnsi 去除输出中的 ANSI 转义码（如 \x1b[32;1m、\x1b[0m）
func stripAnsi(s string) string {
	return ansiRe.ReplaceAllString(s, "")
}

// runScoopTimeout scoop 子命令的超时
const runScoopTimeout = 10 * time.Second

// runScoop 执行 scoop 子命令，返回合并 stdout/stderr 的输出（已去除 ANSI 转义码）。
// 统一收口 scoop 命令的执行并施加超时，避免子进程挂起时程序卡死（GUI 化后尤其重要）
func runScoop(args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), runScoopTimeout)
	defer cancel()

	out, err := exec.CommandContext(ctx, "scoop", args...).CombinedOutput()
	return stripAnsi(string(out)), err
}

// runScoopStream 流式执行 scoop 子命令，供长时间运行的 scoop update 使用：
//   - 子进程 stdout/stderr 实时写入 w（保留颜色；显示侧只输出有内容的行：分隔符折叠、空行与纯空白行丢弃、行尾空白裁剪，对齐真实终端观感），stdin 接管自 os.Stdin
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
		_ = streamPipe(w, &mu, stdoutPipe, onLine)
	}()
	go func() {
		defer wg.Done()
		_ = streamPipe(w, &mu, stderrPipe, onLine)
	}()

	wg.Wait()
	runErr = cmd.Wait()

	return successSeen, runErr
}

// lineFilter 显示侧的行过滤器，滤除管道输出中的伪影，对齐 scoop 在真实终端中
// 直接运行的观感：
//   - 行分隔符折叠：连续 \r 及其后至多一个 \n 归一为单个换行（\r\r\n 重复行尾常见）
//   - 空行与纯空白行不输出（aria2 非终端模式的填充）；行首 \r（进度行前置回车）不产生换行
//   - 行尾空白裁剪；行首缩进保留（空白先缓冲，同行出现内容时冲掉）
type lineFilter struct {
	inSep          bool // 处于行分隔符序列中：已写出换行，后续 \r 与收尾 \n 跳过
	held           int  // 当前行已缓冲、尚未落笔的空白
	lineHasContent bool // 当前行是否已有非空白内容
}

// filter 归一一个字节块，返回应写入显示侧的字节
func (f *lineFilter) filter(chunk []byte) []byte {
	out := make([]byte, 0, len(chunk))
	flushWS := func() { // 行内空白缓冲：内容到来时先冲掉（保留缩进），行结束时丢弃
		for ; f.held > 0; f.held-- {
			out = append(out, ' ')
		}
	}
	for _, b := range chunk {
		switch {
		case b == '\r':
			if !f.inSep { // 分隔符序列开始：有内容才换行，空行/纯空白行不产生换行
				f.inSep = true
				if f.lineHasContent {
					out = append(out, '\r', '\n')
				}
				f.held, f.lineHasContent = 0, false
			}
		case b == '\n':
			if !f.inSep { // inSep 时是分隔符序列的收尾 \n，已计入写出的换行
				if f.lineHasContent {
					out = append(out, '\n')
				}
				f.held, f.lineHasContent = 0, false
			}
		case b == ' ' || b == '\t':
			f.inSep = false
			f.held++
		default:
			f.inSep = false
			flushWS()
			out = append(out, b)
			f.lineHasContent = true
		}
	}
	return out
}

// streamPipe 把 r 的字节实时写入 w（经 mu 加锁，供两个管道协程共用，显示侧过 lineFilter），
// 并把完整行（\n、\r 切分）去 ANSI 后回调 onLine；r 读完后处理剩余的不完整行。
// 扫描判定侧（onLine）吃原始行，不受显示过滤影响
func streamPipe(w io.Writer, mu *sync.Mutex, r io.Reader, onLine func(string)) error {
	const chunkSize = 4096
	tmp := make([]byte, chunkSize)
	var pending []byte // 尚未成行的剩余字节（扫描侧）
	var disp lineFilter // 显示侧过滤状态，跨块保持

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
			if norm := disp.filter(chunk); len(norm) > 0 {
				write(norm) // 保留颜色，仅归一行分隔符与空白
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
			if readErr == io.EOF {
				return nil
			}
			return readErr
		}
	}
}

// fileExists 判断文件是否存在
func fileExists(path string) bool {
	_, err := os.Stat(path) // 文件存在
	return err == nil
}
