// media_crawler 的 GORM Gen 代码生成器入口（双 DB）。
//
// 复制自 harness/tools/gormgen/main.go 模板，扩展为双 DB：
//
//	PG（jobs）           → biz/dal/query/      🔴 禁改
//	MySQL（xhs 只读）    → biz/dal/query_read/  🔴 禁改
//
// model 包共用（biz/dal/model/，手写，gen 不覆盖，只读取结构生成 query）。
//
// 用法（需连真实 DB，schema 由 migrations/ + 爬虫 schema/tables.sql 定义）：
//
//	MEDIA_CRAWLER_DB_DSN=postgres://media_auth:media_auth@127.0.0.1:5432/media-crawler?sslmode=disable \
//	CRAWL_DATA_DSN=root:123456@tcp(127.0.0.1:13306)/media_crawler_pro?charset=utf8mb4 \
//	  go run tools/gormgen/main.go
//
// 或：make gorm-gen（Makefile 从环境变量读 DSN）
//
// schema 变更流程（architecture.md「数据库 Migration」）：
//
//	PG：加 migration SQL → 执行迁移 → 更新 model → 重跑本工具
//	MySQL：爬虫改表 → 更新 model → 重跑本工具（media_crawler 不维护爬虫 schema）
package main

import (
	"log"
	"os"

	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gen"
	"gorm.io/gorm"

	"media_agent/media_crawler/biz/dal/model"
)

func main() {
	// ---- PG: jobs ----
	pgDSN := os.Getenv("MEDIA_CRAWLER_DB_DSN")
	if pgDSN == "" {
		log.Fatal("MEDIA_CRAWLER_DB_DSN is required for GORM Gen (PG)")
	}
	pgDB, err := gorm.Open(postgres.Open(pgDSN), &gorm.Config{})
	if err != nil {
		log.Fatalf("open postgres: %v", err)
	}
	pgGen := gen.NewGenerator(gen.Config{
		OutPath:      "biz/dal/query",
		ModelPkgPath: "biz/dal/model",
		Mode:         gen.WithDefaultQuery | gen.WithQueryInterface,
	})
	pgGen.UseDB(pgDB)
	pgGen.ApplyBasic(model.Job{})
	pgGen.Execute()

	// ---- MySQL: xhs 只读 ----
	mysqlDSN := os.Getenv("CRAWL_DATA_DSN")
	if mysqlDSN == "" {
		log.Fatal("CRAWL_DATA_DSN is required for GORM Gen (MySQL)")
	}
	mysqlDB, err := gorm.Open(mysql.Open(mysqlDSN), &gorm.Config{})
	if err != nil {
		log.Fatalf("open mysql: %v", err)
	}
	mysqlGen := gen.NewGenerator(gen.Config{
		OutPath:      "biz/dal/query_read",
		ModelPkgPath: "biz/dal/model",
		Mode:         gen.WithDefaultQuery | gen.WithQueryInterface,
	})
	mysqlGen.UseDB(mysqlDB)
	mysqlGen.ApplyBasic(model.XhsNote{}, model.XhsComment{})
	mysqlGen.Execute()
}
