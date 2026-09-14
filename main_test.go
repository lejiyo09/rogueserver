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

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAllowedOrigin(t *testing.T) {
	t.Run("SingleConfiguredOriginAlwaysReturnedRegardlessOfRequest", func(t *testing.T) {
		allowed := []string{"https://pokerogue.net"}

		for _, requestOrigin := range []string{"https://pokerogue.net", "https://evil.example", "", "null"} {
			got := allowedOrigin(allowed, requestOrigin)
			want := "https://pokerogue.net"
			if got != want {
				t.Errorf("allowedOrigin(%v, %q) = %q, want %q", allowed, requestOrigin, got, want)
			}
		}
	})

	t.Run("MultipleConfiguredOriginsReflectMatchingRequestOrigin", func(t *testing.T) {
		allowed := []string{"https://pokerogue.net", "null"}

		got := allowedOrigin(allowed, "null")
		want := "null"
		if got != want {
			t.Errorf("allowedOrigin(%v, %q) = %q, want %q", allowed, "null", got, want)
		}
	})

	t.Run("MultipleConfiguredOriginsFallBackToFirstForUnknownOrigin", func(t *testing.T) {
		allowed := []string{"https://pokerogue.net", "null"}

		got := allowedOrigin(allowed, "https://evil.example")
		want := "https://pokerogue.net"
		if got != want {
			t.Errorf("allowedOrigin(%v, %q) = %q, want %q", allowed, "https://evil.example", got, want)
		}
	})
}

func TestProdHandlerCORSHeaders(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/ok", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	t.Run("SingleOriginConfigured_UnchangedHistoricalBehavior", func(t *testing.T) {
		handler := prodHandler(mux, "https://pokerogue.net")

		req := httptest.NewRequest(http.MethodGet, "/ok", nil)
		req.Header.Set("Origin", "null")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		got := rec.Header().Get("Access-Control-Allow-Origin")
		want := "https://pokerogue.net"
		if got != want {
			t.Errorf("Access-Control-Allow-Origin = %q, want %q", got, want)
		}
	})

	t.Run("MultipleOriginsConfigured_ReflectsAllowedRequestOrigin", func(t *testing.T) {
		handler := prodHandler(mux, "https://pokerogue.net, null")

		req := httptest.NewRequest(http.MethodGet, "/ok", nil)
		req.Header.Set("Origin", "null")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		got := rec.Header().Get("Access-Control-Allow-Origin")
		want := "null"
		if got != want {
			t.Errorf("Access-Control-Allow-Origin = %q, want %q", got, want)
		}
	})

	t.Run("OptionsPreflightIsHandledDirectly", func(t *testing.T) {
		handler := prodHandler(mux, "https://pokerogue.net")

		req := httptest.NewRequest(http.MethodOptions, "/ok", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("OPTIONS status = %d, want %d", rec.Code, http.StatusOK)
		}
	})
}

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
