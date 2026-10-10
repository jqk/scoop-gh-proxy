package scoop

import (
	"bytes"
	"errors"
	"io"
	"os"

	"encoding/json/jsontext"
)

// ManifestEdit 表示 manifest 中一处待替换的 url 字符串字面量。
// 应用时把原文件字节区间 [Start, End)（含两侧引号）整体替换为 Replacement
type ManifestEdit struct {
	Start       int
	End         int
	Replacement []byte
}

// prepareAppManifest 定位 app 的 manifest 并分析其中的 GitHub URL。
// 返回 true 表示存在待修改的 GitHub URL（Status 为 IsGitHub），可继续执行代理设置；
// 其余情况返回 false，具体状态记录在 app.Status。--set 与 --update 共用
func prepareAppManifest(cfg *ScoopConfig, app *OutdatedApp) bool {
	if app.Status != Unknown {
		return false // 只处理刚刚初始化，没有被处理过的
	}
	if !fillBucketManifest(cfg.RootPath, app) {
		return false // 找不到对应的 bucket 或 manifest 文件，无法继续处理
	}
	// 分析失败与“无待修改 url（含已设置代理）”都记录在 app.Status
	matched, _ := analyzeManifest(app, cfg.GhProxy)
	return matched
}

// analyzeManifest 读取 manifest 文件并只读定位待修改的 url。
// 有待修改的 url 时设置 Status = IsGitHub，并把原始内容与编辑清单记录到 app，供后续在别处应用；
// url 都已带代理前缀时设置 Status = ProxySet；没有 github 下载链接时设置 Status = NotGitHub。
// 返回 true 表示应该保存
func analyzeManifest(app *OutdatedApp, ghProxy string) (bool, error) {
	data, err := os.ReadFile(app.Manifest) // 读取 manifest 文件
	if err != nil {
		app.Status = ManifestError
		return false, err
	}

	// edits 有元素，说明找到了应该设置代理的 url；proxied 说明已设置了代理。这两者不会同时成立
	edits, proxied, err := locateManifestEdits(data, ghProxy)
	if err != nil {
		app.Status = ManifestError
		return false, err
	}

	if proxied {
		app.Status = ProxySet // 重复执行 set 时的正常状态
		return false, nil
	}
	if len(edits) > 0 { // 说明需要设置 gh 代理
		app.Status = IsGitHub
		app.OriginalManifest = data // 只记录定位结果，实际修改在 setProxiedManifest 中进行
		app.Edits = edits
		return true, nil
	}

	app.Status = NotGitHub // 没有 github 下载链接
	return false, nil
}

// locateManifestEdits 只读扫描 manifest，定位全部待加代理前缀的 url 位置。
// 函数的调用结构就是 manifest 的结构：顶层对象中只有 url 和 architecture
// 两个成员值得看，其它成员整段跳过。
// proxied 表示允许修改的位置上，存在已带代理前缀的 url（此时无需再修改）
func locateManifestEdits(data []byte, ghProxy string) ([]ManifestEdit, bool, error) {
	dec := jsontext.NewDecoder(bytes.NewReader(data))

	if dec.PeekKind() != jsontext.KindBeginObject { // 空文件或顶层不是对象
		return nil, false, errors.New("manifest 顶层不是 JSON 对象")
	}
	dec.ReadToken() // 消耗 '{'

	s := manifestScanner{ghProxy: ghProxy}
	for dec.PeekKind() != jsontext.KindEndObject {
		name, err := readMemberName(dec)
		if err != nil {
			return nil, false, err
		}

		switch name {
		case "url": // url 就在 json 文件顶层
			err = s.scanURLValue(dec)
		case "architecture": // 顶层出现 architecture，其下第二层，也就是 json 文件的第三层会有 url
			err = s.scanArchitecture(dec)
		default:
			err = dec.SkipValue() // 其它成员与修改无关，整段跳过
		}
		if err != nil {
			return nil, false, err
		}
	}
	dec.ReadToken() // 消耗 '}'

	// 顶层值之后只允许空白，与 encoding/json 的行为一致
	if _, err := dec.ReadToken(); !errors.Is(err, io.EOF) {
		if err == nil {
			err = errors.New("manifest 顶层值后有多余内容")
		}
		return nil, false, err
	}

	return s.edits, s.proxied, nil
}

// applyManifestEdits 把编辑清单应用到原字节，返回完整的新文件内容。
// edits 必须按 Start 升序排列，顺序扫描的结果天然如此
func applyManifestEdits(data []byte, edits []ManifestEdit) []byte {
	out := make([]byte, 0, len(data))
	last := 0
	for _, e := range edits {
		out = append(out, data[last:e.Start]...) // 复制原文件未改动的部分
		out = append(out, e.Replacement...)      // 复制改动后的内容，相当于替换掉了被改动的部分
		last = e.End
	}
	return append(out, data[last:]...)
}
