//go:build devsetup
// +build devsetup

package db

import (
	"database/sql"
	"fmt"
	"os"
)

// MaybeSetupDb is called by db.go and runs setupDb only in devsetup builds.
func MaybeSetupDb(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	err = setupDb(tx)
	if err != nil {
		tx.Rollback()
		return err
	}
	err = tx.Commit()
	if err != nil {
		return err
	}
	return nil
}

// index describes a single non-unique index that setupDb ensures exists.
// Kept separate from the CREATE TABLE statements below because MySQL's
// CREATE INDEX (unlike MariaDB's) has no IF NOT EXISTS form - see
// createIndexIfNotExists, which achieves the same idempotency portably
// (MySQL and MariaDB alike) via information_schema instead.
type index struct {
	name    string
	table   string
	columns string
}

var indexes = []index{
	{"accountsByActivity", "accounts", "lastActivity"},
	{"sessionsByUuid", "sessions", "uuid"},
	{"dailyRunsByDateAndSeed", "dailyRuns", "date, seed"},
	{"dailyRunCompletionsByUuidAndSeed", "dailyRunCompletions", "uuid, seed"},
	{"accountDailyRunsByDate", "accountDailyRuns", "date"},
}

// createIndexIfNotExists creates the named index only if it doesn't already
// exist, checked via information_schema.statistics rather than "CREATE INDEX
// IF NOT EXISTS" (MariaDB-only syntax; plain MySQL, including 8.4, rejects it
// with a syntax error - ER_PARSE_ERROR/1064). This makes index creation
// idempotent - safe to run on every startup, including against a database
// that already has the index - on both MySQL and MariaDB.
func createIndexIfNotExists(tx *sql.Tx, idx index) error {
	var count int
	err := tx.QueryRow(
		`SELECT COUNT(*) FROM information_schema.statistics WHERE table_schema = DATABASE() AND table_name = ? AND index_name = ?`,
		idx.table, idx.name,
	).Scan(&count)
	if err != nil {
		return fmt.Errorf("failed to check for existing index %s: %w", idx.name, err)
	}
	if count > 0 {
		return nil
	}

	_, err = tx.Exec(createIndexSQL(idx))
	if err != nil {
		return fmt.Errorf("failed to create index %s: %w", idx.name, err)
	}
	return nil
}

// createIndexSQL builds the (plain, no IF NOT EXISTS) CREATE INDEX statement
// for idx. Split out from createIndexIfNotExists so it can be unit-tested
// without a database connection.
func createIndexSQL(idx index) string {
	return fmt.Sprintf("CREATE INDEX %s ON %s (%s)", idx.name, idx.table, idx.columns)
}

// column describes a single column setupDb ensures exists on an
// already-created table, for columns added after a table's original
// CREATE TABLE statement (that statement still creates it directly for a
// brand-new database) - MySQL has no "ADD COLUMN IF NOT EXISTS" form, so
// existence is checked via information_schema instead, same approach as
// createIndexIfNotExists.
type column struct {
	name, table, definition string
}

var columns = []column{
	{"firebaseUid", "accounts", "VARCHAR(32) UNIQUE DEFAULT NULL"},
	{"cheatsEnabled", "accounts", "TINYINT(1) NOT NULL DEFAULT 0"},
}

// addColumnIfNotExists adds the named column only if it doesn't already
// exist, so it's safe to run on every startup, including against a database
// that already has it.
func addColumnIfNotExists(tx *sql.Tx, col column) error {
	var count int
	err := tx.QueryRow(
		`SELECT COUNT(*) FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = ? AND column_name = ?`,
		col.table, col.name,
	).Scan(&count)
	if err != nil {
		return fmt.Errorf("failed to check for existing column %s: %w", col.name, err)
	}
	if count > 0 {
		return nil
	}

	_, err = tx.Exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", col.table, col.name, col.definition))
	if err != nil {
		return fmt.Errorf("failed to add column %s: %w", col.name, err)
	}
	return nil
}

func setupDb(tx *sql.Tx) error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS accounts (
		       uuid BINARY(16) NOT NULL PRIMARY KEY,
		       username VARCHAR(16) UNIQUE NOT NULL,
		       hash BINARY(32) NOT NULL,
		       salt BINARY(16) NOT NULL,
		       registered TIMESTAMP NOT NULL,
		       lastLoggedIn TIMESTAMP DEFAULT NULL,
		       lastActivity TIMESTAMP DEFAULT NULL,
		       banned TINYINT(1) NOT NULL DEFAULT 0,
		       trainerId SMALLINT(5) UNSIGNED DEFAULT 0,
		       secretId SMALLINT(5) UNSIGNED DEFAULT 0,
		       discordId VARCHAR(32) UNIQUE DEFAULT NULL,
		       googleId VARCHAR(32) UNIQUE DEFAULT NULL,
		       firebaseUid VARCHAR(32) UNIQUE DEFAULT NULL,
		       cheatsEnabled TINYINT(1) NOT NULL DEFAULT 0
	       )`,

		`CREATE TABLE IF NOT EXISTS sessions (
		       token BINARY(32) NOT NULL PRIMARY KEY,
		       uuid BINARY(16) NOT NULL,
		       expire TIMESTAMP DEFAULT NULL,
		       CONSTRAINT sessions_ibfk_1 FOREIGN KEY (uuid) REFERENCES accounts (uuid) ON DELETE CASCADE ON UPDATE CASCADE
	       )`,

		`CREATE TABLE IF NOT EXISTS accountStats (
		       uuid BINARY(16) NOT NULL PRIMARY KEY,
		       playTime INT(11) NOT NULL DEFAULT 0,
		       battles INT(11) NOT NULL DEFAULT 0,
		       classicSessionsPlayed INT(11) NOT NULL DEFAULT 0,
		       sessionsWon INT(11) NOT NULL DEFAULT 0,
		       highestEndlessWave INT(11) NOT NULL DEFAULT 0,
		       highestLevel INT(11) NOT NULL DEFAULT 0,
		       pokemonSeen INT(11) NOT NULL DEFAULT 0,
		       pokemonDefeated INT(11) NOT NULL DEFAULT 0,
		       pokemonCaught INT(11) NOT NULL DEFAULT 0,
		       pokemonHatched INT(11) NOT NULL DEFAULT 0,
		       eggsPulled INT(11) NOT NULL DEFAULT 0,
		       regularVouchers INT(11) NOT NULL DEFAULT 0,
		       plusVouchers INT(11) NOT NULL DEFAULT 0,
		       premiumVouchers INT(11) NOT NULL DEFAULT 0,
		       goldenVouchers INT(11) NOT NULL DEFAULT 0,
		       CONSTRAINT accountStats_ibfk_1 FOREIGN KEY (uuid) REFERENCES accounts (uuid) ON DELETE CASCADE ON UPDATE CASCADE
	       )`,

		`CREATE TABLE IF NOT EXISTS dailyRuns (
		       date DATE NOT NULL PRIMARY KEY,
		       seed CHAR(24) CHARACTER SET ascii COLLATE ascii_bin NOT NULL
	       )`,

		`CREATE TABLE IF NOT EXISTS dailyRunCompletions (
		       uuid BINARY(16) NOT NULL,
		       seed CHAR(24) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
		       mode INT(11) NOT NULL DEFAULT 0,
		       score INT(11) NOT NULL DEFAULT 0,
		       timestamp TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		       PRIMARY KEY (uuid, seed),
		       CONSTRAINT dailyRunCompletions_ibfk_1 FOREIGN KEY (uuid) REFERENCES accounts (uuid) ON DELETE CASCADE ON UPDATE CASCADE
	       )`,

		`CREATE TABLE IF NOT EXISTS accountDailyRuns (
		       uuid BINARY(16) NOT NULL,
		       date DATE NOT NULL,
		       score INT(11) NOT NULL DEFAULT 0,
		       wave INT(11) NOT NULL DEFAULT 0,
		       timestamp TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		       PRIMARY KEY (uuid, date),
		       CONSTRAINT accountDailyRuns_ibfk_1 FOREIGN KEY (uuid) REFERENCES accounts (uuid) ON DELETE CASCADE ON UPDATE CASCADE,
		       CONSTRAINT accountDailyRuns_ibfk_2 FOREIGN KEY (date) REFERENCES dailyRuns (date) ON DELETE NO ACTION ON UPDATE NO ACTION
	       )`,

		`CREATE TABLE IF NOT EXISTS sessionSaveData (
		       uuid BINARY(16),
		       slot TINYINT,
		       data LONGBLOB,
		       timestamp TIMESTAMP,
		       PRIMARY KEY (uuid, slot),
		       FOREIGN KEY (uuid) REFERENCES accounts (uuid) ON DELETE CASCADE ON UPDATE CASCADE
	       )`,

		`CREATE TABLE IF NOT EXISTS activeClientSessions (
		       uuid BINARY(16) NOT NULL PRIMARY KEY,
		       clientSessionId VARCHAR(32) NOT NULL,
		       FOREIGN KEY (uuid) REFERENCES accounts (uuid) ON DELETE CASCADE ON UPDATE CASCADE
	       )`,
	}

	// Conditionally add systemSaveData table if AWS_ENDPOINT_URL_S3 is not set
	if os.Getenv("AWS_ENDPOINT_URL_S3") == "" {
		queries = append(queries, `CREATE TABLE IF NOT EXISTS systemSaveData (
		       uuid BINARY(16) PRIMARY KEY,
		       data LONGBLOB,
		       timestamp TIMESTAMP,
		       FOREIGN KEY (uuid) REFERENCES accounts (uuid) ON DELETE CASCADE ON UPDATE CASCADE
	       )`)
	}

	for _, q := range queries {
		_, err := tx.Exec(q)
		if err != nil {
			return fmt.Errorf("failed to execute query: %w, query: %s", err, q)
		}
	}

	// Columns added after a table's original CREATE TABLE statement (see
	// addColumnIfNotExists) are applied next, before indexes, since an
	// index may reference one of them.
	for _, col := range columns {
		if err := addColumnIfNotExists(tx, col); err != nil {
			return err
		}
	}

	// Indexes are created separately from the tables above - see
	// createIndexIfNotExists for why (MySQL's CREATE INDEX has no
	// IF NOT EXISTS form, unlike MariaDB's).
	for _, idx := range indexes {
		if err := createIndexIfNotExists(tx, idx); err != nil {
			return err
		}
	}

	return nil
}
