package scoop

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testGhProxy = "https://ghproxy.example.com/"

// fullManifest 覆盖：顶层 url、architecture 多架构（string/数组）、
// 不应命中的 homepage/checkver.url/autoupdate.url、中文与转义字符保真、末尾无换行
const fullManifest = `{
	"version": "1.2.3",
	"url": "https://github.com/foo/bar/releases/download/v1.2.3/bar.zip",
	"homepage": "https://github.com/foo/bar",
	"checkver": {
		"url": "https://github.com/foo/bar/releases",
		"regex": "tag/v([\\d.]+)"
	},
	"architecture": {
		"64bit": {
			"url": "https://github.com/foo/bar/releases/download/v1.2.3/bar-x64.zip"
		},
		"32bit": {
			"url": [
				"https://github.com/foo/bar/releases/download/v1.2.3/bar-x86.zip",
				"https://example.com/mirror.zip"
			]
		}
	},
	"autoupdate": {
		"url": "https://github.com/foo/bar/releases/download/$version/bar.zip"
	},
	"notes": "中文 & <测试> \"引号\""
}`

// crlfManifest 覆盖：2 空格缩进、CRLF 换行、末尾有换行
const crlfManifest = "{\r\n  \"url\": \"https://github.com/a/b/c.zip\",\r\n  \"name\": \"c\"\r\n}\r\n"

func TestLocateAndApplyFullManifest(t *testing.T) {
	edits, err := locateManifestEdits([]byte(fullManifest), testGhProxy)
	if err != nil {
		t.Fatalf("locateManifestEdits 报错: %v", err)
	}
	if len(edits) != 3 { // 顶层 1 + 64bit 1 + 32bit 数组内 1
		t.Fatalf("期望 3 处编辑，实际 %d 处: %+v", len(edits), edits)
	}

	want := fullManifest
	for _, old := range []string{
		`"https://github.com/foo/bar/releases/download/v1.2.3/bar.zip"`,
		`"https://github.com/foo/bar/releases/download/v1.2.3/bar-x64.zip"`,
		`"https://github.com/foo/bar/releases/download/v1.2.3/bar-x86.zip"`,
	} {
		want = strings.ReplaceAll(want, old, `"`+testGhProxy+old[1:])
	}

	if got := string(applyManifestEdits([]byte(fullManifest), edits)); got != want {
		t.Fatalf("应用编辑后内容不符\n got: %q\nwant: %q", got, want)
	}

	// 幂等：对输出再次定位应无命中（已带代理前缀）
	if edits2, err := locateManifestEdits([]byte(want), testGhProxy); err != nil || len(edits2) != 0 {
		t.Fatalf("再次定位应无编辑，实际 edits=%v err=%v", edits2, err)
	}
}

func TestLocateAndApplyCRLF(t *testing.T) {
	edits, err := locateManifestEdits([]byte(crlfManifest), testGhProxy)
	if err != nil {
		t.Fatalf("locateManifestEdits 报错: %v", err)
	}
	if len(edits) != 1 {
		t.Fatalf("期望 1 处编辑，实际 %d 处", len(edits))
	}

	want := strings.ReplaceAll(crlfManifest,
		`"https://github.com/a/b/c.zip"`, `"`+testGhProxy+`https://github.com/a/b/c.zip"`)
	if got := string(applyManifestEdits([]byte(crlfManifest), edits)); got != want {
		t.Fatalf("应用编辑后内容不符\n got: %q\nwant: %q", got, want)
	}
}

func TestLocateManifestEdits(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantEdits int
		wantErr   bool
	}{
		{"非 github url", `{"url": "https://example.com/x"}`, 0, false},
		{"已带代理前缀", `{"url": "` + testGhProxy + `https://github.com/a/b"}`, 0, false},
		{"数组内均非 github", `{"url": ["https://example.com/1", "https://example.com/2"]}`, 0, false},
		{"数组内部分命中", `{"url": ["https://github.com/a/1", "https://example.com/2"]}`, 1, false},
		{"顶层与架构块并存都命中", `{"url": "https://github.com/a/b", "architecture": {"64bit": {"url": "https://github.com/a/c"}}}`, 2, false},
		{"架构块数组 url", `{"architecture": {"arm64": {"url": ["https://github.com/a/d"]}}}`, 1, false},
		{"url 值为数字", `{"url": 123}`, 0, false},
		{"url 值为对象", `{"url": {"x": "https://github.com/a/b"}}`, 0, false},
		{"url 数组元素为对象", `{"url": [{"u": "https://github.com/a/b"}]}`, 0, false},
		{"architecture 值为数组", `{"architecture": ["x"], "url": "https://github.com/a/b"}`, 1, false},
		{"非顶层的 architecture 不命中", `{"extra": {"architecture": {"64bit": {"url": "https://github.com/a/b"}}}}`, 0, false},
		{"checkver.url 不命中", `{"checkver": {"url": "https://github.com/a/releases"}}`, 0, false},
		{"autoupdate.url 不命中", `{"autoupdate": {"url": "https://github.com/a/b"}}`, 0, false},
		{"无 url 字段", `{"name": "x", "bin": "x.exe"}`, 0, false},
		{"空文件", ``, 0, true},
		{"顶层是数组", `[1, 2]`, 0, true},
		{"顶层是字符串", `"hello"`, 0, true},
		{"JSON 截断", `{"url": "https://github.com/a/b"`, 0, true},
		{"顶层值后有多余内容", `{"url": "https://github.com/a/b"} extra`, 0, true},
		{"语法错误", `{"url": "a" "b"}`, 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			edits, err := locateManifestEdits([]byte(tt.input), testGhProxy)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("期望报错，实际无错，edits=%v", edits)
				}
				return
			}
			if err != nil {
				t.Fatalf("非期望的报错: %v", err)
			}
			if len(edits) != tt.wantEdits {
				t.Fatalf("期望 %d 处编辑，实际 %d 处: %+v", tt.wantEdits, len(edits), edits)
			}
			// 编辑区间必须指向字符串字面量，且应用后仍是合法输入的超集替换
			data := []byte(tt.input)
			for i, e := range edits {
				if data[e.Start] != '"' || data[e.End-1] != '"' {
					t.Fatalf("第 %d 处编辑区间未落在字符串字面量上: %+v", i, e)
				}
			}
		})
	}
}

func TestLocateManifestItemFlow(t *testing.T) {
	dir := t.TempDir()
	manifest := filepath.Join(dir, "app.json")
	if err := os.WriteFile(manifest, []byte(fullManifest), 0644); err != nil {
		t.Fatal(err)
	}

	item := OutdatedApp{Name: "app", Manifest: manifest}
	matched, err := locateManifest(&item, testGhProxy)
	if err != nil || !matched || item.Status != IsGitHub || len(item.Edits) != 3 {
		t.Fatalf("首次定位不符: matched=%v err=%v status=%v edits=%d", matched, err, item.Status, len(item.Edits))
	}

	// 应用编辑并写回后，再次定位应判为 Not github（幂等）
	out := applyManifestEdits(item.OriginalManifest, item.Edits)
	if err := os.WriteFile(manifest, out, 0644); err != nil {
		t.Fatal(err)
	}
	item2 := OutdatedApp{Name: "app", Manifest: manifest}
	matched2, err := locateManifest(&item2, testGhProxy)
	if err != nil || matched2 || item2.Status != NotGitHub {
		t.Fatalf("再次定位应为 Not github: matched=%v err=%v status=%v", matched2, err, item2.Status)
	}
}
