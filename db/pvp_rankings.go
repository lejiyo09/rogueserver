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
	"math"

	"github.com/pagefaultgames/rogueserver/defs"
)

// RecordPvpResult increments uuid's win or loss count by one.
//
// No HTTP endpoint calls this yet - a client-facing "I won" endpoint would
// let any player inflate their own record, since there is currently no
// server-authoritative source of PvP match results (that requires
// pvp-server, out of scope for this session; see
// docs/pvp-progression-design.md §6/§9). This function exists so the wiring
// only needs a caller, not a new storage layer, once pvp-server can report
// results server-to-server.
func (s *store) RecordPvpResult(uuid []byte, won bool) error {
	win, loss := 0, 0
	if won {
		win = 1
	} else {
		loss = 1
	}

	_, err := handle.Exec(
		"INSERT INTO pvpRecords (uuid, wins, losses, timestamp) VALUES (?, ?, ?, UTC_TIMESTAMP()) "+
			"ON DUPLICATE KEY UPDATE wins = wins + ?, losses = losses + ?, timestamp = UTC_TIMESTAMP()",
		uuid, win, loss, win, loss,
	)
	if err != nil {
		return err
	}

	return nil
}

// FetchPvpRankings returns page (1-indexed, 10 rows per page) of the PvP
// leaderboard, ranked by wins - see docs/pvp-progression-design.md §9.2 for
// why this deliberately mirrors FetchRankings (daily.go) instead of
// introducing a new rating algorithm.
func (s *store) FetchPvpRankings(page int) ([]defs.PvpRanking, error) {
	var rankings []defs.PvpRanking

	offset := (page - 1) * 10

	query := "SELECT RANK() OVER (ORDER BY pr.wins DESC, pr.timestamp), a.username, pr.wins, pr.losses " +
		"FROM pvpRecords pr JOIN accounts a ON a.uuid = pr.uuid WHERE a.banned = 0 LIMIT 10 OFFSET ?"

	results, err := handle.Query(query, offset)
	if err != nil {
		return rankings, err
	}

	defer results.Close()

	for results.Next() {
		var ranking defs.PvpRanking
		err = results.Scan(&ranking.Rank, &ranking.Username, &ranking.Wins, &ranking.Losses)
		if err != nil {
			return rankings, err
		}

		rankings = append(rankings, ranking)
	}

	return rankings, results.Err()
}

// FetchPvpRankingPageCount returns the total number of pages FetchPvpRankings can return.
func (s *store) FetchPvpRankingPageCount() (int, error) {
	var recordCount int
	err := handle.QueryRow("SELECT COUNT(a.username) FROM pvpRecords pr JOIN accounts a ON a.uuid = pr.uuid WHERE a.banned = 0").Scan(&recordCount)
	if err != nil {
		return 0, err
	}

	return int(math.Ceil(float64(recordCount) / 10)), nil
}
