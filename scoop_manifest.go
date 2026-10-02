package main

import (
	"encoding/json"
	"os"
	"strings"
)

const githubURLPrefix = "https://github.com"

// patchMatchedManifest 先确定是否需要修改下载链接，若是且 dryRun 为 false 则修改
func patchMatchedManifest(item *SetCommandItem, ghProxy string, dryRun bool) (matched bool, err error) {
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

	if matched = patchDownloadLinks(m, ghProxy, dryRun); !matched {
		item.Status = NotGitHub // 没匹配上就根本不会修改
	} else if !dryRun {
		item.Status = IsGitHub
		// 找到了，又不是 dryRun，必须就修改了，所以保存修改后的结果，这样可以传出去
		item.ChangedManifest = m
	}

	return matched, nil
}

func patchDownloadLinks(m map[string]any, ghProxy string, dryRun bool) (found bool) {
	found = false

	if url, ok := m["url"]; ok && downloadLinkIsGitHub(url) {
		found = true
		if !dryRun && patchURLValue(&url, ghProxy) {
			m["url"] = url
		}
	}

	if arch, ok := m["architecture"].(map[string]any); ok {
		for _, archVal := range arch {
			av, ok := archVal.(map[string]any)
			if !ok {
				continue
			}

			if url, ok := av["url"]; ok && downloadLinkIsGitHub(url) {
				found = true
				if !dryRun && patchURLValue(&url, ghProxy) {
					av["url"] = url
				}
			}
		}
	}

	return found
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
	case []string:
		for _, s := range u {
			if strings.HasPrefix(s, githubURLPrefix) {
				return true
			}
		}
	}
	return false
}
