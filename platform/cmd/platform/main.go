package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/officecli/officecli/platform/internal/app"
	"github.com/officecli/officecli/platform/internal/dbtools"
	"github.com/officecli/officecli/platform/internal/store/sqlstore"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) >= 2 && args[0] == "db" && args[1] == "migrate" {
		return runMigrate()
	}
	if len(args) >= 2 && args[0] == "db" && args[1] == "copy" {
		return runCopy()
	}

	application, err := app.New()
	if err != nil {
		return err
	}
	return application.Run()
}

func runMigrate() error {
	cfg, err := app.LoadConfig()
	if err != nil {
		return err
	}
	store, err := sqlstore.New(cfg.PostgresDSN)
	if err != nil {
		return err
	}
	ctx := context.Background()
	if err := store.Ping(ctx); err != nil {
		return err
	}
	return store.EnsureMigrations(ctx)
}

func runCopy() error {
	cfg, err := app.LoadConfig()
	if err != nil {
		return err
	}
	mysqlDSN := os.Getenv("MYSQL_DSN")
	if mysqlDSN == "" {
		return fmt.Errorf("MYSQL_DSN is required for db copy")
	}
	summaries, err := dbtools.CopyMySQLToPostgres(context.Background(), mysqlDSN, cfg.PostgresDSN)
	if err != nil {
		return err
	}
	for _, summary := range summaries {
		fmt.Printf("%s rows=%d max_id=%d\n", summary.Table, summary.Rows, summary.MaxIDTarget)
	}
	return nil
}
