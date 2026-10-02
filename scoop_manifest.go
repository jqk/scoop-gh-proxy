package main

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

// manifestEdit 表示 manifest 中一处待替换的 url 字符串字面量。
// 应用时把原文件字节区间 [Start, End)（含两侧引号）整体替换为 Replacement
type manifestEdit struct {
	Start       int
	End         int
	Replacement []byte
}

// locateManifest 读取 manifest 文件并只读定位待修改的 url。
// 命中时设置 Status = IsGitHub，并把原始内容与编辑清单记录到 item，供后续在别处应用
func locateManifest(item *SetCommandItem, ghProxy string) (matched bool, err error) {
	data, err := os.ReadFile(item.Manifest) // 读取 manifest 文件
	if err != nil {
		item.Status = ManifestError
		return false, err
	}

	edits, err := locateManifestEdits(data, ghProxy)
	if err != nil {
		item.Status = ManifestError
		return false, err
	}
	if len(edits) == 0 {
		item.Status = NotGitHub // 没有需要修改的 url
		return false, nil
	}

	item.Status = IsGitHub
	item.OriginalManifest = data // 只记录定位结果，实际修改在 backupManifest 中进行
	item.Edits = edits
	return true, nil
}

// locateManifestEdits 只读扫描 manifest，定位全部待加代理前缀的 url 位置。
// 只处理顶层 url 与 architecture.<架构>.url 两处的字符串值（数组元素也算），不改动任何内容
func locateManifestEdits(data []byte, ghProxy string) ([]manifestEdit, error) {
	dec := jsontext.NewDecoder(bytes.NewReader(data))

	if dec.PeekKind() != jsontext.KindBeginObject { // 空文件或顶层不是对象
		return nil, fmt.Errorf("manifest 顶层不是 JSON 对象")
	}

	loc := manifestLocator{data: data, dec: dec, ghProxy: ghProxy}
	if err := loc.scanObject(objRoot); err != nil {
		return nil, err
	}

	// 顶层对象之后只允许空白，与 encoding/json 一样拒绝多余内容
	if _, err := dec.ReadToken(); !errors.Is(err, io.EOF) {
		if err == nil {
			err = fmt.Errorf("manifest 顶层值后有多余内容")
		}
		return nil, err
	}

	return loc.edits, nil
}

// objRole 表示对象在 manifest 中的位置角色，决定其 url 成员是否允许修改
type objRole uint8

const (
	objRoot    objRole = iota // 顶层对象：url 成员命中；architecture 成员转入 objArch
	objArch                   // architecture 的值对象：各架构块的值转入 objArchSub
	objArchSub                // 架构块对象（64bit 之类）：url 成员命中
	objOther                  // 其它对象：一律不命中
)

// manifestLocator 逐 token 递归下降扫描 manifest，记录命中位置
type manifestLocator struct {
	data    []byte
	dec     *jsontext.Decoder
	ghProxy string
	edits   []manifestEdit
}

// memberTarget 判断对象成员是否命中，hit 为 true 表示其值按 url 处理；
// 若成员的值是对象，childRole 表示该对象的角色
func memberTarget(name string, role objRole) (hit bool, childRole objRole) {
	switch {
	case name == "url" && (role == objRoot || role == objArchSub):
		return true, objOther
	case name == "architecture" && role == objRoot:
		return false, objArch
	case role == objArch: // architecture 对象的每个成员就是一个架构块
		return false, objArchSub
	}
	return false, objOther
}

// scanObject 扫描一个对象。调用前提：PeekKind 为 '{'，返回前消耗对应的 '}'
func (l *manifestLocator) scanObject(role objRole) error {
	if _, err := l.dec.ReadToken(); err != nil { // 消耗 '{'
		return err
	}

	for l.dec.PeekKind() != jsontext.KindEndObject {
		nameTok, err := l.dec.ReadToken() // 成员名，必为字符串 token
		if err != nil {
			return err
		}
		name := nameTok.String() // raw token 只在下一次 Decoder 调用前有效，先取出

		hit, childRole := memberTarget(name, role)
		if err := l.scanValue(hit, childRole); err != nil {
			return err
		}
	}

	_, err := l.dec.ReadToken() // 消耗 '}'
	return err
}

// scanValue 扫描一个值。hit 表示该值（或其数组元素）位于允许修改的 url 位置；
// role 仅当值为对象时使用
func (l *manifestLocator) scanValue(hit bool, role objRole) error {
	switch l.dec.PeekKind() {
	case jsontext.KindBeginObject:
		return l.scanObject(role)
	case jsontext.KindBeginArray:
		return l.scanArray(hit)
	case jsontext.KindString:
		return l.scanString(hit)
	default: // 数字、布尔、null，读过即可
		_, err := l.dec.ReadToken()
		return err
	}
}

// scanArray 扫描一个数组，元素沿用外层的 hit。调用前提：PeekKind 为 '['
func (l *manifestLocator) scanArray(hit bool) error {
	if _, err := l.dec.ReadToken(); err != nil { // 消耗 '['
		return err
	}

	for l.dec.PeekKind() != jsontext.KindEndArray {
		if err := l.scanValue(hit, objOther); err != nil {
			return err
		}
	}

	_, err := l.dec.ReadToken() // 消耗 ']'
	return err
}

// scanString 读取一个字符串 token，hit 时按规则判定，命中则记录一条编辑
func (l *manifestLocator) scanString(hit bool) error {
	offsetBefore := l.dec.InputOffset() // token 起点至多由此向前跳过空白得到
	tok, err := l.dec.ReadToken()
	if err != nil {
		return err
	}
	if !hit {
		return nil
	}

	url := tok.String() // raw token 只在下一次 Decoder 调用前有效，先取出
	newURL := url
	if !patchDownloadLink(&newURL, l.ghProxy) {
		return nil // 非 github 前缀，或已带代理前缀
	}

	off := int(offsetBefore)
	start := off + leadingSeparator(l.data[off:]) // 跳过 token 前的分隔符，得到字面量起点
	end := int(l.dec.InputOffset())
	if l.data[start] != '"' { // 防御：偏移计算异常时宁可报错，不可写坏文件
		return fmt.Errorf("定位 url 字符串偏移失败: %q", url)
	}

	replacement, err := jsontext.AppendQuote(nil, newURL)
	if err != nil {
		return err
	}
	l.edits = append(l.edits, manifestEdit{Start: start, End: end, Replacement: replacement})
	return nil
}

// applyManifestEdits 把编辑清单应用到原字节，返回完整的新文件内容。
// edits 必须按 Start 升序排列，顺序扫描的结果天然如此
func applyManifestEdits(data []byte, edits []manifestEdit) []byte {
	out := make([]byte, 0, len(data))
	last := 0
	for _, e := range edits {
		out = append(out, data[last:e.Start]...)
		out = append(out, e.Replacement...)
		last = e.End
	}
	return append(out, data[last:]...)
}

// patchDownloadLink 修改单个 url 字符串，返回是否实际修改
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

// leadingSeparator 返回 b 开头连续 JSON 分隔符（空白、冒号、逗号）的字节数。
// 相邻两个 token 之间只可能有这些字节，跳过它们即得到后一个 token 的起点
func leadingSeparator(b []byte) int {
	i := 0
	for i < len(b) {
		switch b[i] {
		case ' ', '\t', '\n', '\r', ':', ',':
			i++
		default:
			return i
		}
	}
	return i
}
