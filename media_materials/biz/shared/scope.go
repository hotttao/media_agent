package shared

// supportedScopes 当前部署支持的 scope 闭集合。
// scope 降为 shared 值对象（见 docs/ddd/media_materials/domain/0001-scope-del/scope.md），
// 由 service 层在资源创建时调 IsKnownScope 校验。
//
// TODO: 改为启动期从 config（yaml / Consul KV）加载 supported_scopes；当前硬编码常量，
// 后续 config proto 加 supported_scopes 字段后替换。
var supportedScopes = map[string]bool{
	"ai_clothing": true,
	"ai_guoxue":   true,
}

// IsKnownScope 校验 scope 非空且在 supportedScopes 闭集合内。
// 跨 service 从同一份常量，结果一致（不暴露 RPC）。
func IsKnownScope(s string) bool {
	return supportedScopes[s]
}

// IsSameScope 严格相等。material_bindings INSERT 前由 service 调，校验两端同 scope。
func IsSameScope(a, b string) bool { return a == b }
