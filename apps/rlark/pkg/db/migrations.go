package db

import (
	"github.com/uptrace/bun/migrate"
)

// Migrations is the registry of all database migrations.
var Migrations = migrate.NewMigrations()
