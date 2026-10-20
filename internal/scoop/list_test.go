package scoop

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// listRowFmt scoop list 表格行的构造格式：Name@0 Version@10 Source@21 Updated@30 Info@51
// （各列宽 8/9/7/19、列间两个空格，与 findColumnStarts 解析出的列起始位置自洽）
const listRowFmt = "%-8s  %-9s  %-7s  %-19s  %s\n"

func listRow(name, version, source, updated, info string) string {
	return fmt.Sprintf(listRowFmt, name, version, source, updated, info)
}

// sampleListOutput 构造 scoop list 的表格输出（列起始位置与真实输出同构）
func sampleListOutput() string {
	sep := fmt.Sprintf(listRowFmt,
		strings.Repeat("-", 8), strings.Repeat("-", 9), strings.Repeat("-", 7),
		strings.Repeat("-", 19), strings.Repeat("-", 4))
	return "Installed apps:\n\n" +
		fmt.Sprintf(listRowFmt, "Name", "Version", "Source", "Updated", "Info") +
		sep +
		listRow("7zip", "26.04", "main", "2026-10-08 07:22:54", "") +
		listRow("aria2", "1.37.0-1", "main", "2024-06-23 22:01:29", "") +
		listRow("cmirror", "0.1.2", "ajqk", "2025-12-17 06:39:32", "Deprecated package")
}

func mustTime(t *testing.T, s string) time.Time {
	t.Helper()
	v, err := time.ParseInLocation(time.DateTime, s, time.Local)
	if err != nil {
		t.Fatalf("解析测试时间 %q 失败: %s", s, err)
	}
	return v
}

func TestParseListApps(t *testing.T) {
	apps, err := parseListApps(sampleListOutput())
	if err != nil {
		t.Fatalf("parseListApps 失败: %s", err)
	}
	if len(apps) != 3 {
		t.Fatalf("解析出行数 = %d, want 3", len(apps))
	}

	want := []ListApp{
		{"7zip", "26.04", "main", mustTime(t, "2026-10-08 07:22:54"), ""},
		{"aria2", "1.37.0-1", "main", mustTime(t, "2024-06-23 22:01:29"), ""},
		{"cmirror", "0.1.2", "ajqk", mustTime(t, "2025-12-17 06:39:32"), "Deprecated package"},
	}
	for i, w := range want {
		if apps[i] != w {
			t.Errorf("第 %d 行解析结果 = %+v, want %+v", i, apps[i], w)
		}
	}
}

func TestParseListAppsErrors(t *testing.T) {
	tests := []struct {
		name   string
		output string
	}{
		{"行数不足（缺少列名行或分隔线）", "Installed apps:\nName  Version\n"},
		{"分隔线列数不足", "Installed apps:\nName  Version\n----  -------\n7zip  26.04\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := parseListApps(tt.output); err == nil {
				t.Error("应返回错误")
			}
		})
	}
}

func TestParseListAppRobustness(t *testing.T) {
	starts := []int{0, 10, 21, 30, 51}

	t.Run("行短于后续列", func(t *testing.T) {
		app := parseListApp("7zip", starts)
		if app.Name != "7zip" || app.Version != "" || app.Source != "" || !app.Updated.IsZero() || app.Info != "" {
			t.Errorf("解析短行 = %+v", app)
		}
	})

	t.Run("Updated 无法解析时为零值", func(t *testing.T) {
		if app := parseListApp(listRow("7zip", "26.04", "main", "不是时间", ""), starts); !app.Updated.IsZero() {
			t.Errorf("Updated = %v, want 零值", app.Updated)
		}
	})
}

func TestSortListApps(t *testing.T) {
	t.Run("按更新时间升序（最久未更新的在前）", func(t *testing.T) {
		apps, err := parseListApps(sampleListOutput())
		if err != nil {
			t.Fatalf("parseListApps 失败: %s", err)
		}
		sortListApps(apps)

		got := []string{apps[0].Name, apps[1].Name, apps[2].Name}
		want := []string{"aria2", "cmirror", "7zip"}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("排序后顺序 = %v, want %v", got, want)
			}
		}
	})

	t.Run("时间相同时按名称字母序定序", func(t *testing.T) {
		ts := mustTime(t, "2026-10-08 07:22:54")
		apps := []ListApp{
			{Name: "Bat", Updated: ts},
			{Name: "aria2", Updated: ts},
			{Name: "alma", Updated: ts},
		}
		sortListApps(apps)

		got := []string{apps[0].Name, apps[1].Name, apps[2].Name}
		want := []string{"alma", "aria2", "Bat"}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("排序后顺序 = %v, want %v", got, want)
			}
		}
	})

	t.Run("零值时间视为最旧", func(t *testing.T) {
		apps := []ListApp{
			{Name: "old", Updated: mustTime(t, "2024-01-01 00:00:00")},
			{Name: "unknown", Updated: time.Time{}},
		}
		sortListApps(apps)
		if apps[0].Name != "unknown" {
			t.Fatalf("零值时间应排在最前，实际首位 = %s", apps[0].Name)
		}
	})
}
