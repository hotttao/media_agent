package testpostgres

import (
	"context"
	"database/sql"
	"testing"
)

func TestUnsafeIsolatedDatabaseConfigurationCannotReachSQL(t *testing.T) {
	for _, tc := range []struct{ dsn, prefix string }{
		{"http://localhost/postgres", "workflow"},
		{"postgres:///postgres", "workflow"},
		{"postgres://localhost/postgres", `workflow"; DROP DATABASE production; --`},
		{"postgres://localhost/postgres", ""},
		{"postgres://localhost/postgres?dbname=production", "workflow"},
		{"postgres://localhost/postgres?database=production", "workflow"},
		{"postgres://localhost/postgres?service=production", "workflow"},
	} {
		// An unusable pool would panic if validation allowed an SQL operation.
		dsn, cleanup, err := Create(context.Background(), &sql.DB{}, tc.dsn, tc.prefix)
		if err == nil || dsn != "" || cleanup != nil {
			t.Fatal("unsafe configuration admitted")
		}
	}
}
