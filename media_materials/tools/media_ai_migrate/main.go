package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
)

func main() {
	if err := run(context.Background(), os.Args[1:]); err != nil {
		log.Printf("迁移失败: %v", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	cfg, err := parseConfig(args)
	if err != nil {
		return err
	}
	db, err := openSource(ctx, cfg.sourceDSN)
	if err != nil {
		return err
	}
	defer db.Close()
	data, err := loadSource(ctx, db)
	if err != nil {
		return err
	}
	logger := log.New(os.Stdout, "", log.LstdFlags)
	logger.Printf("源数据: materials=%d virtual_ips=%d products=%d product_images=%d", len(data.Materials), len(data.VirtualIPs), len(data.Products), productImageCount(data.Products))
	client := &http.Client{Timeout: cfg.requestTimeout}
	m := &migrator{
		cfg: cfg, sourceHTTP: client, logger: logger,
		target: &targetClient{baseURL: cfg.targetURL, organizationID: cfg.organizationID, scope: cfg.scope, sessionCookieName: cfg.sessionCookieName, sessionCookie: cfg.sessionCookie, bearerToken: cfg.bearerToken, http: client},
	}
	runErr := m.run(ctx, data)
	logger.Printf("结果: materials=%s virtual_ips=%s products=%s images=%s", formatStats(m.stats.Materials), formatStats(m.stats.VirtualIPs), formatStats(m.stats.Products), formatStats(m.stats.Images))
	return runErr
}

func formatStats(stats resourceStats) string {
	return fmt.Sprintf("created:%d updated:%d skipped:%d failed:%d", stats.Created, stats.Updated, stats.Skipped, stats.Failed)
}
