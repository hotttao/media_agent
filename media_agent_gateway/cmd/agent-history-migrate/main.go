package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"

	_ "github.com/jackc/pgx/v5/stdlib"
	"media_agent/media_agent_gateway/biz/migration/agenthistory"
)

func main() {
	mode := flag.String("mode", "verify", "import, verify, or cleanup")
	sourceDSN := flag.String("gateway-dsn", os.Getenv("POSTGRES_DSN"), "Gateway PostgreSQL DSN")
	targetDSN := flag.String("agents-dsn", os.Getenv("MEDIA_AGENTS_MIGRATION_DSN"), "Agent PostgreSQL DSN")
	confirm := flag.Bool("confirm-drop-legacy-history", false, "required for cleanup")
	flag.Parse()
	if *sourceDSN == "" || *targetDSN == "" {
		log.Fatal("both Gateway and Agent DSNs are required")
	}
	source, err := sql.Open("pgx", *sourceDSN)
	if err != nil {
		log.Fatal(err)
	}
	defer source.Close()
	target, err := sql.Open("pgx", *targetDSN)
	if err != nil {
		log.Fatal(err)
	}
	defer target.Close()
	m := agenthistory.Migrator{Source: source, Target: target}
	ctx := context.Background()
	switch *mode {
	case "import":
		err = m.Import(ctx)
	case "verify":
		err = m.Verify(ctx)
	case "cleanup":
		err = m.Cleanup(ctx, *confirm)
	default:
		err = fmt.Errorf("unsupported mode %q", *mode)
	}
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("Agent history %s completed", *mode)
}
