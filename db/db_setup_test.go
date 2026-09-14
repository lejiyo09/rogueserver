//go:build devsetup
// +build devsetup

/*
	Copyright (C) 2024 - 2025  Pagefault Games

	This program is free software: you can redistribute it and/or modify
	it under the terms of the GNU Affero General Public License as published by
	the Free Software Foundation, either version 3 of the License, or
	(at your option) any later version.

	This program is distributed in the hope that it will be useful,
	but WITHOUT ANY WARRANTY; without even the implied warranty of
	MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
	GNU Affero General Public License for more details.

	You should have received a copy of the GNU Affero General Public License
	along with this program.  If not, see <http://www.gnu.org/licenses/>.
*/

package db

import (
	"database/sql"
	"fmt"
	"testing"

	_ "github.com/go-sql-driver/mysql"
)

func TestCreateIndexSQL(t *testing.T) {
	got := createIndexSQL(index{name: "accountsByActivity", table: "accounts", columns: "lastActivity"})
	want := "CREATE INDEX accountsByActivity ON accounts (lastActivity)"
	if got != want {
		t.Errorf("createIndexSQL() = %q, want %q", got, want)
	}
}

func TestIndexesAreWellFormed(t *testing.T) {
	seen := make(map[string]bool)
	for _, idx := range indexes {
		if idx.name == "" || idx.table == "" || idx.columns == "" {
			t.Errorf("index %+v has an empty field", idx)
		}
		key := idx.table + "." + idx.name
		if seen[key] {
			t.Errorf("duplicate index definition for %s", key)
		}
		seen[key] = true
	}
}

// testDSN points at a local MySQL/MariaDB instance for the integration test
// below. It's only used if reachable - see openTestDB - so `go test ./...`
// stays safe to run in environments without a local database (e.g. CI).
const testDSN = "rstest:rstest@tcp(127.0.0.1:3306)/rogueserver_test"

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()

	handle, err := sql.Open("mysql", testDSN)
	if err != nil {
		t.Skipf("skipping: failed to open test database connection: %s", err)
	}
	if err := handle.Ping(); err != nil {
		t.Skipf("skipping: no local MySQL/MariaDB reachable at %s (set one up with the DSN above to run this test): %s", testDSN, err)
	}
	return handle
}

// dropAllTestTables removes every table setupDb might create, so each test
// run starts from a clean slate regardless of what a previous run left behind.
func dropAllTestTables(t *testing.T, handle *sql.DB) {
	t.Helper()

	tables := []string{
		"systemSaveData", "activeClientSessions", "sessionSaveData",
		"accountDailyRuns", "dailyRunCompletions", "dailyRuns",
		"accountStats", "sessions", "accounts",
	}

	if _, err := handle.Exec("SET FOREIGN_KEY_CHECKS = 0"); err != nil {
		t.Fatalf("failed to disable FK checks: %s", err)
	}
	defer handle.Exec("SET FOREIGN_KEY_CHECKS = 1")

	for _, table := range tables {
		if _, err := handle.Exec(fmt.Sprintf("DROP TABLE IF EXISTS %s", table)); err != nil {
			t.Fatalf("failed to drop table %s: %s", table, err)
		}
	}
}

// TestSetupDbIdempotent is the real regression check for the MySQL 8.4
// "CREATE INDEX IF NOT EXISTS" incompatibility (ER_PARSE_ERROR/1064 - that
// syntax is MariaDB-only): it runs setupDb twice in a row against a real
// MySQL/MariaDB database, exactly as happens on every rogueserver
// (re)deploy. The second run - against a database that already has every
// table and index - must succeed too, not fail with a duplicate-index or
// duplicate-key error.
func TestSetupDbIdempotent(t *testing.T) {
	handle := openTestDB(t)
	defer handle.Close()

	dropAllTestTables(t, handle)
	defer dropAllTestTables(t, handle)

	runSetup := func() error {
		tx, err := handle.Begin()
		if err != nil {
			return err
		}
		if err := setupDb(tx); err != nil {
			tx.Rollback()
			return err
		}
		return tx.Commit()
	}

	if err := runSetup(); err != nil {
		t.Fatalf("first setupDb() run (fresh database) failed: %s", err)
	}

	for _, idx := range indexes {
		var count int
		err := handle.QueryRow(
			`SELECT COUNT(*) FROM information_schema.statistics WHERE table_schema = DATABASE() AND table_name = ? AND index_name = ?`,
			idx.table, idx.name,
		).Scan(&count)
		if err != nil {
			t.Fatalf("failed to verify index %s: %s", idx.name, err)
		}
		if count == 0 {
			t.Errorf("index %s on %s was not created by the first setupDb() run", idx.name, idx.table)
		}
	}

	if err := runSetup(); err != nil {
		t.Fatalf("second setupDb() run (simulated redeploy, everything already exists) failed: %s", err)
	}
}
