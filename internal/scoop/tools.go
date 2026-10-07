package scoop

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"regexp"
	"sync"
	"sync/atomic"
	"time"
)

// 执行命令行命令，得到的输出中有以下字符
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

// appUpdateTimeout --update 中 scoop update <app> 的超时。
// app 下载被杀不会续传，超时太短大文件永远更新不完，所以要足够宽松
const appUpdateTimeout = 10 * time.Minute

// runScoopStream 流式执行 scoop 子命令，供长时间运行的 scoop update 使用：
//   - 子进程 stdout/stderr 的原始字节实时写入 w（保留颜色与进度条动画），stdin 接管自 os.Stdin
//   - 读取侧按行缓冲（\n、\r 切分），每行去 ANSI 后经 classifyUpdateLine 判定，命中的行收集返回
//   - 超过 timeout 未结束时，通过 Job Object 终止整棵进程树
//
// 返回命中的错误行、是否因超时被终止、进程等待错误。
// 检测到错误行时不提前杀进程，让 scoop 自己的收尾/重试逻辑走完
func runScoopStream(w io.Writer, timeout time.Duration, args ...string) (errLines []string, timedOut bool, runErr error) {
	cmd := exec.Command("scoop", args...)
	cmd.Stdin = os.Stdin // scoop 需要确认时可直接应答

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, false, err
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return nil, false, err
	}

	job, err := newJobObject()
	if err != nil {
		job = nil // 没有 Job Object 时退化为只终止直接子进程
	} else {
		defer job.close() // KILL_ON_JOB_CLOSE：本程序退出时子进程树一并终止
	}

	if err := cmd.Start(); err != nil {
		return nil, false, err
	}
	if job != nil {
		_ = job.assign(cmd.Process) // 加入失败只是超时退化为终止直接子进程，不影响正常流程
	}

	var mu sync.Mutex        // 保护 w 与 errLines：两个管道的读取协程并发回调
	var timedOutFlag atomic.Bool

	killTree := func() {
		timedOutFlag.Store(true)
		if job != nil {
			_ = job.terminate()
		} else {
			_ = cmd.Process.Kill()
		}
	}
	timer := time.AfterFunc(timeout, killTree)
	defer timer.Stop()

	onLine := func(line string) {
		if !classifyUpdateLine(line) {
			return
		}
		mu.Lock()
		errLines = append(errLines, line)
		mu.Unlock()
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
	timedOut = timedOutFlag.Load()

	return errLines, timedOut, runErr
}

// streamPipe 把 r 的原始字节实时写入 w（经 mu 加锁，供两个管道协程共用），
// 并把完整行（\n、\r 切分）去 ANSI 后回调 onLine；r 读完后处理剩余的不完整行
func streamPipe(w io.Writer, mu *sync.Mutex, r io.Reader, onLine func(string)) error {
	const chunkSize = 4096
	tmp := make([]byte, chunkSize)
	var pending []byte // 尚未成行的剩余字节

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
			write(chunk) // 原样透传，保留颜色与 \r 进度动画

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
