// Package testpostgres creates disposable databases on an explicitly supplied
// PostgreSQL server. It never migrates or drops the administrative database.
package testpostgres

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"net/url"
	"regexp"
)

var prefixPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,19}$`)

// Create returns a new database DSN and its exact cleanup operation. Callers
// must close every test connection before cleanup, then close the admin pool.
// Errors deliberately omit credentials, DSNs and server response bodies.
func Create(ctx context.Context, admin *sql.DB, adminDSN, prefix string) (string, func(context.Context) error, error) {
	u, err := url.Parse(adminDSN)
	if err != nil || u == nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") || u.Host == "" || !prefixPattern.MatchString(prefix) || admin == nil {
		return "", nil, errors.New("invalid isolated PostgreSQL configuration")
	}
	// libpq/pgx query overrides or a service file must never redirect the
	// returned connection back to an existing administrative/business DB.
	query := u.Query()
	for _, key := range []string{"dbname", "database", "service", "servicefile"} {
		if query.Has(key) {
			return "", nil, errors.New("database overrides are not allowed in isolated configuration")
		}
	}
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return "", nil, errors.New("isolated database identity unavailable")
	}
	name := "e2e_" + prefix + "_" + hex.EncodeToString(id[:])
	// All identifier characters are generated or strictly validated above.
	quoted := `"` + name + `"`
	if _, err := admin.ExecContext(ctx, "CREATE DATABASE "+quoted); err != nil {
		return "", nil, errors.New("isolated database creation failed")
	}
	u.Path, u.RawPath = "/"+name, ""
	return u.String(), func(cleanupCtx context.Context) error {
		if _, err := admin.ExecContext(cleanupCtx, "DROP DATABASE "+quoted); err != nil {
			return errors.New("isolated database cleanup failed")
		}
		return nil
	}, nil
}
