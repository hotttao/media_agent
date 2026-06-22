// Package config 提供 hertz 服务共享的配置加载能力。
//
// 加载链路（已移除 koanf 依赖，改用 yaml.v3 + fsnotify + 手写 deep-merge）：
//
//	.env (godotenv)
//	  → APP_ENV
//	  → conf/<env>.yaml
//	  → expandEnv (shell ${VAR:-default}, isEnvName 过滤 accesslog 模板变量)
//	  → yaml.v3 unmarshal → map[string]any
//	  → [可选] Consul KV JSON deep-merge
//	  → json.Marshal → protojson.Unmarshal → *configpb.Config
//	  → fsnotify 文件 watch / Consul KV blocking-query watch 触发 reload
//
// 命名规则单一：YAML 里写什么 env 变量名就用什么（MYSQL_DSN / SERVER_ADDR / APP_ENV）。
// 不再有 `MEDIA_EXAMPLE_*` 前缀覆盖路径。
package config

import (
	"os"
	"strings"
)

// expandEnv 实现 shell 风格 ${VAR} 与 ${VAR:-default} 插值。
//
// os.ExpandEnv 只识别 $VAR / ${VAR}，不识别 :- 默认值语法；
// 这里通过 os.Expand + 自定义 mapper 补齐。
//
// 行为：
//   - ${VAR}           → os.Getenv("VAR")，未设置为空串
//   - ${VAR:-default}  → VAR 非空取 VAR，否则取 default
//   - $VAR             → 同 ${VAR}
//
// 仅对大写 / 下划线开头的 env 变量名生效（如 ${APP_ENV}、${SERVER_ADDR}）；
// 小写 / 混合大小写的 ${...}（如 access log 模板里的 ${time}、${method}、${path}）
// 原样保留，不当作 env 变量展开。这避免 access log format 模板被误展开。
//
// default 字面量不再做二次展开，避免递归失控。
func expandEnv(input string) string {
	return os.Expand(input, func(name string) string {
		// name 形如 "VAR" 或 "VAR:-default"。
		key := name
		def := ""
		hasDefault := false
		if idx := strings.Index(name, ":-"); idx >= 0 {
			key = name[:idx]
			def = name[idx+2:]
			hasDefault = true
		}
		// 仅大写字母 + 下划线 + 数字视为 env 变量名。
		if !isEnvName(key) {
			// 原样保留 ${...}，让下游 YAML / 模板引擎处理。
			return "${" + name + "}"
		}
		if v := os.Getenv(key); v != "" {
			return v
		}
		if hasDefault {
			return def
		}
		return ""
	})
}

// isEnvName 判断 name 是否符合 env 变量命名约定：
// 非空、首字符大写字母 / 下划线、其余大写字母 / 下划线 / 数字。
func isEnvName(name string) bool {
	if name == "" {
		return false
	}
	for i, r := range name {
		switch {
		case r == '_':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9' && i > 0:
		default:
			return false
		}
	}
	return true
}
