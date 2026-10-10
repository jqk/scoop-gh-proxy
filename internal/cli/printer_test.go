package cli

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
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
