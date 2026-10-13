package scoop

import (
	"slices"
	"strings"
)

// scoopSuccessMarkers scoop install/update 成功的输出标志（大小写不敏感子串）。
// 两者都由 scoop 在流程末尾打印，是最终定论；其后可能还有 notes 等输出，不影响判定。
// update 与 install 共用（"was installed successfully" 出自 install.ps1）
var scoopSuccessMarkers = []string{
	"was installed successfully",                 // install.ps1：安装/更新完成（其后 notes 不定长）
	"latest versions for all apps are installed", // scoop-update.ps1：已是最新版本，无需更新
}

// matchScoopSuccess 判断一行（已去 ANSI 的）scoop 输出是否命中成功标志
func matchScoopSuccess(line string) bool {
	line = strings.ToLower(line)
	return slices.ContainsFunc(scoopSuccessMarkers, func(m string) bool {
		return strings.Contains(line, m)
	})
}

// scoopFailureMarkers scoop install/update 失败的输出标志（大小写不敏感子串），
// 取自 scoop 源码（lib\install.ps1 / lib\download.ps1）：失败时先输出 ERROR 行
// （abort 型失败随后以退出码 1 结束），另有静默失败（error 后直接 return，退出码 0）。
// 命中仅用于捕获失败原因行供输出层提示；成败判定仍以成功标志为最终定论
var scoopFailureMarkers = []string{
	"download failed",                      // 下载失败（aria2 与默认下载器）：Download failed! (Error N) ...
	"hash check failed",                    // 哈希校验失败：Hash check failed!
	"installation aborted",                 // 安装器/安装脚本执行失败：Installation aborted. ...
	"is not valid",                         // 下载 URL 不可用：ERROR URL <url> is not valid
	"couldn't find manifest",               // manifest 缺失或损坏（本工具已预检，兜底）
	"doesn't support current architecture", // 架构不支持（静默失败：error 后直接 return）
}

// matchScoopFailure 判断一行（已去 ANSI 的）scoop 输出是否命中失败标志
func matchScoopFailure(line string) bool {
	line = strings.ToLower(line)
	return slices.ContainsFunc(scoopFailureMarkers, func(m string) bool {
		return strings.Contains(line, m)
	})
}
