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

import (
	"testing"

	"github.com/pagefaultgames/rogueserver/defs"
)

type fakeUpsertStore struct {
	stored []defs.BankedPokemonData
}

func (f *fakeUpsertStore) UpsertBankedPokemon(uuid []byte, entries []defs.BankedPokemonData) error {
	f.stored = entries
	return nil
}

func TestUpsertCollectionRejectsMissingUid(t *testing.T) {
	store := &fakeUpsertStore{}

	entries := []defs.BankedPokemonData{
		{Uid: "a-valid-uid", PveLevel: 12},
		{Uid: "", PveLevel: 5},
	}

	err := UpsertCollection(store, []byte("uuid"), entries)
	if err == nil {
		t.Fatal("expected an error for an entry with an empty uid, got nil")
	}

	if store.stored != nil {
		t.Fatal("expected the store to not be called when validation fails")
	}
}

func TestUpsertCollectionAcceptsValidEntries(t *testing.T) {
	store := &fakeUpsertStore{}

	entries := []defs.BankedPokemonData{
		{Uid: "a", PveLevel: 12},
		{Uid: "b", PveLevel: 5},
	}

	err := UpsertCollection(store, []byte("uuid"), entries)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(store.stored) != len(entries) {
		t.Fatalf("expected %d entries to reach the store, got %d", len(entries), len(store.stored))
	}
}
