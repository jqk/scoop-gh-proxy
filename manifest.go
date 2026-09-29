package main

import (
	"encoding/json"
	"os"
	"strings"
)

// patchGitHubURLs 将 manifest 中所有以 https://github.com 开头的 url
// 前面加上 ghProxy 前缀。返回是否实际修改。
// 只修改一级 url 和 architecture.xxx.url；值可以是 string 或 []string。
func patchGitHubURLs(manifestPath, ghProxy string) (bool, error) {
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return false, err
	}

	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return false, err
	}

	changed := patchManifestURLs(m, ghProxy)
	if !changed {
		return false, nil
	}

	out, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return false, err
	}
	out = append(out, '\n')
	return true, os.WriteFile(manifestPath, out, 0644)
}

// patchManifestURLs 就地修改已解析的 manifest，返回是否有实际修改
func patchManifestURLs(m map[string]any, ghProxy string) bool {
	changed := false

	if v, ok := m["url"]; ok {
		changed = patchURLValue(&v, ghProxy) || changed
		m["url"] = v
	}

	if arch, ok := m["architecture"].(map[string]any); ok {
		for key, archVal := range arch {
			av, ok := archVal.(map[string]any)
			if !ok {
				continue
			}
			if v, exists := av["url"]; exists {
				changed = patchURLValue(&v, ghProxy) || changed
				av["url"] = v
			}
			arch[key] = av
		}
	}

	return changed
}

// patchURLValue 就地修改 url 值（string 或 []string）
func patchURLValue(v *any, ghProxy string) bool {
	switch u := (*v).(type) {
	case string:
		return patchSingleURL(&u, ghProxy)
	case []any:
		changed := false
		for i, item := range u {
			if s, ok := item.(string); ok {
				if patchSingleURL(&s, ghProxy) {
					changed = true
				}
				u[i] = s
			}
		}
		return changed
	default:
		return false
	}
}

// patchSingleURL 修改单个 url 字符串，返回是否实际修改
func patchSingleURL(s *string, ghProxy string) bool {
	if !strings.HasPrefix(*s, "https://github.com") {
		return false
	}
	// 已带 ghProxy 前缀 → 不重复修改
	if strings.HasPrefix(*s, ghProxy) {
		return false
	}
	*s = ghProxy + *s
	return true
}

// manifestHasGitHubDownloadURL 判断 manifest 文件中是否至少有一个 github.com 下载链接
func manifestHasGitHubDownloadURL(manifestPath string) (bool, error) {
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return false, err
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return false, err
	}

	if v, ok := m["url"]; ok && contentHasGitHubPrefix(v) {
		return true, nil
	}
	if arch, ok := m["architecture"].(map[string]any); ok {
		for _, archVal := range arch {
			av, ok := archVal.(map[string]any)
			if !ok {
				continue
			}
			if v, exists := av["url"]; exists && contentHasGitHubPrefix(v) {
				return true, nil
			}
		}
	}
	return false, nil
}

func contentHasGitHubPrefix(v any) bool {
	switch u := v.(type) {
	case string:
		return strings.HasPrefix(u, "https://github.com")
	case []any:
		for _, item := range u {
			if s, ok := item.(string); ok && strings.HasPrefix(s, "https://github.com") {
				return true
			}
		}
	}
	return false
}
