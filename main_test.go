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

package main

import "testing"

func TestResolveAddr(t *testing.T) {
	t.Run("NeitherSet", func(t *testing.T) {
		got := resolveAddr()
		want := "0.0.0.0:8001"
		if got != want {
			t.Errorf("resolveAddr() = %q, want %q", got, want)
		}
	})

	t.Run("PortSetByPlatform", func(t *testing.T) {
		t.Setenv("PORT", "10000")
		got := resolveAddr()
		want := "0.0.0.0:10000"
		if got != want {
			t.Errorf("resolveAddr() = %q, want %q", got, want)
		}
	})

	t.Run("ExplicitAddrTakesPrecedenceOverPort", func(t *testing.T) {
		t.Setenv("PORT", "10000")
		t.Setenv("addr", "127.0.0.1:9999")
		got := resolveAddr()
		want := "127.0.0.1:9999"
		if got != want {
			t.Errorf("resolveAddr() = %q, want %q", got, want)
		}
	})

	t.Run("ExplicitAddrTakesPrecedenceWithoutPort", func(t *testing.T) {
		t.Setenv("addr", "unix:/tmp/rogueserver.sock")
		got := resolveAddr()
		want := "unix:/tmp/rogueserver.sock"
		if got != want {
			t.Errorf("resolveAddr() = %q, want %q", got, want)
		}
	})
}
