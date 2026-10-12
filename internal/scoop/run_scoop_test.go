package scoop

import (
	"bytes"
	"io"
	"strings"
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
	// 非终端模式：行归一（\r+ 及其后至多一个 \n 为一个行结束、空行与纯空白行
	// 丢弃、行尾空白裁剪、行首缩进保留、行首 \r 不产生换行），
	// \r 重绘（\r 后跟新内容）视为同一行的状态更新、内容变化才输出
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"裸 CR 后跟新内容：重绘，输出上一状态", "a\rb", "a\nb\n"},
		{"已有 CRLF：常规行", "a\r\nb", "a\nb\n"},
		{"纯 LF：常规行", "a\nb", "a\nb\n"},
		{"无换行：EOF 补换行", "abc", "abc\n"},
		{"进度行重绘各状态不同：逐条输出", "\rDownload: [50%]\rDownload: [100%]\n", "Download: [50%]\nDownload: [100%]\n"},
		{"连续相同重绘折叠为一行", "\rP\rP\rP\rP2\r\n", "P\nP2\n"},
		{"重绘被常规行终结：常规行总是输出", "\rP\rP\rResults\r\nNext\r\n", "P\nResults\nNext\n"},
		{"结尾悬挂 CR：EOF 补换行", "abc\r", "abc\n"},
		{"重复行尾 CR CR LF 折叠为一次换行", "a\r\r\nb", "a\nb\n"},
		{"连续 CR 折叠为一次换行", "a\rrb", "a\nrb\n"},
		{"CRLF 空行丢弃", "a\r\n\r\nb", "a\nb\n"},
		{"LF 空行丢弃", "a\n\nb", "a\nb\n"},
		{"纯空白行丢弃（aria2 填充）", "row1\r\n   \r\nrow2", "row1\nrow2\n"},
		{"重绘序列中的空白状态不产生输出", "P\r   \rQ\r\n", "P\nQ\n"},
		{"行尾空白裁剪", "Download: Results:   \r\n", "Download: Results:\n"},
		{"行首缩进保留", "  indented\r\n", "  indented\n"},
		{"全空白输入", "   \r\n", ""},
		{"重绘保留 ANSI 颜色", "\r\x1b[36mP\x1b[0m\r\x1b[36mP\x1b[0m\r\x1b[36mP2\x1b[0m\r\n", "\x1b[36mP\x1b[0m\n\x1b[36mP2\x1b[0m\n"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var out bytes.Buffer
			err := streamPipe(&out, &sync.Mutex{}, &byteReader{data: []byte(c.in)}, func(string) {}, false)
			if err != nil {
				t.Fatalf("streamPipe: %v", err)
			}
			if out.String() != c.want {
				t.Errorf("got %q, want %q", out.String(), c.want)
			}
		})
	}
}

func TestStreamPipeTerminalPassthrough(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"字节原样透传，EOF 半行补换行", "abc\rdef", "abc\rdef\n"},
		{"已以换行结尾不补", "x\r\ny\n", "x\r\ny\n"},
		{"空输入无输出", "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var out bytes.Buffer
			err := streamPipe(&out, &sync.Mutex{}, &byteReader{data: []byte(c.in)}, func(string) {}, true)
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
	// 扫描侧吃原始行（\n、\r 切分），不受显示模式影响
	for _, raw := range []bool{false, true} {
		var lines []string
		err := streamPipe(io.Discard, &sync.Mutex{}, &byteReader{data: []byte("Downloading...\rDownload: [50%] DL:9MiB\nOK")}, func(line string) {
			lines = append(lines, line)
		}, raw)
		if err != nil {
			t.Fatalf("streamPipe(raw=%v): %v", raw, err)
		}
		want := []string{"Downloading...", "Download: [50%] DL:9MiB", "OK"}
		if len(lines) != len(want) {
			t.Fatalf("raw=%v: got %v, want %v", raw, lines, want)
		}
		for i := range want {
			if lines[i] != want[i] {
				t.Errorf("raw=%v, line %d: got %q, want %q", raw, i, lines[i], want[i])
			}
		}
	}
}

func TestStreamPipeScoopAria2Download(t *testing.T) {
	// 模拟 aria2 启用时的完整 scoop 下载块：进度行以 \r 前缀重绘、无换行
	// （真实终端里原地覆盖、整段只占一行），结果表行以换行结尾，行尾带擦除填充
	input := "Starting download with aria2...\r\n" +
		"\rDownload: [#x 0B/0B CN:1 DL:0B]   \rDownload: [#x 0B/0B CN:1 DL:0B]   \rDownload: [#x 0B/0B CN:1 DL:0B]   \rDownload: [#x 1.2MiB/28MiB(4%) CN:4 DL:1.3MiB ETA:20s]   \rDownload: Download Results:   \r\n" +
		"\rDownload: gid   |stat|avg speed  |path/URI   \r\n" +
		"\rDownload: (OK):download completed.   \n"

	wantDisplay := "Starting download with aria2...\n" +
		"Download: [#x 0B/0B CN:1 DL:0B]\n" +
		"Download: [#x 1.2MiB/28MiB(4%) CN:4 DL:1.3MiB ETA:20s]\n" +
		"Download: Download Results:\n" +
		"Download: gid   |stat|avg speed  |path/URI\n" +
		"Download: (OK):download completed.\n"

	// 扫描侧在 \n、\r 处切行：Download 行的行首 \r 紧跟上一行的 \n，
	// 会切出一个空行（既有扫描行为，对标志判定与 observe 回调无害）
	wantLines := []string{
		"Starting download with aria2...",
		"",
		"Download: [#x 0B/0B CN:1 DL:0B]   ",
		"Download: [#x 0B/0B CN:1 DL:0B]   ",
		"Download: [#x 0B/0B CN:1 DL:0B]   ",
		"Download: [#x 1.2MiB/28MiB(4%) CN:4 DL:1.3MiB ETA:20s]   ",
		"Download: Download Results:   ",
		"",
		"Download: gid   |stat|avg speed  |path/URI   ",
		"",
		"Download: (OK):download completed.   ",
	}

	var out bytes.Buffer
	var lines []string
	err := streamPipe(&out, &sync.Mutex{}, strings.NewReader(input), func(line string) {
		lines = append(lines, line)
	}, false)
	if err != nil {
		t.Fatalf("streamPipe: %v", err)
	}
	if out.String() != wantDisplay {
		t.Errorf("display got %q, want %q", out.String(), wantDisplay)
	}
	if len(lines) != len(wantLines) {
		t.Fatalf("onLine got %d lines (%v), want %d", len(lines), lines, len(wantLines))
	}
	for i := range wantLines {
		if lines[i] != wantLines[i] {
			t.Errorf("onLine line %d: got %q, want %q", i+1, lines[i], wantLines[i])
		}
	}
}
