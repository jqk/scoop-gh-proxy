package scoop

import (
	"strings"

	"encoding/json/jsontext"
)

const githubURLPrefix = "https://github.com"

// manifestScanner 流式扫描 manifest 的状态：已收集的编辑与"已带代理前缀"标志。
// 扫描方法的调用结构就是 manifest 的结构，每个方法只返回 error
type manifestScanner struct {
	ghProxy string
	edits   []ManifestEdit
	proxied bool
}

// scanArchitecture 扫描 architecture 的值。它的值是对象，每个成员是一个架构块：
// 成员名是 64bit 之类的架构名，块的值是块对象
func (s *manifestScanner) scanArchitecture(dec *jsontext.Decoder) error {
	if dec.PeekKind() != jsontext.KindBeginObject {
		return dec.SkipValue() // architecture 的值不是对象，跳过
	}
	dec.ReadToken() // 消耗 '{'

	for dec.PeekKind() != jsontext.KindEndObject {
		if _, err := readMemberName(dec); err != nil { // 架构名，名字本身无所谓
			return err
		}
		if err := s.scanArchBlock(dec); err != nil {
			return err
		}
	}
	dec.ReadToken() // 消耗 '}'

	return nil
}

// scanArchBlock 扫描一个架构块的值。它的值是对象，其中只有 url 成员算数，其余跳过
func (s *manifestScanner) scanArchBlock(dec *jsontext.Decoder) error {
	if dec.PeekKind() != jsontext.KindBeginObject {
		return dec.SkipValue() // 架构块的值不是对象，跳过
	}
	dec.ReadToken() // 消耗 '{'

	for dec.PeekKind() != jsontext.KindEndObject {
		name, err := readMemberName(dec)
		if err != nil {
			return err
		}

		if name == "url" {
			err = s.scanURLValue(dec)
		} else {
			err = dec.SkipValue()
		}
		if err != nil {
			return err
		}
	}
	dec.ReadToken() // 消耗 '}'

	return nil
}

// readMemberName 读取对象的成员名
func readMemberName(dec *jsontext.Decoder) (string, error) {
	tok, err := dec.ReadToken() // 成员名必为字符串 token
	if err != nil {
		return "", err
	}
	return tok.String(), nil // raw token 只在下一次 Decoder 调用前有效，立即取出
}

// scanURLValue 扫描 url 成员的值：字符串按单个处理，数组逐元素处理，其它类型跳过
func (s *manifestScanner) scanURLValue(dec *jsontext.Decoder) error {
	switch dec.PeekKind() {
	case jsontext.KindString: // url 是字符串，也就是只有一个下载地址
		return s.scanURLElement(dec)

	case jsontext.KindBeginArray: // url 是字符串数组，要下载多个软件包
		dec.ReadToken() // 消耗 '['

		for dec.PeekKind() != jsontext.KindEndArray {
			var err error
			if dec.PeekKind() == jsontext.KindString {
				err = s.scanURLElement(dec)
			} else {
				err = dec.SkipValue() // 数组中的非字符串元素，跳过
			}
			if err != nil {
				return err
			}
		}

		dec.ReadToken() // 消耗 ']'
		return nil

	default:
		return dec.SkipValue() // url 的值既不是字符串也不是数组，跳过
	}
}

// scanURLElement 读取一个字符串字面量，命中修改规则则记录一条编辑。
// ReadValue 返回含两侧引号的原始字节，与文件内容逐字节一致，
// 因此字面量区间可以直接由结束位置减去字节长度得出。
// 已带代理前缀的 url 不修改，只置位 proxied
func (s *manifestScanner) scanURLElement(dec *jsontext.Decoder) error {
	raw, err := dec.ReadValue() // 原始字节，含两侧引号
	if err != nil {
		return err
	}

	url, err := jsontext.AppendUnquote(nil, raw) // 去引号、去转义，得到实际 url
	if err != nil {
		return err
	}

	u := string(url)
	switch {
	case strings.HasPrefix(u, s.ghProxy): // 已带代理前缀，无需修改
		s.proxied = true
		return nil
	case !strings.HasPrefix(u, githubURLPrefix): // 非 github 链接
		return nil
	}

	replacement, err := jsontext.AppendQuote(nil, s.ghProxy+u)
	if err != nil {
		return err
	}

	end := int(dec.InputOffset()) // 刚读过的字面量的结束位置（闭引号之后）
	start := end - len(raw)
	s.edits = append(s.edits, ManifestEdit{Start: start, End: end, Replacement: replacement})
	return nil
}
