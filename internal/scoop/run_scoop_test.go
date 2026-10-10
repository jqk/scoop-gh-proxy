package scoop

import (
	"bytes"
	"io"
	"sync"
	"testing"
)

// byteReader 每次只吐一个字节，制造最恶劣的块边界
type byteReader struct {
	data []byte
	i    int
}

func (r *byteReader) Read(p []byte) (int, error) {
	if r.i >= len(r.data) {
		return 0, io.EOF
	}
	p[0] = r.data[r.i]
	r.i++
	return 1, nil
}

func TestStreamPipeDisplayNormalization(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"裸 CR 结束有内容的行", "a\rb", "a\r\nb"},
		{"已有 CRLF 保留", "a\r\nb", "a\r\nb"},
		{"纯 LF 保留", "a\nb", "a\nb"},
		{"无换行", "abc", "abc"},
		{"行首 CR 不产生空行（进度行前置回车）", "\rDownload: [50%]\rDownload: [100%]\n", "Download: [50%]\r\nDownload: [100%]\n"},
		{"结尾悬挂 CR", "abc\r", "abc\r\n"},
		{"重复行尾 CR CR LF 折叠为一次换行", "a\r\r\nb", "a\r\nb"},
		{"连续 CR 折叠为一次换行", "a\rrb", "a\r\nrb"},
		{"CRLF 空行丢弃", "a\r\n\r\nb", "a\r\nb"},
		{"LF 空行丢弃", "a\n\nb", "a\nb"},
		{"纯空白行丢弃（aria2 填充）", "row1\r\n   \r\nrow2", "row1\r\nrow2"},
		{"行尾空白裁剪", "Download: Results:   \r\n", "Download: Results:\r\n"},
		{"行首缩进保留", "  indented\r\n", "  indented\r\n"},
		{"全空白输入", "   \r\n", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var out bytes.Buffer
			err := streamPipe(&out, &sync.Mutex{}, &byteReader{data: []byte(c.in)}, func(string) {})
			if err != nil {
				t.Fatalf("streamPipe: %v", err)
			}
			if out.String() != c.want {
				t.Errorf("got %q, want %q", out.String(), c.want)
			}
		})
	}
}

func TestStreamPipeSplitsLinesOnCR(t *testing.T) {
	var lines []string
	err := streamPipe(io.Discard, &sync.Mutex{}, &byteReader{data: []byte("Downloading...\rDownload: [50%] DL:9MiB\nOK")}, func(line string) {
		lines = append(lines, line)
	})
	if err != nil {
		t.Fatalf("streamPipe: %v", err)
	}
	want := []string{"Downloading...", "Download: [50%] DL:9MiB", "OK"}
	if len(lines) != len(want) {
		t.Fatalf("got %v, want %v", lines, want)
	}
	for i := range want {
		if lines[i] != want[i] {
			t.Errorf("line %d: got %q, want %q", i, lines[i], want[i])
		}
	}
}
