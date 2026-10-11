package cli

import "testing"

// TestRequestedApps requestedApps：任一参数为 * 时视为全量更新（requested 为 nil）；
// 既有 * 又点名时 starMixed 为 true（调用方负责提示），仅 * 或仅点名时为 false
func TestRequestedApps(t *testing.T) {
	tests := []struct {
		name          string
		args          []string
		want          []string
		wantStarMixed bool
	}{
		{"无参数", []string{}, nil, false},
		{"点名", []string{"git", "nodejs"}, []string{"git", "nodejs"}, false},
		{"仅 *", []string{"*"}, nil, false},
		{"点名混入 *", []string{"git", "*", "nodejs"}, nil, true},
		{"仅一个点名混入 *", []string{"app1", "*"}, nil, true},
		{"仅 *（重复）", []string{"*", "*"}, nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, starMixed := requestedApps(tt.args)
			if len(got) != len(tt.want) {
				t.Fatalf("requestedApps(%v) requested = %v, want %v", tt.args, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("requestedApps(%v) requested = %v, want %v", tt.args, got, tt.want)
				}
			}
			if starMixed != tt.wantStarMixed {
				t.Errorf("requestedApps(%v) starMixed = %v, want %v", tt.args, starMixed, tt.wantStarMixed)
			}
		})
	}
}

// TestParseInstallArg parseInstallArg：<bucket/app>（bucket 可省略）；
// app 不允许为空或包含 /
func TestParseInstallArg(t *testing.T) {
	tests := []struct {
		name    string
		arg     string
		bucket  string
		app     string
		wantErr bool
	}{
		{"指定桶", "extras/vscode", "extras", "vscode", false},
		{"省略桶", "vscode", "", "vscode", false},
		{"空参数", "", "", "", true},
		{"空桶", "/vscode", "", "", true},
		{"空 app", "extras/", "", "", true},
		{"app 含 /", "extras/a/b", "", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bucket, app, err := parseInstallArg(tt.arg)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("parseInstallArg(%q) 应报错, got bucket=%q app=%q", tt.arg, bucket, app)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseInstallArg(%q) 意外出错: %v", tt.arg, err)
			}
			if bucket != tt.bucket || app != tt.app {
				t.Errorf("parseInstallArg(%q) = (%q, %q), want (%q, %q)", tt.arg, bucket, app, tt.bucket, tt.app)
			}
		})
	}
}
