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
	"context"
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/go-sql-driver/mysql"
)

var handle *sql.DB
var s3client *s3.Client

// internal type used to implement the Store interface
type store struct{}

// Store is the global instance for DB access.
var Store = &store{}

// tlsConfigName is the key this package registers a custom TLS config
// under with the mysql driver (see mysql.RegisterTLSConfig), and the
// value used for the DSN's "tls" parameter when one is configured.
const tlsConfigName = "custom"

// connDialTimeout, connReadTimeout and connWriteTimeout bound how long the
// mysql driver will wait on a connection attempt or a single I/O operation.
// Without these, a connection that something between this server and the
// database has silently dropped - which happens routinely for idle
// connections crossing the public internet, e.g. between a Render web
// service and a managed database like Aiven, without either side seeing a
// TCP FIN/RST - hangs instead of failing. That hang can outlast the
// platform's own request timeout, which then answers with its own error
// page instead of this server's ever being reached - one with none of this
// server's CORS headers, which surfaces to a browser as a CORS failure
// instead of the ordinary DB error it actually is. Bounding it here makes
// a dead connection fail fast, as a normal Go error handled (and given
// CORS headers) the same way any other database error already is.
const (
	connDialTimeout  = "5s"
	connReadTimeout  = "10s"
	connWriteTimeout = "5s"
)

// buildDSN assembles the go-sql-driver/mysql DSN used to open the database
// connection. When tlsConfigKey is non-empty, a "tls" parameter referencing
// a config previously registered via mysql.RegisterTLSConfig is appended -
// see loadCACertPool and Init below. Kept separate from Init so it can be
// unit-tested without touching global mysql driver state.
func buildDSN(username, password, protocol, address, database, tlsConfigKey string) string {
	dsn := username + ":" + password + "@" + protocol + "(" + address + ")/" + database

	params := []string{
		"timeout=" + connDialTimeout,
		"readTimeout=" + connReadTimeout,
		"writeTimeout=" + connWriteTimeout,
	}
	if tlsConfigKey != "" {
		params = append(params, "tls="+tlsConfigKey)
	}

	return dsn + "?" + strings.Join(params, "&")
}

// loadCACertPool reads a PEM-encoded CA certificate (or bundle) from
// caCertPath and returns a cert pool containing it, for use as the RootCAs
// of a tls.Config passed to mysql.RegisterTLSConfig. Used to trust a cloud
// database provider's own CA (e.g. Aiven's project CA) instead of a
// publicly-trusted one.
func loadCACertPool(caCertPath string) (*x509.CertPool, error) {
	pem, err := os.ReadFile(caCertPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read CA certificate at %s: %w", caCertPath, err)
	}

	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pem) {
		return nil, fmt.Errorf("failed to parse CA certificate at %s: no valid PEM certificate found", caCertPath)
	}

	return pool, nil
}

// Init opens the database connection and (in devsetup builds) creates any
// missing tables.
//
// If the "dbcacert" environment variable is set to the path of a PEM CA
// certificate, the connection uses TLS validated against that CA - required
// by managed MySQL providers such as Aiven that enforce TLS-only connections
// with their own project CA rather than a publicly-trusted one. When unset,
// behavior is unchanged from a plain, non-TLS connection (e.g. a local or
// docker-compose MariaDB), so existing setups are unaffected.
func Init(username, password, protocol, address, database string) error {
	var err error

	tlsConfigKey := ""
	if caCertPath := os.Getenv("dbcacert"); caCertPath != "" {
		pool, err := loadCACertPool(caCertPath)
		if err != nil {
			return fmt.Errorf("failed to configure database TLS: %w", err)
		}

		if err := mysql.RegisterTLSConfig(tlsConfigName, &tls.Config{RootCAs: pool}); err != nil {
			return fmt.Errorf("failed to register database TLS config: %w", err)
		}

		tlsConfigKey = tlsConfigName
	}

	handle, err = sql.Open("mysql", buildDSN(username, password, protocol, address, database, tlsConfigKey))
	if err != nil {
		return fmt.Errorf("failed to open database connection: %s", err)
	}

	// Recycle connections well before any silent middlebox/provider idle
	// timeout (commonly 5-15 minutes for a connection crossing the public
	// internet) has a chance to drop one from under us, and cap pool size
	// to stay comfortably within a free-tier managed database's connection
	// limit. See connDialTimeout et al. above for the complementary fix -
	// bounding how long a single operation can hang on a connection that
	// went stale anyway.
	handle.SetConnMaxLifetime(3 * time.Minute)
	handle.SetConnMaxIdleTime(1 * time.Minute)
	handle.SetMaxOpenConns(10)
	handle.SetMaxIdleConns(5)

	if os.Getenv("AWS_ENDPOINT_URL_S3") != "" {
		cfg, err := config.LoadDefaultConfig(context.TODO())
		if err != nil {
			return err
		}

		s3client = s3.NewFromConfig(cfg)
	}

	// Conditionally run DB setup (devsetup build tag controls behavior)
	err = MaybeSetupDb(handle)
	if err != nil {
		return err
	}

	return nil
}
