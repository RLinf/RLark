package db

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"time"

	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/driver/pgdriver"
	"github.com/uptrace/bun/extra/bundebug"
	"github.com/uptrace/bun/migrate"
)

// DB wraps a Bun database connection and provides health checks.
type DB struct {
	*bun.DB
	cfg      Config
	migrator *migrate.Migrator
}

// Open connects to PostgreSQL via Bun+pgdriver and returns a DB handle.
func Open(cfg Config) (*DB, error) {
	connector := pgdriver.NewConnector(
		pgdriver.WithDSN(cfg.DSN()),
		pgdriver.WithTimeout(5*time.Second),
		pgdriver.WithWriteTimeout(30*time.Second),
		pgdriver.WithReadTimeout(30*time.Second),
	)

	sqlDB := sql.OpenDB(connector)
	sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(cfg.ConnMaxLifetime)
	sqlDB.SetConnMaxIdleTime(cfg.ConnMaxIdleTime)

	db := bun.NewDB(sqlDB, pgdialect.New())

	if cfg.Debug {
		db.AddQueryHook(bundebug.NewQueryHook(
			bundebug.WithVerbose(true),
			bundebug.FromEnv(""),
		))
	}

	// Verify connectivity
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	return &DB{
		DB:       db,
		cfg:      cfg,
		migrator: migrate.NewMigrator(db, Migrations),
	}, nil
}

// OpenFromFileConfig opens the fromFileConfig.
func OpenFromFileConfig(path string) (*DB, error) {
	dbConfig := DefaultConfig()
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read database config file: %w", err)
	}
	if err := UnmarshalConfig(data, &dbConfig); err != nil {
		return nil, fmt.Errorf("unmarshal database config: %w", err)
	}

	return Open(dbConfig)
}

// Config returns the config used to create this DB.
func (d *DB) Config() Config {
	return d.cfg
}

// HealthCheck pings the database.
func (d *DB) HealthCheck(ctx context.Context) error {
	return d.PingContext(ctx)
}

// Close closes the underlying sql.DB connection pool.
func (d *DB) Close() error {
	return d.DB.Close()
}

// Migrator returns the Bun migrator for manual migration management.
func (d *DB) Migrator() *migrate.Migrator {
	return d.migrator
}

// Migrate runs all pending migrations. Safe to call repeatedly — only
// unapplied migrations will execute.
func (d *DB) Migrate(ctx context.Context) error {
	if err := d.migrator.Init(ctx); err != nil {
		return fmt.Errorf("init migrations table: %w", err)
	}
	group, err := d.migrator.Migrate(ctx)
	if err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	if group.ID == 0 {
		// no pending migrations
		return nil
	}
	return nil
}

// Rollback reverses the last migration group.
func (d *DB) Rollback(ctx context.Context) error {
	group, err := d.migrator.Rollback(ctx)
	if err != nil {
		return fmt.Errorf("rollback: %w", err)
	}
	if group.ID == 0 {
		return nil
	}
	return nil
}

// Reset drops everything and re-runs all migrations from scratch.
func (d *DB) Reset(ctx context.Context) error {
	if err := d.migrator.Reset(ctx); err != nil {
		return fmt.Errorf("reset: %w", err)
	}
	return nil
}

// MigrationStatus returns the list of applied and pending migrations.
func (d *DB) MigrationStatus(ctx context.Context) ([]migrate.Migration, error) {
	if err := d.migrator.Init(ctx); err != nil {
		return nil, err
	}
	ms, err := d.migrator.MigrationsWithStatus(ctx)
	if err != nil {
		return nil, err
	}
	return ms, nil
}
