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
