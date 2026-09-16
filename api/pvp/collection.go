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

// Package pvp implements the PvP Global Pokémon Collection API described in
// docs/pvp-progression-design.md §2 (pokerogue repo) - the account-wide,
// cross-run store of individually banked Pokémon that PvP team building
// draws from.
package pvp

import (
	"fmt"

	"github.com/pagefaultgames/rogueserver/defs"
)

type ListCollectionStore interface {
	ReadBankedPokemon(uuid []byte) ([]defs.BankedPokemonData, error)
}

// ListCollection returns every individual banked in uuid's collection.
func ListCollection[T ListCollectionStore](store T, uuid []byte) ([]defs.BankedPokemonData, error) {
	entries, err := store.ReadBankedPokemon(uuid)
	if err != nil {
		return nil, err
	}

	return entries, nil
}

type UpsertCollectionStore interface {
	UpsertBankedPokemon(uuid []byte, entries []defs.BankedPokemonData) error
}

// UpsertCollection banks each of entries into uuid's collection. An entry
// whose Uid already exists is overwritten, never duplicated - see
// docs/pvp-progression-design.md §2.2's upsert-not-duplicate requirement.
func UpsertCollection[T UpsertCollectionStore](store T, uuid []byte, entries []defs.BankedPokemonData) error {
	for i, entry := range entries {
		if entry.Uid == "" {
			return fmt.Errorf("entry %d: missing uid", i)
		}
	}

	return store.UpsertBankedPokemon(uuid, entries)
}
