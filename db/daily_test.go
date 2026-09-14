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

import "testing"

// setupDailyTestDB points the package-level `handle` at a local MySQL/
// MariaDB instance (via the real Init(), so schema setup runs exactly as it
// does on a real startup) and clears out any leftover dailyRuns row for
// today before/after the test. Skips if no such database is reachable, so
// `go test ./...` stays safe to run without one (e.g. CI).
func setupDailyTestDB(t *testing.T) {
	t.Helper()

	if err := Init("rstest", "rstest", "tcp", "127.0.0.1:3306", "rogueserver_test"); err != nil {
		t.Skipf("skipping: no local MySQL/MariaDB reachable for integration test: %s", err)
	}

	handle.Exec("DELETE FROM dailyRuns")
	t.Cleanup(func() { handle.Exec("DELETE FROM dailyRuns") })
}

// TestTryAddDailyRunReturnsNonEmptySeed is the regression check for the
// MySQL 8.x incompatibility this fixes: TryAddDailyRun's query used to end
// in "... RETURNING seed", a MariaDB-only extension that plain MySQL
// (including 8.x, as used by managed providers like Aiven) rejects with a
// syntax error (ER_PARSE_ERROR/1064) - which surfaced as an empty daily
// seed (the "Daily Run Seed:" log line with nothing after it) once schema
// setup itself got past the earlier CREATE INDEX incompatibility.
func TestTryAddDailyRunReturnsNonEmptySeed(t *testing.T) {
	setupDailyTestDB(t)

	const proposedSeed = "test-seed-1234567890123"
	seed, err := Store.TryAddDailyRun(proposedSeed)
	if err != nil {
		t.Fatalf("TryAddDailyRun() unexpected error: %s", err)
	}
	if seed == "" {
		t.Fatal("TryAddDailyRun() returned an empty seed")
	}
	if seed != proposedSeed {
		t.Errorf("TryAddDailyRun() = %q, want the seed just claimed (%q)", seed, proposedSeed)
	}
}

// TestTryAddDailyRunIsIdempotentForTheSameDay verifies TryAddDailyRun keeps
// its "first caller for the day wins" semantics after being rewritten
// without RETURNING: a second, different seed proposed for the same day
// must not overwrite the first, and must get back whichever seed actually
// won instead.
func TestTryAddDailyRunIsIdempotentForTheSameDay(t *testing.T) {
	setupDailyTestDB(t)

	first, err := Store.TryAddDailyRun("first-seed-123456789012")
	if err != nil {
		t.Fatalf("first TryAddDailyRun() failed: %s", err)
	}
	if first == "" {
		t.Fatal("first TryAddDailyRun() returned an empty seed")
	}

	second, err := Store.TryAddDailyRun("second-seed-12345678901")
	if err != nil {
		t.Fatalf("second TryAddDailyRun() failed: %s", err)
	}
	if second != first {
		t.Errorf("second TryAddDailyRun() = %q, want the already-claimed seed %q", second, first)
	}
}

func TestGetDailyRunSeedMatchesWhatWasClaimed(t *testing.T) {
	setupDailyTestDB(t)

	claimed, err := Store.TryAddDailyRun("get-seed-test-123456789")
	if err != nil {
		t.Fatalf("TryAddDailyRun() failed: %s", err)
	}

	got, err := Store.GetDailyRunSeed()
	if err != nil {
		t.Fatalf("GetDailyRunSeed() unexpected error: %s", err)
	}
	if got == "" {
		t.Fatal("GetDailyRunSeed() returned an empty seed")
	}
	if got != claimed {
		t.Errorf("GetDailyRunSeed() = %q, want %q", got, claimed)
	}
}
