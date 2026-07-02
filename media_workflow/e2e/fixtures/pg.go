//go:build e2e

// Package fixtures 提供 media_workflow e2e 测试的资源 fixture（testcontainers pg）。
//
// 资源管理范式（harness/contributing/e2e.md）：每个资源一个 NewXxx(t) + t.Cleanup teardown。
// 重资源（pg 容器）由 TestMain 进程级共享，各测试用独立 database 隔离。
package fixtures

import (
	"context"
	"database/sql"
	"fmt"
	"sync/atomic"
	"testing"

	_ "github.com/lib/pq"
	"github.com/testcontainers/testcontainers-go"
	tcpq "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	gpg "gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// sharedPG 是 TestMain 起的进程级共享 pg 容器。
var sharedPG *Postgres

// SetSharedPG 由 TestMain 注入共享容器。
func SetSharedPG(p *Postgres) { sharedPG = p }

// Postgres 是共享 pg 容器句柄。
type Postgres struct {
	Container *tcpq.PostgresContainer
	RootDSN   string
}

// StartSharedPG 起一个 pg 容器供整个 e2e 进程共享。
func StartSharedPG(ctx context.Context) (*Postgres, error) {
	c, err := tcpq.RunContainer(ctx,
		tcpq.WithDatabase("media_workflow_e2e"),
		tcpq.WithUsername("media_workflow"),
		tcpq.WithPassword("media_workflow"),
		testcontainers.WithWaitStrategy(wait.ForLog("database system is ready to accept connections").WithOccurrence(2)),
	)
	if err != nil {
		return nil, err
	}
	dsn, err := c.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		_ = c.Terminate(ctx)
		return nil, err
	}
	return &Postgres{Container: c, RootDSN: dsn}, nil
}

// Terminate 销毁共享容器。
func (p *Postgres) Terminate(ctx context.Context) error { return p.Container.Terminate(ctx) }

var dbSeq uint64

// Database 是单个测试专属的隔离 database。
type Database struct {
	Gorm *gorm.DB
	DSN  string
	name string
}

// NewDatabase 在共享 pg 里创建独立 database。测试结束 t.Cleanup 自动删。
func NewDatabase(t *testing.T) *Database {
	t.Helper()
	if sharedPG == nil {
		t.Skip("shared pg not started (need docker)")
	}
	ctx := context.Background()
	name := fmt.Sprintf("e2e_%d", atomic.AddUint64(&dbSeq, 1))

	root, err := sql.Open("postgres", sharedPG.RootDSN)
	if err != nil {
		t.Fatalf("open root: %v", err)
	}
	if _, err := root.ExecContext(ctx, fmt.Sprintf("CREATE DATABASE %s", name)); err != nil {
		_ = root.Close()
		t.Fatalf("create db %s: %v", name, err)
	}
	root.Close()
	t.Cleanup(func() {
		c, _ := sql.Open("postgres", sharedPG.RootDSN)
		defer c.Close()
		_, _ = c.ExecContext(context.Background(), fmt.Sprintf("DROP DATABASE IF EXISTS %s", name))
	})

	dsn := withDBName(sharedPG.RootDSN, name)
	db, err := gorm.Open(gpg.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open gorm: %v", err)
	}
	t.Cleanup(func() { sqlDB, _ := db.DB(); _ = sqlDB.Close() })
	return &Database{Gorm: db, DSN: dsn, name: name}
}

// withDBName 替换 dsn 里的库名（postgres://user:pass@host:port/db?...）。
func withDBName(dsn, name string) string {
	for i := len(dsn) - 1; i >= 0; i-- {
		if dsn[i] == '/' {
			rest := dsn[i+1:]
			if q := indexByte(rest, '?'); q >= 0 {
				return dsn[:i+1] + name + rest[q:]
			}
			return dsn[:i+1] + name
		}
	}
	return dsn
}

func indexByte(s string, b byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == b {
			return i
		}
	}
	return -1
}
