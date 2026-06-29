// migrate.go：启动期数据库迁移（golang-migrate）。
//
// 迁移 SQL 放在 <app>/migrations/（项目根目录），运行时从磁盘读取（不 embed）。
// 故部署时需带上 migrations 目录；或 prod 用 `migrate` CLI 单独跑（migrate_on_start=false）。
//
// main.go 在 NewDB 后、NewServerSuite 前调 Migrate(dir, cfg)——幂等：
// 已最新则 no-op，落后则执行到最新。由 database.migrate_on_start 门控。
//
// Migrate 用独立 *sql.DB（按 cfg.dsn 新建）执行迁移，与业务 *gorm.DB 隔离——
// golang-migrate 的 m.Close() 会关闭其持有的 database driver，若复用 gorm 的 *sql.DB
// 会导致业务连接被关。故此处独立开连接、独立关。
//
// 与 GORM Gen 的关系（architecture.md「数据库 Migration」）：
// schema 变更 = 加 migration SQL → 执行迁移 → 重跑 GORM Gen。
// migration 定义 schema，GORM Gen 生成查询，二者各司其职。
package dal

import (
	"database/sql"
	"fmt"
	"os"

	_ "github.com/go-sql-driver/mysql" // register mysql driver for database/sql
	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/mysql"
	"github.com/golang-migrate/migrate/v4/source/iofs"

	configpb "media_agent/hertz_gen/config"
)

// Migrate 执行数据库迁移到最新版本。
//
// migrationsDir 是迁移 SQL 所在目录路径（运行时从磁盘读，不 embed）；
// cfg 提供 dsn 与 migrate_on_start 门控。Migrate 按 dsn 新建独立 *sql.DB，
// 迁移完成后关闭，不影响业务 *gorm.DB。
// 已是最新版本时 migrate.Up 返回 migrate.ErrNoChange，视为成功（幂等 no-op）。
func Migrate(migrationsDir string, cfg *configpb.DatabaseConfig) error {
	if cfg == nil || !cfg.GetMigrateOnStart() {
		return nil
	}
	dsn := cfg.GetDsn()
	if dsn == "" {
		return fmt.Errorf("dal: migrate: empty dsn")
	}
	if migrationsDir == "" {
		return fmt.Errorf("dal: migrate: empty migrations dir")
	}
	if _, err := os.Stat(migrationsDir); err != nil {
		return fmt.Errorf("dal: migrate dir %q: %w", migrationsDir, err)
	}

	// os.DirFS 返回 fs.FS，iofs 读取磁盘上的 .sql 文件。
	src, err := iofs.New(os.DirFS(migrationsDir), ".")
	if err != nil {
		return fmt.Errorf("dal: migrate source: %w", err)
	}

	// 独立 *sql.DB：migrate 用完即关，不波及业务连接。
	sqlDB, err := sql.Open("mysql", dsn)
	if err != nil {
		return fmt.Errorf("dal: open sql for migrate: %w", err)
	}
	defer sqlDB.Close()

	dbDriver, err := mysql.WithInstance(sqlDB, &mysql.Config{})
	if err != nil {
		return fmt.Errorf("dal: mysql migrate driver: %w", err)
	}

	m, err := migrate.NewWithInstance("iofs", src, "mysql", dbDriver)
	if err != nil {
		return fmt.Errorf("dal: new migrate: %w", err)
	}
	defer m.Close()

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("dal: migrate up: %w", err)
	}
	return nil
}
