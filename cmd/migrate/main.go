package main

import (
	"context"
	"fmt"
	"os"

	"database/sql"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

func main() {
	databaseURL := os.Getenv("CDK_DATABASE_URL")
	if databaseURL == "" {
		fmt.Fprintln(os.Stderr, "CDK_DATABASE_URL is required")
		os.Exit(1)
	}

	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to open database: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	if err := db.PingContext(context.Background()); err != nil {
		fmt.Fprintf(os.Stderr, "failed to ping database: %v\n", err)
		os.Exit(1)
	}

	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: migrate <up|down|status>")
		os.Exit(1)
	}

	provider, err := goose.NewProvider(
		goose.DialectPostgres,
		db,
		os.DirFS("db/migrations"),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create goose provider: %v\n", err)
		os.Exit(1)
	}

	ctx := context.Background()
	command := os.Args[1]
	switch command {
	case "up":
		_, err = provider.Up(ctx)
	case "down":
		_, err = provider.Down(ctx)
	case "status":
		version, err := provider.GetDBVersion(ctx)
		if err == nil {
			fmt.Printf("database version: %d\n", version)
		}
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", command)
		os.Exit(1)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "migration failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("migration completed")
}
