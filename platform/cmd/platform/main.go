package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/officecli/officecli-internal/platform/internal/app"
	"github.com/officecli/officecli-internal/platform/internal/store/sqlstore"
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
		return fmt.Errorf("db copy is no longer supported; this project only supports PostgreSQL")
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
