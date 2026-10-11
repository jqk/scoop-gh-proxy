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
