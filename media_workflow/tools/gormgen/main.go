// media_workflow 的 GORM Gen 代码生成器入口。
//
//	PG → biz/dal/query/  🔴 禁改（生成代码）
//
// model 包 biz/dal/model/ 手写，gen 只读取结构生成 query。
//
// 用法（需连真实 DB，schema 由 migrations/ 定义）：
//
//	MEDIA_WORKFLOW_DB_DSN=postgres://media_auth:media_auth@127.0.0.1:5432/media_workflow?sslmode=disable \
//	  go run tools/gormgen/main.go
//
// 或：make gorm-gen
package main

import (
	"log"
	"os"

	"gorm.io/driver/postgres"
	"gorm.io/gen"
	"gorm.io/gorm"

	"media_agent/media_workflow/biz/dal/model"
)

func main() {
	pgDSN := os.Getenv("MEDIA_WORKFLOW_DB_DSN")
	if pgDSN == "" {
		log.Fatal("MEDIA_WORKFLOW_DB_DSN is required for GORM Gen (PG)")
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
	g.ApplyBasic(
		model.Template{},
		model.TemplateInput{},
		model.TemplateVersion{},
		model.TemplateGrant{},
		model.Workflow{},
		model.WorkflowStep{},
		model.StepBinding{},
		model.WorkflowGrant{},
		model.Job{},
		model.StepRun{},
	)
	g.Execute()
}
