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

package account

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// FirebaseProjectID is the Firebase project this server accepts ID tokens
// for (the "projectId" field of the client's firebaseConfig). Login via
// Firebase is disabled (VerifyFirebaseIDToken always fails) while this is empty.
var FirebaseProjectID string

// allowedSchoolEmail restricts accounts to this school's email shape:
// <4-digit year><4-digit id>@hanilgo.cnehs.kr (e.g. 20260001@hanilgo.cnehs.kr).
// A token for any other email, including a real hanilgo.cnehs.kr address
// with a different local-part shape, is rejected. This is checked as
// written at the email the account was registered with (Firebase's
// email/password provider here, not a third-party identity provider) -
// email ownership is intentionally not verified; see registerFirebaseAccount.
var allowedSchoolEmail = regexp.MustCompile(`^2026\d{4}@hanilgo\.cnehs\.kr$`)

// cheatAccountEmails are the school emails allowed full gameplay-cheat
// access (every starter/item unlocked, unlimited money, in-game level/nature
// editing) - see loginWithIdentity, which resets every account's
// cheatsEnabled flag to match this list on every login.
var cheatAccountEmails = []string{
	"20261230@hanilgo.cnehs.kr",
	"20261206@hanilgo.cnehs.kr",
}

func isCheatAccountEmail(email string) bool {
	for _, allowed := range cheatAccountEmails {
		if email == allowed {
			return true
		}
	}
	return false
}

// firebaseCertsURL serves Google's current public keys for verifying
// Firebase Auth ID token signatures, keyed by "kid". See
// https://firebase.google.com/docs/auth/admin/verify-id-tokens#verify_id_tokens_using_a_third-party_jwt_library
const firebaseCertsURL = "https://www.googleapis.com/service_accounts/v1/metadata/x509/securetoken@system.gserviceaccount.com"

const certCacheTTL = time.Hour

// firebaseCertCache caches Google's public certs so a normal login doesn't
// need a network round trip of its own; refreshed at most once per certCacheTTL.
type firebaseCertCache struct {
	mu      sync.RWMutex
	keys    map[string]*rsa.PublicKey
	fetched time.Time
}

var certCache firebaseCertCache

func (c *firebaseCertCache) get(kid string) (*rsa.PublicKey, error) {
	c.mu.RLock()
	key, ok := c.keys[kid]
	stale := time.Since(c.fetched) > certCacheTTL
	c.mu.RUnlock()

	if ok && !stale {
		return key, nil
	}

	if err := c.refresh(); err != nil {
		if ok {
			// Serve the stale-but-still-known key rather than fail a login
			// outright over a transient refresh error.
			return key, nil
		}
		return nil, err
	}

	c.mu.RLock()
	defer c.mu.RUnlock()
	key, ok = c.keys[kid]
	if !ok {
		return nil, fmt.Errorf("no matching Firebase public key for kid %q", kid)
	}
	return key, nil
}

func (c *firebaseCertCache) refresh() error {
	resp, err := http.Get(firebaseCertsURL)
	if err != nil {
		return fmt.Errorf("failed to fetch Firebase public certs: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to fetch Firebase public certs: status %d", resp.StatusCode)
	}

	var certsByKid map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&certsByKid); err != nil {
		return fmt.Errorf("failed to decode Firebase public certs: %w", err)
	}

	keys := make(map[string]*rsa.PublicKey, len(certsByKid))
	for kid, certPEM := range certsByKid {
		key, err := parseRSAPublicKeyFromCertPEM(certPEM)
		if err != nil {
			continue
		}
		keys[kid] = key
	}

	c.mu.Lock()
	c.keys = keys
	c.fetched = time.Now()
	c.mu.Unlock()

	return nil
}

func parseRSAPublicKeyFromCertPEM(certPEM string) (*rsa.PublicKey, error) {
	block, _ := pem.Decode([]byte(certPEM))
	if block == nil {
		return nil, errors.New("failed to decode PEM block")
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse certificate: %w", err)
	}

	key, ok := cert.PublicKey.(*rsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("certificate public key is %T, not RSA", cert.PublicKey)
	}

	return key, nil
}

// FirebaseIdentity is the verified identity extracted from a Firebase ID token.
type FirebaseIdentity struct {
	UID   string
	Email string
}

// VerifyFirebaseIDToken cryptographically verifies idToken against Google's
// published Firebase Auth public keys for FirebaseProjectID, then checks
// the token's email against allowedSchoolEmail. This is the sole trust
// boundary for Firebase-based login - a token that fails any of these
// checks must never be treated as authenticating anyone.
func VerifyFirebaseIDToken(idToken string) (*FirebaseIdentity, error) {
	return verifyFirebaseIDToken(idToken, FirebaseProjectID, certCache.get)
}

// verifyFirebaseIDToken is the testable core of VerifyFirebaseIDToken, with
// the public-key lookup injected so tests can verify against a key pair
// they control instead of Google's real, live endpoint.
func verifyFirebaseIDToken(idToken, projectID string, lookupKey func(kid string) (*rsa.PublicKey, error)) (*FirebaseIdentity, error) {
	if projectID == "" {
		return nil, errors.New("firebase project id is not configured")
	}

	token, err := jwt.Parse(idToken, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		kid, ok := t.Header["kid"].(string)
		if !ok || kid == "" {
			return nil, errors.New("token has no kid header")
		}
		return lookupKey(kid)
	},
		jwt.WithValidMethods([]string{"RS256"}),
		jwt.WithIssuer("https://securetoken.google.com/"+projectID),
		jwt.WithAudience(projectID),
		jwt.WithExpirationRequired(),
	)
	if err != nil {
		return nil, fmt.Errorf("invalid sign-in token: %w", err)
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, errors.New("invalid token claims")
	}

	sub, err := claims.GetSubject()
	if err != nil || sub == "" {
		return nil, errors.New("token has no subject")
	}

	// Deliberately not requiring claims["email_verified"]: this account's
	// email is never confirmed to actually belong to the registering user
	// (no verification link is sent) - see registerFirebaseAccount's doc
	// comment for why that's an accepted tradeoff here.
	email, _ := claims["email"].(string)
	if !allowedSchoolEmail.MatchString(email) {
		return nil, fmt.Errorf("%q is not an allowed school email", email)
	}

	return &FirebaseIdentity{UID: sub, Email: email}, nil
}
