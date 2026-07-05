// media_auth 的 GORM Gen 代码生成器入口。
//
// 从 harness/tools/gormgen/main.go 模板复制，改 postgres driver + 本服务 model 列表。
// 生成 biz/dal/query/（🔴 禁改，schema 变更 → 重跑本工具）。
//
// 用法（需连真实 pg，schema 由 migrations 定义）：
//
//	MEDIA_AUTH_DB_DSN=postgres://media_auth:media_auth@127.0.0.1:5432/media_auth?sslmode=disable \
//	  go run tools/gormgen/main.go
//
// 或：make gorm-gen（Makefile 已配 DSN 从环境变量读）
//
// schema 变更流程（architecture.md「数据库 Migration」）：
// 加 migration SQL → 执行迁移 → 更新 model 列表（本文件 ApplyBasic）→ 重跑本工具。
package main

import (
	"log"
	"os"

	"gorm.io/driver/postgres"
	"gorm.io/gen"
	"gorm.io/gorm"

	"media_agent/media_auth/biz/dal/model"
)

func main() {
	dsn := os.Getenv("MEDIA_AUTH_DB_DSN")
	if dsn == "" {
		log.Fatal("MEDIA_AUTH_DB_DSN is required for GORM Gen")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("open postgres: %v", err)
	}
	g := gen.NewGenerator(gen.Config{
		OutPath:      "biz/dal/query",
		ModelPkgPath: "biz/dal/model",
		Mode:         gen.WithDefaultQuery | gen.WithQueryInterface,
	})
	g.UseDB(db)
	// 列出本服务所有需要生成 query 的 row model。
	// 新增表 → 加 model + 在此 ApplyBasic → 重跑。
	g.ApplyBasic(
		model.User{},
		model.UserIdentity{},
		model.ServiceToken{},
		model.OAuthClient{},
		model.OAuthAuthorizationCode{},
		model.OAuthRefreshToken{},
		model.McpOAuthClient{},
		model.McpOAuthToken{},
		model.Team{},
		model.TeamMember{},
		model.TeamInvite{},
	)
	g.Execute()
}
