package scoop

import "testing"

// 回归：status -l 的数据行同样按分隔线列位切片。名称含非 ASCII 字符（如中文，
// UTF-8 每字占 3 字节）时按字节切片会让该行整体错位——PowerShell 表格按字符数
// 对齐，解析须按 rune 切。本行的列位无填充富余（列宽即内容宽 + 两空格分隔），
// 字节错位无法被列宽吸收
func TestParseStatusAppNonAsciiName(t *testing.T) {
	starts := []int{0, 8, 15, 22, 31} // Name 6 rune + 2 分隔、Installed 5 + 2、Latest 5 + 2、Missing 7 + 2
	row := "LXGW字体  1.530  1.540           "

	app := parseStatusApp(row, starts)
	if app.Name != "LXGW字体" || app.Installed != "1.530" || app.Latest != "1.540" ||
		app.Missing != "" || app.Info != "" {
		t.Errorf("非 ASCII 名称行解析结果 = %+v", app)
	}
	if app.Status != Unknown {
		t.Errorf("信息齐全的行 Status = %v, want %v", app.Status, Unknown)
	}
}
