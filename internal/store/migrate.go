package store

import (
	"context"
	_ "embed"

	"github.com/jackc/pgx/v4/pgxpool"
)

//go:embed migrations/001_init.sql
var migrationSQL string

//go:embed migrations/002_events.sql
var migrationEvents string

func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	if _, err := pool.Exec(ctx, migrationSQL); err != nil {
		return err
	}
	if _, err := pool.Exec(ctx, migrationEvents); err != nil {
		return err
	}
	return nil
}
