package config

import (
	"google.golang.org/protobuf/encoding/protojson"
)

// deepMerge 把 src 深合并到 dst，返回新 map（不修改原 dst）。
//
// 规则：
//   - 两边都是 map → 递归合并
//   - 否则 src 覆盖 dst
//
// 用于多 Source 合并：KV 源（priority=20）覆盖文件源（priority=10）。
func deepMerge(dst, src map[string]any) map[string]any {
	out := make(map[string]any, len(dst))
	for k, v := range dst {
		out[k] = v
	}
	for k, sv := range src {
		if dv, ok := out[k]; ok {
			dm, dmOk := dv.(map[string]any)
			sm, smOk := sv.(map[string]any)
			if dmOk && smOk {
				out[k] = deepMerge(dm, sm)
				continue
			}
		}
		out[k] = sv
	}
	return out
}

// protojsonUnmarshalOptions 返回 protojson.UnmarshalOptions，DiscardUnknown=true
// 容忍 YAML 里有 proto 未定义的字段（向前兼容）。
func protojsonUnmarshalOptions() protojson.UnmarshalOptions {
	return protojson.UnmarshalOptions{DiscardUnknown: true}
}
