package cli

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/fatih/color"

	"github.com/jqk/scoop-gh-proxy/internal/scoop"
)

func TestPrintTable(t *testing.T) {
	var buf bytes.Buffer
	printTable(&buf,
		[]string{"App Name", "Bucket Name"},
		[][]string{{"git", "main"}, {"verylongappname", "extras"}},
		nil,
	)

	// 列宽 = max(表头, 数据)：第一列 15、第二列 11
	rowFmt := "%-15s  %-11s\n"
	want := fmt.Sprintf(rowFmt, "App Name", "Bucket Name") +
		fmt.Sprintf(rowFmt, strings.Repeat("-", 15), strings.Repeat("-", 11)) +
		fmt.Sprintf(rowFmt, "git", "main") +
		fmt.Sprintf(rowFmt, "verylongappname", "extras")

	if got := buf.String(); got != want {
		t.Errorf("表格输出不符\n got: %q\nwant: %q", got, want)
	}
}

func TestPrintTableSingleRow(t *testing.T) {
	var buf bytes.Buffer
	printTable(&buf, []string{"Name", "Status"}, [][]string{{"uv", "Updated"}}, nil)
	want := "Name  Status \n----  -------\nuv    Updated\n"
	if got := buf.String(); got != want {
		t.Errorf("窄列（数据未超表头）也应按表头宽度对齐\n got: %q\nwant: %q", got, want)
	}
}

func TestBannerText(t *testing.T) {
	oldVersion, oldCommitDate := version, commitDate
	defer func() { version, commitDate = oldVersion, oldCommitDate }()

	version, commitDate = "1.2.3", "2026-10-10"
	want := "Copyright (c) 1999-2026 Not a dream Co., Ltd.\nscoop-gh-proxy 1.2.3, 2026-10-10\n\n"
	if got := bannerText(); got != want {
		t.Errorf("banner 不符\n got: %q\nwant: %q", got, want)
	}

	// 直接构建（无注入日期）：省略日期，版权年份取当前年
	version, commitDate = "dev", ""
	got := bannerText()
	want = fmt.Sprintf("Copyright (c) 1999-%d Not a dream Co., Ltd.\nscoop-gh-proxy dev\n\n", time.Now().Year())
	if got != want {
		t.Errorf("dev banner 不符\n got: %q\nwant: %q", got, want)
	}
}

func TestRenderDownloadLine(t *testing.T) {
	total := int64(15 * 1024 * 1024)
	got := renderDownloadLine("uv.zip", total/2, total) // 50%
	if !strings.Contains(got, "uv.zip (  7.5 MB/ 15.0 MB ) [") {
		t.Errorf("50%% 行前缀不符: %q", got)
	}
	if !strings.HasSuffix(got, "]  50%") {
		t.Errorf("50%% 行结尾不符: %q", got)
	}
	bar := got[strings.Index(got, "[")+1 : strings.Index(got, "]")]
	if !strings.Contains(bar, ">") || len(bar) != 30 {
		t.Errorf("进度条形态不符: %q", bar)
	}

	// 定宽对齐：508KB 与 23.3MB 的 "[" 应在同一列
	a := renderDownloadLine("f.zip", 508*1024, 15*1024*1024)
	b := renderDownloadLine("f.zip", 233*1024*1024/10, 15*1024*1024)
	if strings.Index(a, "[") != strings.Index(b, "[") {
		t.Errorf("定宽后 [ 应对齐: %q vs %q", a, b)
	}

	got = renderDownloadLine("uv.zip", total, total) // 100%
	if !strings.HasSuffix(got, "] 100%") || strings.Contains(got, ">") {
		t.Errorf("100%% 行不符: %q", got)
	}

	got = renderDownloadLine("uv.zip", 2048, 0) // 总量未知
	if !strings.HasPrefix(got, "uv.zip (2.0 KB) [") {
		t.Errorf("未知总量行不符: %q", got)
	}
}

func TestHumanSize(t *testing.T) {
	cases := []struct {
		in   int64
		want string
	}{
		{512, "512 B"},
		{1024, "1024 B"}, // scoop filesize 为严格大于：恰好 1KB 仍显示 B
		{1025, "1.0 KB"},
		{15 * 1024 * 1024, "15.0 MB"},
		{int64(1.5 * 1024 * 1024 * 1024), "1.5 GB"},
	}
	for _, c := range cases {
		if got := humanSize(c.in); got != c.want {
			t.Errorf("humanSize(%d) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestProgressPrinterLines(t *testing.T) {
	// 非终端（bytes.Buffer）：进度行同样 \r 单行刷新，内容未变不重写；
	// finishLine 补换行落定
	var buf bytes.Buffer
	p := newProgressPrinter(&buf)

	first := renderDownloadLine("f.zip", 1024, 4096)
	second := renderDownloadLine("f.zip", 2048, 4096)
	p.printProgress(scoop.Progress{File: "f.zip", Downloaded: 1024, Total: 4096})
	p.printProgress(scoop.Progress{File: "f.zip", Downloaded: 1024, Total: 4096}) // 内容未变，不再重写
	p.printProgress(scoop.Progress{File: "f.zip", Downloaded: 2048, Total: 4096})
	p.finishLine()

	want := "\r" + first + "\r" + second + "\n"
	if got := buf.String(); got != want {
		t.Errorf("非终端进度输出不符\n got: %q\nwant: %q", got, want)
	}
}

func TestProgressPrinterConsole(t *testing.T) {
	// 终端：\r 原地重绘不换行，新行更短补空格覆盖残留；流式输出与 finishLine 前先补换行
	var buf bytes.Buffer
	p := &progressPrinter{w: &buf, console: true}

	full := renderDownloadLine("f.zip", 4096, 4096) // 100%，宽行
	short := renderDownloadLine("f.zip", 2048, 0)   // 总量未知，短行
	p.printProgress(scoop.Progress{File: "f.zip", Downloaded: 4096, Total: 4096})
	p.printProgress(scoop.Progress{File: "f.zip", Downloaded: 2048, Total: 0})

	want := "\r" + full + "\r" + short + strings.Repeat(" ", len(full)-len(short))

	if _, err := p.Write([]byte("next")); err != nil { // 流式输出前终结进度行
		t.Fatalf("Write 失败: %v", err)
	}
	want += "\nnext"

	p.printProgress(scoop.Progress{File: "g.zip", Downloaded: 1, Total: 0}) // 新一轮下载从新行开始
	want += "\r" + renderDownloadLine("g.zip", 1, 0)
	p.finishLine() // 流程结束时终结未完的进度行
	want += "\n"

	if got := buf.String(); got != want {
		t.Errorf("终端进度输出不符\n got: %q\nwant: %q", got, want)
	}
}

func TestProgressPrinterFinishLineNoop(t *testing.T) {
	// 无未完进度行时 finishLine 与 Write 均不额外输出换行
	var buf bytes.Buffer
	p := newProgressPrinter(&buf)
	p.finishLine()
	if _, err := p.Write([]byte("hello")); err != nil {
		t.Fatalf("Write 失败: %v", err)
	}
	if got, want := buf.String(), "hello"; got != want {
		t.Errorf("无未完行时不应有额外输出\n got: %q\nwant: %q", got, want)
	}
}

func TestListRowColor(t *testing.T) {
	// 测试进程 stdout 非终端，fatih/color 默认关闭转义序列，强制开启以比较颜色码
	oldNoColor := color.NoColor
	color.NoColor = false
	defer func() { color.NoColor = oldNoColor }()

	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.Local)
	rowColor := listRowColor(now)

	code := func(c *color.Color) string { // nil（不着色）与各颜色的稳定签名
		if c == nil {
			return ""
		}
		return c.Sprint("")
	}

	cases := []struct {
		name    string
		updated time.Time
		want    *color.Color // nil = 正常色
	}{
		{"1年内", now.AddDate(0, 0, -1), nil},
		{"恰好1年", now.AddDate(-1, 0, 0), nil},
		{"1年零1天", now.AddDate(-1, 0, -1), color.New(color.FgYellow)},
		{"恰好2年", now.AddDate(-2, 0, 0), color.New(color.FgYellow)},
		{"2年零1天", now.AddDate(-2, 0, -1), color.New(color.FgRed)},
	}
	for _, c := range cases {
		row := []string{"app", "1.0", "main", c.updated.Format(time.DateTime), ""}
		if got := code(rowColor(row)); got != code(c.want) {
			t.Errorf("%s: 颜色不符 got %q, want %q", c.name, got, code(c.want))
		}
	}

	// 无法解析的 Updated（零值输出的空串、非日期文本）不着色
	for _, cells := range [][]string{
		{"app", "1.0", "main", "", ""},
		{"app", "1.0", "main", "Updated", ""},
	} {
		if got := rowColor(cells); got != nil {
			t.Errorf("Updated %q 不应着色", cells[3])
		}
	}
}
