package scoop

import (
	"io"
	"sync"
)

// runResult 一次 scoop update/install 流式执行的判定摘要
type runResult struct {
	successSeen bool   // 是否命中成功标志
	failureLine string // 最后命中的失败标志行（空 = 未命中），失败时供输出层提示原因
}

// runScoopForApp 流式执行 scoop <cmd> <spec>：启动下载进度采样、透传输出，
// 逐行扫描成功/失败标志，返回判定摘要。update 与 install 共用
func runScoopForApp(cfg *ScoopConfig, w io.Writer, onProgress func(Progress), cmd, spec string) runResult {
	dp := newDownloadProgressFor(cfg, onProgress)
	stop := dp.start()
	defer stop()

	if dp != nil {
		// 流式输出写入前补画下载收尾的 100%（.download 改名消失 = 下载成功），
		// 见 completingWriter；aria2 启用时 dp 为 nil，透传路径不受影响
		w = &completingWriter{w: w, dp: dp}
	}

	var mu sync.Mutex // 保护 failureLine：两个管道的读取协程并发回调
	failureLine := ""
	observe := func(line string) {
		dp.feed(line)
		if matchScoopFailure(line) {
			mu.Lock()
			failureLine = line // 记录最后一次命中：最接近真实失败原因
			mu.Unlock()
		}
	}

	successSeen, _ := runScoopStream(w, observe, cmd, spec)
	return runResult{successSeen: successSeen, failureLine: failureLine}
}

// proxiedAppCmd 在 manifest 保护罩内执行 scoop <cmd> <spec>：
// 先把 gh_proxy 前缀写入 manifest，结束后无论成败都还原 manifest。
// successStatus / failStatus 为成功标志命中与否对应的结果状态（update 为
// Updated / Update failed，install 为 Installed / Install failed）；
// 还原失败意味着 manifest 仍处于已修改状态，优先展示为 RestoreFailed
func proxiedAppCmd(cfg *ScoopConfig, app *ManagedApp, w io.Writer, onProgress func(Progress), cmd, spec string, successStatus, failStatus ManagedAppStatus) {
	if err := setProxiedManifest(app); err != nil {
		// 备份完成但写入失败时 manifest 缺失，必须把备份还原回去
		_ = restoreProxiedManifest(app)
		app.Status = ManifestError
		return
	}
	if app.Status != IsGitHub { // BackupExists 等状态：无法安全执行，跳过
		return
	}

	r := runScoopForApp(cfg, w, onProgress, cmd, spec)
	app.Failure = r.failureLine // 失败原因行（未命中为空），install 失败时由输出层提示
	v := failStatus
	if r.successSeen {
		v = successStatus
	}

	_ = restoreProxiedManifest(app) // Status 被覆盖为 RestoreSuccess / RestoreFailed
	if app.Status != RestoreSuccess {
		v = RestoreFailed // 还原失败优先展示
	}
	app.Status = v
}
