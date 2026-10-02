package main

import (
	"encoding/json"
	"os"
	"strings"
)

const githubURLPrefix = "https://github.com"

// tryPatchManifest 在 mainfest 中匹配要修改下载链接，若匹配且 dryRun 为 false 则修改
func tryPatchManifest(item *SetCommandItem, ghProxy string, dryRun bool) (matched bool, err error) {
	data, err := os.ReadFile(item.Manifest) // 读取 manifest 文件
	if err != nil {
		item.Status = ManifestError
		return false, err
	}

	var manifestContent map[string]any
	if err = json.Unmarshal(data, &manifestContent); err != nil { // 解析 json
		item.Status = ManifestError
		return false, err
	}

	if matched = findAndPatchURL(manifestContent, ghProxy, dryRun); !matched {
		item.Status = NotGitHub // 没匹配上不会修改，没有“新的内容”需要返回
	} else {
		item.Status = IsGitHub
		if !dryRun { // 找到了，又不是 dryRun，必须修改，所以保存修改后的结果，这样可以传出去
			item.ChangedManifest = manifestContent
		}
	}

	return matched, nil
}

// findAndPatchURL 匹配要修改下载链接，若匹配且 dryRun 为 false 则修改
func findAndPatchURL(content map[string]any, ghProxy string, dryRun bool) (found bool) {
	found = false

	// architecture 是一级节点，二级节点是 x64，arm64 之类的，三级节点才是 url 元素
	if arch, ok := content["architecture"].(map[string]any); ok {
		for _, archVal := range arch { // 二级节点有多个，所以要遍历一下
			secondLevelElement, ok := archVal.(map[string]any)
			if !ok {
				continue
			}

			// 二级节点下可以有多个三级节点，但只能有一个 url
			if url, ok := secondLevelElement["url"]; ok && urlIsMatched(url) {
				found = true
				if !dryRun && patchURL(&url, ghProxy) {
					secondLevelElement["url"] = url
				}
			}
		}
	}

	// Scoop 逻辑：只要 architecture 存在，就按当前系统架构读取架构块内的 url，顶层 url 直接无视
	if found {
		return true
	}

	// url 元素出现在一级节点。注意，url 元素的值可能是 string，也可能是 []string
	if url, ok := content["url"]; ok && urlIsMatched(url) {
		found = true
		if !dryRun && patchURL(&url, ghProxy) {
			content["url"] = url
		}
	}

	return found
}

// patchURL 就地修改 url 值（string 或 []string），结果写回 *v
func patchURL(v *any, ghProxy string) bool {
	cur := *v
	switch u := cur.(type) {
	case string:
		changed := patchDownloadLink(&u, ghProxy)
		if changed {
			*v = u
		}
		return changed
	case []any:
		changed := false
		for i, item := range u {
			if s, ok := item.(string); ok {
				if patchDownloadLink(&s, ghProxy) {
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

// patchDownloadLink 修改单个 url 字符串，返回是否实际修改
func patchDownloadLink(s *string, ghProxy string) bool {
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

// urlIsMatched 判断给定的对象是否匹配修改规则
func urlIsMatched(v any) bool {
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
