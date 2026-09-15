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

package api

import (
	"sync/atomic"
	"time"

	"github.com/robfig/cron/v3"
)

// playerCount, battleCount, and classicSessionCount are written from the
// cron scheduler's goroutine (below) and read concurrently from HTTP handler
// goroutines (handleGameTitleStats, handleGameClassicSessionCount in
// endpoints.go) - every net/http request runs in its own goroutine, so a
// plain int here would be a data race. atomic.Int64 makes each read/write a
// single atomic operation with no risk of a torn/stale value.
var (
	scheduler           = cron.New(cron.WithLocation(time.UTC))
	playerCount         atomic.Int64
	battleCount         atomic.Int64
	classicSessionCount atomic.Int64
)

func scheduleStatRefresh[T updateStatsStore](store T) error {
	_, err := scheduler.AddFunc("@every 1m", func() {
		if count, err := store.FetchPlayerCount(); err == nil {
			playerCount.Store(int64(count))
		}
	})
	if err != nil {
		return err
	}

	_, err = scheduler.AddFunc("@every 1h", func() {
		if count, err := store.FetchBattleCount(); err == nil {
			battleCount.Store(int64(count))
		}
	})
	if err != nil {
		return err
	}

	_, err = scheduler.AddFunc("@every 1h", func() {
		if count, err := store.FetchClassicSessionCount(); err == nil {
			classicSessionCount.Store(int64(count))
		}
	})
	if err != nil {
		return err
	}

	scheduler.Start()

	return nil
}

type updateStatsStore interface {
	FetchPlayerCount() (int, error)
	FetchBattleCount() (int, error)
	FetchClassicSessionCount() (int, error)
}
