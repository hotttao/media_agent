package dal

import (
	"database/sql"
	"fmt"
	"os"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	_ "github.com/lib/pq"

	configpb "media_agent/hertz_gen/config"
)

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

	src, err := iofs.New(os.DirFS(migrationsDir), ".")
	if err != nil {
		return fmt.Errorf("dal: migrate source: %w", err)
	}

	sqlDB, err := sql.Open("postgres", dsn)
	if err != nil {
		return fmt.Errorf("dal: open sql for migrate: %w", err)
	}
	defer sqlDB.Close()

	dbDriver, err := postgres.WithInstance(sqlDB, &postgres.Config{})
	if err != nil {
		return fmt.Errorf("dal: postgres migrate driver: %w", err)
	}

	m, err := migrate.NewWithInstance("iofs", src, "postgres", dbDriver)
	if err != nil {
		return fmt.Errorf("dal: new migrate: %w", err)
	}
	defer m.Close()

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("dal: migrate up: %w", err)
	}
	return nil
}
