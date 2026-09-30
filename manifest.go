package main

import (
	"encoding/json"
	"os"
	"strings"
)

const githubURLPrefix = "https://github.com"

func patchManifest(item *SetCommandItem, ghProxy string, dryRun bool) (isGitHub bool, err error) {
	data, err := os.ReadFile(item.Manifest) // 读取 manifest 文件
	if err != nil {
		item.Status = ManifestError
		return false, err
	}

	var m map[string]any
	if err = json.Unmarshal(data, &m); err != nil { // 解析 json
		item.Status = ManifestError
		return false, err
	}

	isGitHub, _ = patchDownloadLinks(m, ghProxy, dryRun)
	item.ChangedManifest = m

	return isGitHub, nil
}

func patchDownloadLinks(m map[string]any, ghProxy string, dryRun bool) (isGitHub bool, changed bool) {
	isGitHub = false
	changed = false

	if url, ok := m["url"]; ok && downloadLinkIsGitHub(url) {
		isGitHub = true
		if !dryRun && patchURLValue(&url, ghProxy) {
			m["url"] = url
			changed = true
		}
	}

	if arch, ok := m["architecture"].(map[string]any); ok {
		for _, archVal := range arch {
			av, ok := archVal.(map[string]any)
			if !ok {
				continue
			}

			if url, ok := av["url"]; ok && downloadLinkIsGitHub(url) {
				isGitHub = true
				if !dryRun && patchURLValue(&url, ghProxy) {
					av["url"] = url
					changed = true
				}
			}
		}
	}

	return isGitHub, changed
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

// patchURLValue 就地修改 url 值（string 或 []string），结果写回 *v
func patchURLValue(v *any, ghProxy string) bool {
	cur := *v
	switch u := cur.(type) {
	case string:
		changed := patchSingleURL(&u, ghProxy)
		if changed {
			*v = u
		}
		return changed
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
		if changed {
			*v = u
		}
		return changed
	default:
		return false
	}
}

// patchSingleURL 修改单个 url 字符串，返回是否实际修改
func patchSingleURL(s *string, ghProxy string) bool {
	if !strings.HasPrefix(*s, githubURLPrefix) {
		return false
	}
	// 已带 ghProxy 前缀 → 不重复修改
	if strings.HasPrefix(*s, ghProxy) {
		return false
	}
	*s = ghProxy + *s
	return true
}

func downloadLinkIsGitHub(v any) bool {
	switch u := v.(type) {
	case string:
		return strings.HasPrefix(u, githubURLPrefix)
	case []any:
		for _, item := range u {
			if s, ok := item.(string); ok && strings.HasPrefix(s, githubURLPrefix) {
				return true
			}
		}
	}
	return false
}
