package main

import (
	"errors"
	"flag"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"
)

type config struct {
	sourceDSN         string
	sourceAssetsURL   string
	targetURL         string
	organizationID    string
	scope             string
	only              string
	sessionCookieName string
	sessionCookie     string
	bearerToken       string
	dryRun            bool
	requestTimeout    time.Duration
}

func parseConfig(args []string) (config, error) {
	fs := flag.NewFlagSet("media_ai_migrate", flag.ContinueOnError)
	var cfg config
	fs.StringVar(&cfg.sourceDSN, "source-dsn", env("MEDIA_AI_DATABASE_URL", ""), "旧 media_ai MySQL DSN（支持 mysql:// URL）")
	fs.StringVar(&cfg.sourceAssetsURL, "source-assets-url", env("MEDIA_AI_ASSETS_URL", "http://192.168.2.38"), "旧图片服务根地址")
	fs.StringVar(&cfg.targetURL, "target-url", env("MEDIA_MATERIALS_URL", "http://127.0.0.1:18080"), "目标网关地址")
	fs.StringVar(&cfg.organizationID, "organization-id", env("MEDIA_MATERIALS_ORGANIZATION_ID", ""), "目标公司 ID")
	fs.StringVar(&cfg.scope, "scope", env("MEDIA_MATERIALS_SCOPE", "ai_clothing"), "目标业务 scope")
	fs.StringVar(&cfg.only, "only", "all", "迁移范围：all、material、virtual-ip、product")
	fs.StringVar(&cfg.sessionCookieName, "session-cookie-name", env("MEDIA_MATERIALS_SESSION_COOKIE_NAME", "ory_kratos_session"), "目标会话 cookie 名")
	fs.StringVar(&cfg.sessionCookie, "session-cookie", env("MEDIA_E2E_KRATOS_SESSION_COOKIE", ""), "ory_kratos_session cookie 值")
	fs.StringVar(&cfg.bearerToken, "bearer-token", env("MEDIA_MATERIALS_BEARER_TOKEN", ""), "目标 API Bearer token")
	fs.BoolVar(&cfg.dryRun, "dry-run", false, "只读取并校验源数据，不调用目标 API")
	fs.DurationVar(&cfg.requestTimeout, "request-timeout", 60*time.Second, "单次 HTTP 请求超时")
	if err := fs.Parse(args); err != nil {
		return config{}, err
	}

	missing := make([]string, 0, 2)
	if strings.TrimSpace(cfg.sourceDSN) == "" {
		missing = append(missing, "--source-dsn/MEDIA_AI_DATABASE_URL")
	}
	if !cfg.dryRun && strings.TrimSpace(cfg.organizationID) == "" {
		missing = append(missing, "--organization-id/MEDIA_MATERIALS_ORGANIZATION_ID")
	}
	if len(missing) > 0 {
		return config{}, fmt.Errorf("缺少必填参数: %s", strings.Join(missing, ", "))
	}
	if !cfg.dryRun && cfg.sessionCookie == "" && cfg.bearerToken == "" {
		return config{}, errors.New("需要 --session-cookie 或 --bearer-token 访问目标 API")
	}
	if strings.TrimSpace(cfg.sessionCookieName) == "" {
		return config{}, errors.New("session-cookie-name 不能为空")
	}
	switch cfg.only {
	case "all", "material", "virtual-ip", "product":
	default:
		return config{}, fmt.Errorf("未知迁移范围: %q", cfg.only)
	}
	for name, raw := range map[string]string{"source-assets-url": cfg.sourceAssetsURL, "target-url": cfg.targetURL} {
		parsed, err := url.ParseRequestURI(strings.TrimRight(raw, "/"))
		if err != nil || parsed.Scheme == "" || parsed.Host == "" {
			return config{}, fmt.Errorf("%s 不是有效 URL: %q", name, raw)
		}
	}
	cfg.sourceAssetsURL = strings.TrimRight(cfg.sourceAssetsURL, "/")
	cfg.targetURL = strings.TrimRight(cfg.targetURL, "/")
	return cfg, nil
}

func env(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
