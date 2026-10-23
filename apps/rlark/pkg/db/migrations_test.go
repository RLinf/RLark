package db

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// migrationFilePattern matches the migration file naming scheme that bun uses
// to derive a migration name from the file that calls MustRegister.
var migrationFilePattern = regexp.MustCompile(`^(\d{14})_([a-z0-9_]+)\.go$`)

// TestMigrationsAreRegistered guards the silent failure mode of bun's
// file-name based registration: a migration file that forgets to call
// Migrations.MustRegister in an init function compiles, ships and simply never
// runs, leaving the schema behind without any error.
func TestMigrationsAreRegistered(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}

	wantNames := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		match := migrationFilePattern.FindStringSubmatch(entry.Name())
		if match == nil {
			continue
		}
		wantNames = append(wantNames, match[1])
	}
	if len(wantNames) == 0 {
		t.Fatal("no migration files found; the naming scheme or this test is wrong")
	}
	sort.Strings(wantNames)

	registered := map[string]struct{}{}
	for _, migration := range Migrations.Sorted() {
		registered[migration.Name] = struct{}{}
	}

	for _, name := range wantNames {
		if _, ok := registered[name]; !ok {
			t.Errorf("migration %s is not registered; does its file call Migrations.MustRegister in init?", name)
		}
		delete(registered, name)
	}
	for name := range registered {
		t.Errorf("migration %s is registered but has no matching migration file", name)
	}
}

// TestMigrationsAreOrderedAndReversible checks the invariants the migrator
// relies on: names order the migrations deterministically, and every migration
// can be rolled back.
func TestMigrationsAreOrderedAndReversible(t *testing.T) {
	sorted := Migrations.Sorted()

	previous := ""
	for _, migration := range sorted {
		if migration.Name <= previous {
			t.Errorf("migration %q does not sort strictly after %q; names must be unique and increasing", migration.Name, previous)
		}
		previous = migration.Name

		if migration.Up == nil {
			t.Errorf("migration %s has no up function", migration.Name)
		}
		if migration.Down == nil {
			t.Errorf("migration %s has no down function; a migration that cannot be rolled back blocks recovery", migration.Name)
		}
	}
}
