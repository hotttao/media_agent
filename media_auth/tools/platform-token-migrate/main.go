// platform-token-migrate is an offline maintenance command. It defaults to
// rollback and never prints credentials, connection errors or SQL.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	config "media_agent/hertz_infra/config"
	"media_agent/media_auth/biz/dal"
	legacy "media_agent/media_auth/biz/dal/repo/platform_token"
	platform "media_agent/media_auth/biz/dal/repo/platform_token"
	"media_agent/media_auth/biz/shared/platformcipher"
	"media_agent/media_auth/biz/workflow"
)

func main() {
	// Shared loaders can log source errors; this process exposes only fixed
	// maintenance errors, never their potentially sensitive diagnostic text.
	log.SetOutput(io.Discard)
	if run(os.Args[1:], os.Stdout) != nil {
		fmt.Fprintln(os.Stderr, "platform token migration failed; verify maintenance prerequisites")
		os.Exit(1)
	}
}

func run(args []string, out io.Writer) error {
	flags := flag.NewFlagSet("platform-token-migrate", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	path := flags.String("config", "", "service configuration file (required)")
	keyFile := flags.String("key-file", "", "private 32-byte Secret file (required)")
	version := flags.String("key-version", "", "Secret version (required)")
	apply := flags.Bool("apply", false, "commit migration; requires stopped legacy callers and prepared cutover")
	timeout := flags.Duration("timeout", 2*time.Minute, "maximum maintenance duration")
	if err := flags.Parse(args); err != nil {
		return workflow.ErrPlatformMigration
	}
	if flags.NArg() != 0 || *path == "" || *timeout <= 0 || *timeout > 30*time.Minute {
		return workflow.ErrPlatformMigration
	}
	ring, err := platformcipher.LoadFile(*keyFile, *version)
	if err != nil {
		return workflow.ErrPlatformMigration
	}
	loader, err := config.NewLoaderFromPath(*path)
	if err != nil {
		return workflow.ErrPlatformMigration
	}
	defer loader.Close()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, *timeout)
	defer cancel()
	db, err := dal.NewMaintenanceDB(loader.Current().GetDatabase())
	if err != nil || db == nil {
		return workflow.ErrPlatformMigration
	}
	sqlDB, err := db.DB()
	if err != nil {
		return workflow.ErrPlatformMigration
	}
	defer sqlDB.Close()
	if err := sqlDB.PingContext(ctx); err != nil {
		return workflow.ErrPlatformMigration
	}
	w := workflow.NewPlatformTokenMigration(dal.NewTxRunner(db), legacy.NewPlatformMigrationSource(db), platform.NewRepo(db), ring)
	result, err := w.Run(ctx, *apply)
	if err != nil {
		return workflow.ErrPlatformMigration
	}
	return json.NewEncoder(out).Encode(result)
}
