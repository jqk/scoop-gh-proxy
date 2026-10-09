package scoop

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"encoding/json/jsontext"
)

const githubURLPrefix = "https://github.com"

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
		return nil, false, fmt.Errorf("manifest 顶层不是 JSON 对象")
	}
	dec.ReadToken() // 消耗 '{'

	var edits []ManifestEdit
	proxied := false
	for dec.PeekKind() != jsontext.KindEndObject {
		name, err := readMemberName(dec)
		if err != nil {
			return nil, false, err
		}

		switch name {
		case "url": // url 就在 json 文件顶层
			edits, proxied, err = scanURLValue(dec, ghProxy, edits)
		case "architecture": // 顶层出现 architecture，其下第二层，也就是 json 文件的第三层会有 url
			var archProxied bool
			edits, archProxied, err = scanArchitecture(dec, ghProxy, edits)
			proxied = proxied || archProxied
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
			err = fmt.Errorf("manifest 顶层值后有多余内容")
		}
		return nil, false, err
	}

	return edits, proxied, nil
}

// scanArchitecture 扫描 architecture 的值。它的值是对象，每个成员是一个架构块：
// 成员名是 64bit 之类的架构名，块的值是块对象
func scanArchitecture(dec *jsontext.Decoder, ghProxy string, edits []ManifestEdit) ([]ManifestEdit, bool, error) {
	if dec.PeekKind() != jsontext.KindBeginObject {
		return edits, false, dec.SkipValue() // architecture 的值不是对象，跳过
	}
	dec.ReadToken() // 消耗 '{'

	proxied := false
	for dec.PeekKind() != jsontext.KindEndObject {
		if _, err := readMemberName(dec); err != nil { // 架构名，名字本身无所谓
			return nil, false, err
		}

		var err error
		var blockProxied bool

		// archBlock 是 x64，arm64 之类的节点，其下有 url
		edits, blockProxied, err = scanArchBlock(dec, ghProxy, edits)
		proxied = proxied || blockProxied
		if err != nil {
			return nil, false, err
		}
	}
	dec.ReadToken() // 消耗 '}'

	return edits, proxied, nil
}

// scanArchBlock 扫描一个架构块的值。它的值是对象，其中只有 url 成员算数，其余跳过
func scanArchBlock(dec *jsontext.Decoder, ghProxy string, edits []ManifestEdit) ([]ManifestEdit, bool, error) {
	if dec.PeekKind() != jsontext.KindBeginObject {
		return edits, false, dec.SkipValue() // 架构块的值不是对象，跳过
	}
	dec.ReadToken() // 消耗 '{'

	proxied := false
	for dec.PeekKind() != jsontext.KindEndObject {
		name, err := readMemberName(dec)
		if err != nil {
			return nil, false, err
		}

		if name == "url" {
			edits, proxied, err = scanURLValue(dec, ghProxy, edits)
		} else {
			err = dec.SkipValue()
		}
		if err != nil {
			return nil, false, err
		}
	}
	dec.ReadToken() // 消耗 '}'

	return edits, proxied, nil
}

// readMemberName 读取对象的成员名
func readMemberName(dec *jsontext.Decoder) (string, error) {
	tok, err := dec.ReadToken() // 成员名必为字符串 token
	if err != nil {
		return "", err
	}
	return tok.String(), nil // raw token 只在下一次 Decoder 调用前有效，立即取出
}

// scanURLValue 扫描 url 成员的值：字符串按单个处理，数组逐元素处理，其它类型跳过。
// proxied 表示其中存在已带代理前缀的 url
func scanURLValue(dec *jsontext.Decoder, ghProxy string, edits []ManifestEdit) ([]ManifestEdit, bool, error) {
	switch dec.PeekKind() {
	case jsontext.KindString: // url 是字符串，也就是只有一个下载地址
		return scanURLElement(dec, ghProxy, edits)

	case jsontext.KindBeginArray: // url 是字符串数据，要下载多个软件包
		dec.ReadToken() // 消耗 '['
		proxied := false

		for dec.PeekKind() != jsontext.KindEndArray {
			var err error
			var elemProxied bool

			if dec.PeekKind() == jsontext.KindString {
				edits, elemProxied, err = scanURLElement(dec, ghProxy, edits)
				proxied = proxied || elemProxied
			} else {
				err = dec.SkipValue() // 数组中的非字符串元素，跳过
			}
			if err != nil {
				return nil, false, err
			}
		}

		dec.ReadToken() // 消耗 ']'
		return edits, proxied, nil

	default:
		return edits, false, dec.SkipValue() // url 的值既不是字符串也不是数组，跳过
	}
}

// scanURLElement 读取一个字符串字面量，命中修改规则则记录一条编辑。
// ReadValue 返回含两侧引号的原始字节，与文件内容逐字节一致，
// 因此字面量区间可以直接由结束位置减去字节长度得出。
// 返回的 proxied 表示该 url 已带代理前缀（不修改，但属于"已设置代理"）
func scanURLElement(dec *jsontext.Decoder, ghProxy string, edits []ManifestEdit) ([]ManifestEdit, bool, error) {
	raw, err := dec.ReadValue() // 原始字节，含两侧引号
	if err != nil {
		return nil, false, err
	}

	url, err := jsontext.AppendUnquote(nil, raw) // 去引号、去转义，得到实际 url
	if err != nil {
		return nil, false, err
	}

	newURL := string(url)

	if strings.HasPrefix(newURL, ghProxy) { // 已带代理前缀，无需修改
		return edits, true, nil
	}

	// 即便做了修改，只要不保存(setProxiedManifest)，也不会改变已存在的 manifest
	if !patchDownloadLink(&newURL, ghProxy) { // 非 github 前缀
		return edits, false, nil
	}

	replacement, err := jsontext.AppendQuote(nil, newURL)
	if err != nil {
		return nil, false, err
	}

	end := int(dec.InputOffset()) // 刚读过的字面量的结束位置（闭引号之后）
	start := end - len(raw)
	return append(edits, ManifestEdit{Start: start, End: end, Replacement: replacement}), false, nil
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

// patchDownloadLink 修改单个 url 字符串，返回是否实际修改。
// 即便修改了，只要不保存，也不会影响已存在的 manifest
func patchDownloadLink(s *string, ghProxy string) bool {
	if !strings.HasPrefix(*s, githubURLPrefix) {
		return false
	}
	// 已带 ghProxy 前缀 → 不重复修改
	if strings.HasPrefix(*s, ghProxy) {
		return false
	}
	*s = ghProxy + *s
	return true
}
