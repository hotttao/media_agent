// Package swaggerui 内嵌 swagger-ui 静态资源（go:embed），提供 Hertz handler。
//
// 替代已废弃的 hertz-contrib/swagger：本包直接 embed swagger-ui-dist 的运行必需文件
// （index.html / swagger-ui-bundle.js / swagger-ui.css / swagger-initializer.js 等），
// 由 serverhertz 注册到 /swagger/*any。spec（/openapi.yaml）由 serverhertz 单独提供，
// swagger-initializer.js 的 url 已在 fetch 阶段改为 "/openapi.yaml"。
//
// 升级 swagger-ui 版本：make swagger-ui-fetch（重新拉取并覆盖 dist/）。
package swaggerui

import (
	"context"
	"embed"
	"path"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
)

//go:embed dist
var distFS embed.FS

// StaticHandler 返回服务 swagger-ui 静态资源的 Hertz handler。
//
// 路由应挂在 "<prefix>/*any"（默认 /swagger/*any）。本 handler strip 该前缀后从
// embed dist 读文件；路径为空或 "/" 时返回 index.html（即 <prefix>/ 直接进 UI）。
// prefix 必须是路由挂载前缀（如 "/swagger"），不含通配。
func StaticHandler(prefix string) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		p := strings.TrimPrefix(string(c.Request.Path()), prefix)
		p = strings.TrimPrefix(p, "/")
		if p == "" {
			p = "index.html"
		}
		data, err := distFS.ReadFile("dist/" + p)
		if err != nil {
			c.AbortWithStatus(consts.StatusNotFound)
			return
		}
		c.Data(consts.StatusOK, contentType(p), data)
	}
}

// contentType 按扩展名返回 swagger-ui 静态资源的 Content-Type。
func contentType(name string) string {
	switch path.Ext(name) {
	case ".html":
		return "text/html; charset=utf-8"
	case ".js":
		return "application/javascript; charset=utf-8"
	case ".css":
		return "text/css; charset=utf-8"
	case ".png":
		return "image/png"
	case ".json":
		return "application/json; charset=utf-8"
	case ".yaml", ".yml":
		return "application/x-yaml"
	default:
		return "application/octet-stream"
	}
}
