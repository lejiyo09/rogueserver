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
	"bytes"
	"encoding/gob"
	"fmt"

	"github.com/klauspost/compress/zstd"
	"github.com/pagefaultgames/rogueserver/defs"
)

func encodeBankedPokemon(entry defs.BankedPokemonData) ([]byte, error) {
	buf := new(bytes.Buffer)

	zw, err := zstd.NewWriter(buf)
	if err != nil {
		return nil, err
	}

	err = gob.NewEncoder(zw).Encode(entry)
	if err != nil {
		return nil, err
	}

	err = zw.Close()
	if err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func decodeBankedPokemon(data []byte) (defs.BankedPokemonData, error) {
	var entry defs.BankedPokemonData

	zr, err := zstd.NewReader(bytes.NewReader(data))
	if err != nil {
		return entry, err
	}

	defer zr.Close()

	err = gob.NewDecoder(zr).Decode(&entry)
	if err != nil {
		return entry, err
	}

	return entry, nil
}

// ReadBankedPokemon returns every individual banked in uuid's PvP Global
// Pokémon Collection.
func (s *store) ReadBankedPokemon(uuid []byte) ([]defs.BankedPokemonData, error) {
	rows, err := handle.Query("SELECT data FROM bankedPokemon WHERE uuid = ? ORDER BY timestamp ASC", uuid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []defs.BankedPokemonData
	for rows.Next() {
		var data []byte
		if err := rows.Scan(&data); err != nil {
			return nil, err
		}

		entry, err := decodeBankedPokemon(data)
		if err != nil {
			return nil, err
		}

		entries = append(entries, entry)
	}

	return entries, rows.Err()
}

// UpsertBankedPokemon banks each entry into uuid's collection, keyed by its
// Uid - an entry whose Uid is already banked is overwritten in place rather
// than duplicated, so re-syncing the same individual (e.g. after a level up)
// never grows the collection.
func (s *store) UpsertBankedPokemon(uuid []byte, entries []defs.BankedPokemonData) error {
	if len(entries) == 0 {
		return nil
	}

	tx, err := handle.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, entry := range entries {
		if entry.Uid == "" {
			return fmt.Errorf("banked pokemon entry missing uid")
		}

		data, err := encodeBankedPokemon(entry)
		if err != nil {
			return err
		}

		_, err = tx.Exec(
			"REPLACE INTO bankedPokemon (uuid, pokemonUid, data, timestamp) VALUES (?, ?, ?, UTC_TIMESTAMP())",
			uuid, entry.Uid, data,
		)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}
