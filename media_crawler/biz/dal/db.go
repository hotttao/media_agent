package dal

import (
	"fmt"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	configpb "media_agent/hertz_gen/config"
)

func NewDB(cfg *configpb.DatabaseConfig) (*gorm.DB, error) {
	if cfg == nil || !cfg.GetEnabled() {
		return nil, nil
	}
	switch cfg.GetDriver() {
	case "postgres", "":
		db, err := gorm.Open(postgres.Open(cfg.GetDsn()), &gorm.Config{
			Logger: gormlogger.Default.LogMode(gormlogger.Warn),
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

// NewCrawlDataDB 连爬虫 MySQL（只读，查内容数据）。独立于自有 PG。
// 仅只读查询，不开事务（TxRunner 仅 PG）；MySQL schema 由爬虫维护，不 migration。
func NewCrawlDataDB(cfg *configpb.CrawlDataConfig) (*gorm.DB, error) {
	if cfg == nil || !cfg.GetEnabled() {
		return nil, nil
	}
	db, err := gorm.Open(mysql.Open(cfg.GetDsn()), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Warn),
	})
	if err != nil {
		return nil, fmt.Errorf("dal: open mysql: %w", err)
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
	return db, nil
}
