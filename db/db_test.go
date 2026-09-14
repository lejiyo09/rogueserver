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
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestBuildDSN(t *testing.T) {
	t.Run("WithoutTLS", func(t *testing.T) {
		got := buildDSN("user", "pass", "tcp", "localhost:3306", "mydb", "")
		want := "user:pass@tcp(localhost:3306)/mydb?timeout=" + connDialTimeout +
			"&readTimeout=" + connReadTimeout + "&writeTimeout=" + connWriteTimeout
		if got != want {
			t.Errorf("buildDSN() = %q, want %q", got, want)
		}
	})

	t.Run("WithTLS", func(t *testing.T) {
		got := buildDSN("avnadmin", "secret", "tcp", "mysql-example.aivencloud.com:22671", "defaultdb", tlsConfigName)
		want := "avnadmin:secret@tcp(mysql-example.aivencloud.com:22671)/defaultdb?timeout=" + connDialTimeout +
			"&readTimeout=" + connReadTimeout + "&writeTimeout=" + connWriteTimeout + "&tls=" + tlsConfigName
		if got != want {
			t.Errorf("buildDSN() = %q, want %q", got, want)
		}
	})
}

// generateTestCAPEM returns a throwaway, self-signed CA certificate PEM
// block, valid only for exercising loadCACertPool - not a real trust anchor.
func generateTestCAPEM(t *testing.T) []byte {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate test key: %s", err)
	}

	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "rogueserver test CA"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
	}

	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("failed to create test certificate: %s", err)
	}

	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}

func TestLoadCACertPool(t *testing.T) {
	t.Run("ValidPEM", func(t *testing.T) {
		certPEM := generateTestCAPEM(t)
		path := filepath.Join(t.TempDir(), "ca.pem")
		if err := os.WriteFile(path, certPEM, 0o600); err != nil {
			t.Fatalf("failed to write test CA file: %s", err)
		}

		pool, err := loadCACertPool(path)
		if err != nil {
			t.Fatalf("loadCACertPool() unexpected error: %s", err)
		}
		if pool == nil {
			t.Fatal("loadCACertPool() returned a nil pool for a valid PEM file")
		}
	})

	t.Run("MissingFile", func(t *testing.T) {
		_, err := loadCACertPool(filepath.Join(t.TempDir(), "does-not-exist.pem"))
		if err == nil {
			t.Fatal("loadCACertPool() expected an error for a missing file, got nil")
		}
	})

	t.Run("NotPEM", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "not-a-cert.pem")
		if err := os.WriteFile(path, []byte("this is not a PEM certificate"), 0o600); err != nil {
			t.Fatalf("failed to write test file: %s", err)
		}

		_, err := loadCACertPool(path)
		if err == nil {
			t.Fatal("loadCACertPool() expected an error for non-PEM content, got nil")
		}
	})
}
