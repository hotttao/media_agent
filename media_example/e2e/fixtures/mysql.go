// Package fixtures 提供 e2e 测试的资源 fixture（setup + t.Cleanup teardown）。
//
// 资源管理范式（harness/contributing/e2e.md「资源管理」）：
// 每个资源一个 NewXxx(t) 函数，setup 资源并用 t.Cleanup 注册 teardown；
// 测试只声明「需要什么资源」，不写 cleanup。teardown 按 t.Cleanup 的 LIFO 自动执行。
//
// 重资源（MySQL 容器）由 TestMain 进程级共享一次（见 e2e_test.go TestMain），
// 各测试用独立 database/schema 隔离（NewDatabase），避免互相干扰，又省容器启动开销。
package fixtures

import (
	"context"
	"database/sql"
	"fmt"
	"sync/atomic"
	"testing"

	_ "github.com/go-sql-driver/mysql" // register mysql driver
	tcmysql "github.com/testcontainers/testcontainers-go/modules/mysql"
	gmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// sharedMySQL 是 TestMain 起的进程级共享 MySQL 容器。
// 各测试不直接用它，而是经 NewDatabase 派生独立 database。
var sharedMySQL *MySQL

// SetSharedMySQL 由 TestMain 调用，注入进程级共享 MySQL 容器句柄。
func SetSharedMySQL(m *MySQL) { sharedMySQL = m }

// MySQL 是共享的 MySQL 容器句柄。
type MySQL struct {
	Container *tcmysql.MySQLContainer
	RootDSN   string // root DSN，用于创建/删除测试 database
}

// StartSharedMySQL 起一个 MySQL 容器，供整个 e2e 进程共享。
// 由 TestMain 调用一次；无 docker 返回 error（TestMain 据 skip）。
func StartSharedMySQL(ctx context.Context) (*MySQL, error) {
	c, err := tcmysql.RunContainer(ctx,
		tcmysql.WithDatabase("media_example_e2e"),
	)
	if err != nil {
		return nil, err
	}
	// 默认 DSN 用 test 用户（无 CREATE DATABASE 权限）。
	// root DSN：testcontainers mysql 镜像 root 密码同 MYSQL_PASSWORD（默认 "test"），
	// 用 root 连接才能 CREATE/DROP 各测试的隔离 database。
	testDSN, err := c.ConnectionString(ctx, "charset=utf8mb4", "parseTime=True", "loc=Local")
	if err != nil {
		_ = c.Terminate(ctx)
		return nil, err
	}
	return &MySQL{Container: c, RootDSN: withUser(testDSN, "root")}, nil
}

// withUser 把 dsn 里的用户名替换为 user（dsn 形如 user:pass@tcp(host:port)/db?params）。
func withUser(dsn, user string) string {
	colon := -1
	at := -1
	for i := 0; i < len(dsn); i++ {
		if dsn[i] == ':' && colon < 0 {
			colon = i
		}
		if dsn[i] == '@' {
			at = i
			break
		}
	}
	if colon < 0 || at < 0 {
		return dsn
	}
	// user + :pass@tcp...（colon..at 含 ":pass@"）
	return user + dsn[colon:at] + dsn[at:]
}

// Terminate 销毁共享容器。由 TestMain 在 m.Run() 后调用。
func (m *MySQL) Terminate(ctx context.Context) error {
	return m.Container.Terminate(ctx)
}

var dbSeq uint64 // 测试 database 自增序号，保证名称唯一

// Database 是单个测试专属的隔离 database（含 gorm 连接 + dsn）。
type Database struct {
	Gorm *gorm.DB
	DSN  string
	name string
}

// NewDatabase 在共享 MySQL 容器里创建一个独立 database 供当前测试用。
// 测试结束 t.Cleanup 自动删 database；容器不销毁（容器归 TestMain 管）。
//
// 隔离策略：每测试一个独立 database，互不影响；容器只起一次（TestMain），省启动开销。
func NewDatabase(t *testing.T) *Database {
	t.Helper()
	if sharedMySQL == nil {
		t.Skip("shared mysql not started (need docker)")
	}
	ctx := context.Background()

	name := fmt.Sprintf("e2e_%d", atomic.AddUint64(&dbSeq, 1))
	// 用 root 连接创建 database。
	root, err := sql.Open("mysql", sharedMySQL.RootDSN)
	if err != nil {
		t.Fatalf("open root conn: %v", err)
	}
	if _, err := root.ExecContext(ctx, fmt.Sprintf("CREATE DATABASE `%s`", name)); err != nil {
		_ = root.Close()
		t.Fatalf("create database %s: %v", name, err)
	}
	root.Close()
	t.Cleanup(func() {
		// 测试结束删 database（独立连接，避免占用 gorm 连接池）。
		cleanup, err := sql.Open("mysql", sharedMySQL.RootDSN)
		if err != nil {
			return
		}
		defer cleanup.Close()
		_, _ = cleanup.ExecContext(context.Background(), fmt.Sprintf("DROP DATABASE IF EXISTS `%s`", name))
	})

	// 该 database 的 dsn：在 rootDSN 末尾替换 / 后的库名。
	dsn := withDBName(sharedMySQL.RootDSN, name)
	db, err := gorm.Open(gmysql.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open gorm for %s: %v", name, err)
	}
	t.Cleanup(func() {
		sqlDB, _ := db.DB()
		_ = sqlDB.Close()
	})
	return &Database{Gorm: db, DSN: dsn, name: name}
}

// withDBName 把 dsn 里的库名替换为 name（dsn 形如 user:pass@tcp(host:port)/db?params）。
func withDBName(dsn, name string) string {
	for i := len(dsn) - 1; i >= 0; i-- {
		if dsn[i] == '/' {
			// 找 / 之后到 ? 之前的部分替换。
			rest := dsn[i+1:]
			if q := indexOf(rest, '?'); q >= 0 {
				return dsn[:i+1] + name + rest[q:]
			}
			return dsn[:i+1] + name
		}
	}
	return dsn
}

func indexOf(s string, b byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == b {
			return i
		}
	}
	return -1
}
