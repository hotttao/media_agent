// Package dal 存放数据库访问：db.go（初始化）、migrate.go（迁移）、transaction.go（事务抽象）、
// model/（GORM row model）、query/（GORM Gen 生成）、repo/（domain 仓储实现）。
//
// 禁止 import biz/handler、hertz_gen/model；不得把 GORM row model 返回给 service/workflow。
// service/workflow/policy/domain 不得 import biz/dal/query 或 biz/dal/model。
//
// 事务约定（harness/contributing/architecture.md「事务 TxRunner + ctx 传递」）：
//   - 事务统一由 TxRunner.RunInTx 开启（单 domain 用例在 service 顶部、跨 domain 在 workflow 顶部）；
//   - tx 经 context.Context 隐式传递，repo 用 FromContext(ctx, db) 取连接；
//   - service 内部方法 / repo 不开事务，只透传 ctx。
//
// 驱动：postgres（control-plane 现状 pg，spec §7）。
package dal

import (
	"fmt"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	configpb "media_agent/hertz_gen/config"
)

// NewDB 根据 DatabaseConfig 构造 *gorm.DB。
//
// database.enabled=false 时返回 (nil, nil)——调用方可无条件初始化，禁用时不阻断启动。
// 当前 driver 仅支持 postgres；新增 mysql/sqlite 在此分支扩展。
func NewDB(cfg *configpb.DatabaseConfig) (*gorm.DB, error) {
	return newDB(cfg, gormlogger.Default.LogMode(gormlogger.Warn), false)
}

// NewMaintenanceDB disables SQL logging for sensitive offline transfers.
// The tool must also avoid printing the returned infrastructure error.
func NewMaintenanceDB(cfg *configpb.DatabaseConfig) (*gorm.DB, error) {
	return newDB(cfg, gormlogger.Default.LogMode(gormlogger.Silent), true)
}

func newDB(cfg *configpb.DatabaseConfig, logger gormlogger.Interface, disablePing bool) (*gorm.DB, error) {
	if cfg == nil || !cfg.GetEnabled() {
		return nil, nil
	}
	switch cfg.GetDriver() {
	case "postgres", "": // 默认 postgres
		db, err := gorm.Open(postgres.Open(cfg.GetDsn()), &gorm.Config{
			Logger:               logger,
			DisableAutomaticPing: disablePing,
			TranslateError:       true, // 把底层唯一/外键冲突翻译为 gorm.ErrDuplicatedKey/ErrForeignKeyViolation，便于 repo 用 errors.Is 判定
		})
		if err != nil {
			return nil, fmt.Errorf("dal: open postgres: %w", err)
		}
		sqlDB, err := db.DB()
		if err != nil {
			return nil, fmt.Errorf("dal: get *sql.DB: %w", err)
		}
		if n := int(cfg.GetMaxOpenConns()); n > 0 {
			sqlDB.SetMaxOpenConns(n)
		}
		if n := int(cfg.GetMaxIdleConns()); n > 0 {
			sqlDB.SetMaxIdleConns(n)
		}
		if s := cfg.GetConnMaxLifetimeSeconds(); s > 0 {
			sqlDB.SetConnMaxLifetime(time.Duration(s) * time.Second)
		}
		return db, nil
	default:
		return nil, fmt.Errorf("dal: unsupported database driver %q", cfg.GetDriver())
	}
}
