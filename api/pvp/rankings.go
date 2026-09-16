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

package pvp

import "github.com/pagefaultgames/rogueserver/defs"

type RankingsStore interface {
	FetchPvpRankings(page int) ([]defs.PvpRanking, error)
}

// Rankings returns page (1-indexed) of the PvP lobby's leaderboard.
func Rankings[T RankingsStore](store T, page int) ([]defs.PvpRanking, error) {
	rankings, err := store.FetchPvpRankings(page)
	if err != nil {
		return rankings, err
	}

	return rankings, nil
}

type RankingPageCountStore interface {
	FetchPvpRankingPageCount() (int, error)
}

// RankingPageCount returns the total number of pages Rankings can return.
func RankingPageCount[T RankingPageCountStore](store T) (int, error) {
	pageCount, err := store.FetchPvpRankingPageCount()
	if err != nil {
		return pageCount, err
	}

	return pageCount, nil
}
