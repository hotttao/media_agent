// media_materials 的 GORM Gen 代码生成器入口。
//
// 复制自 harness/tools/gormgen/main.go 模板，单 PG DB。
//
//	PG → biz/dal/query/  🔴 禁改（生成代码）
//
// model 包 biz/dal/model/ 手写，gen 只读取结构生成 query。
//
// 用法（需连真实 DB，schema 由 migrations/ 定义）：
//
//	MEDIA_MATERIALS_DB_DSN=postgres://media_auth:media_auth@127.0.0.1:5432/media-materials?sslmode=disable \
//	  go run tools/gormgen/main.go
//
// 或：make gorm-gen（Makefile 从环境变量读 DSN）
//
// schema 变更流程（architecture.md「数据库 Migration」）：
//
//	加 migration SQL → 执行迁移 → 更新 model → 重跑本工具
package main

import (
	"log"
	"os"

	"gorm.io/driver/postgres"
	"gorm.io/gen"
	"gorm.io/gorm"

	"media_agent/media_materials/biz/dal/model"
)

func main() {
	pgDSN := os.Getenv("MEDIA_MATERIALS_DB_DSN")
	if pgDSN == "" {
		log.Fatal("MEDIA_MATERIALS_DB_DSN is required for GORM Gen (PG)")
	}
	db, err := gorm.Open(postgres.Open(pgDSN), &gorm.Config{})
	if err != nil {
		log.Fatalf("open postgres: %v", err)
	}
	g := gen.NewGenerator(gen.Config{
		OutPath:      "biz/dal/query",
		ModelPkgPath: "biz/dal/model",
		Mode:         gen.WithDefaultQuery | gen.WithQueryInterface,
	})
	g.UseDB(db)
	// 列出该服务所有需要生成 query 的 row model（手写 model 包里的全部表）
	g.ApplyBasic(
		model.AssetKind{},
		model.Material{},
		model.MaterialBinding{},
		model.GeneratedAsset{},
		model.Product{},
		model.VirtualIp{},
		model.VirtualIpGrant{},
	)
	g.Execute()
}
