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
	"crypto/rand"
	"crypto/rsa"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const testProjectID = "pokerogue-1ed9c"

// testTokenClaims are the fields a real test case cares about overriding;
// signTestToken fills in sane defaults (a valid, current, correctly-issued
// token for testProjectID) for anything left zero-valued.
type testTokenClaims struct {
	subject       string
	email         string
	emailVerified *bool // nil defaults to true
	issuer        string
	audience      string
	expiresAt     time.Time
	kid           string
}

func signTestToken(t *testing.T, key *rsa.PrivateKey, overrides testTokenClaims) string {
	t.Helper()

	claims := testTokenClaims{
		subject:   "test-uid-123",
		email:     "20260001@hanilgo.cnehs.kr",
		issuer:    "https://securetoken.google.com/" + testProjectID,
		audience:  testProjectID,
		expiresAt: time.Now().Add(time.Hour),
		kid:       "test-kid",
	}
	if overrides.subject != "" {
		claims.subject = overrides.subject
	}
	if overrides.email != "" {
		claims.email = overrides.email
	}
	if overrides.issuer != "" {
		claims.issuer = overrides.issuer
	}
	if overrides.audience != "" {
		claims.audience = overrides.audience
	}
	if !overrides.expiresAt.IsZero() {
		claims.expiresAt = overrides.expiresAt
	}
	if overrides.kid != "" {
		claims.kid = overrides.kid
	}
	emailVerified := true
	if overrides.emailVerified != nil {
		emailVerified = *overrides.emailVerified
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"sub":            claims.subject,
		"email":          claims.email,
		"email_verified": emailVerified,
		"iss":            claims.issuer,
		"aud":            claims.audience,
		"exp":            claims.expiresAt.Unix(),
		"iat":            time.Now().Add(-time.Minute).Unix(),
	})
	token.Header["kid"] = claims.kid

	signed, err := token.SignedString(key)
	if err != nil {
		t.Fatalf("failed to sign test token: %s", err)
	}
	return signed
}

func TestVerifyFirebaseIDToken(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate test key: %s", err)
	}
	otherKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate second test key: %s", err)
	}

	lookupKey := func(kid string) (*rsa.PublicKey, error) {
		if kid == "test-kid" {
			return &key.PublicKey, nil
		}
		return nil, errNoSuchKey
	}

	t.Run("ValidToken", func(t *testing.T) {
		tokenStr := signTestToken(t, key, testTokenClaims{})

		identity, err := verifyFirebaseIDToken(tokenStr, testProjectID, lookupKey)
		if err != nil {
			t.Fatalf("expected success, got error: %s", err)
		}
		if identity.UID != "test-uid-123" {
			t.Errorf("UID = %q, want %q", identity.UID, "test-uid-123")
		}
		if identity.Email != "20260001@hanilgo.cnehs.kr" {
			t.Errorf("Email = %q, want %q", identity.Email, "20260001@hanilgo.cnehs.kr")
		}
	})

	t.Run("ProjectIDNotConfigured", func(t *testing.T) {
		tokenStr := signTestToken(t, key, testTokenClaims{})

		_, err := verifyFirebaseIDToken(tokenStr, "", lookupKey)
		if err == nil {
			t.Fatal("expected an error when the project id isn't configured")
		}
	})

	t.Run("WrongSigningKey", func(t *testing.T) {
		tokenStr := signTestToken(t, otherKey, testTokenClaims{})

		_, err := verifyFirebaseIDToken(tokenStr, testProjectID, lookupKey)
		if err == nil {
			t.Fatal("expected signature verification to fail for a token signed with an untrusted key")
		}
	})

	t.Run("WrongIssuer", func(t *testing.T) {
		tokenStr := signTestToken(t, key, testTokenClaims{issuer: "https://evil.example/" + testProjectID})

		_, err := verifyFirebaseIDToken(tokenStr, testProjectID, lookupKey)
		if err == nil {
			t.Fatal("expected an error for a mismatched issuer")
		}
	})

	t.Run("WrongAudience", func(t *testing.T) {
		tokenStr := signTestToken(t, key, testTokenClaims{audience: "some-other-project"})

		_, err := verifyFirebaseIDToken(tokenStr, testProjectID, lookupKey)
		if err == nil {
			t.Fatal("expected an error for a mismatched audience")
		}
	})

	t.Run("Expired", func(t *testing.T) {
		tokenStr := signTestToken(t, key, testTokenClaims{expiresAt: time.Now().Add(-time.Minute)})

		_, err := verifyFirebaseIDToken(tokenStr, testProjectID, lookupKey)
		if err == nil {
			t.Fatal("expected an error for an expired token")
		}
	})

	t.Run("EmailNotVerified", func(t *testing.T) {
		unverified := false
		tokenStr := signTestToken(t, key, testTokenClaims{emailVerified: &unverified})

		_, err := verifyFirebaseIDToken(tokenStr, testProjectID, lookupKey)
		if err == nil {
			t.Fatal("expected an error for an unverified email")
		}
	})

	t.Run("EmailWrongDomain", func(t *testing.T) {
		tokenStr := signTestToken(t, key, testTokenClaims{email: "20260001@gmail.com"})

		_, err := verifyFirebaseIDToken(tokenStr, testProjectID, lookupKey)
		if err == nil {
			t.Fatal("expected an error for an email outside the school domain")
		}
	})

	t.Run("EmailWrongLocalPartShape", func(t *testing.T) {
		for _, email := range []string{
			"student@hanilgo.cnehs.kr",   // not <year><id>
			"20250001@hanilgo.cnehs.kr",  // wrong year prefix
			"2026001@hanilgo.cnehs.kr",   // one digit short
			"202600001@hanilgo.cnehs.kr", // one digit too many
		} {
			tokenStr := signTestToken(t, key, testTokenClaims{email: email})

			_, err := verifyFirebaseIDToken(tokenStr, testProjectID, lookupKey)
			if err == nil {
				t.Errorf("email %q: expected an error, got none", email)
			}
		}
	})

	t.Run("EmailRightShapeIsAllowed", func(t *testing.T) {
		tokenStr := signTestToken(t, key, testTokenClaims{email: "20269999@hanilgo.cnehs.kr"})

		_, err := verifyFirebaseIDToken(tokenStr, testProjectID, lookupKey)
		if err != nil {
			t.Errorf("expected this email shape to be allowed, got error: %s", err)
		}
	})

	t.Run("UnexpectedSigningMethod", func(t *testing.T) {
		// HS256 (symmetric) tokens must never be accepted for an RS256-only
		// verifier - otherwise anyone could forge a token using the public
		// key itself as an HMAC secret.
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
			"sub": "attacker", "email": "20260001@hanilgo.cnehs.kr", "email_verified": true,
			"iss": "https://securetoken.google.com/" + testProjectID, "aud": testProjectID,
			"exp": time.Now().Add(time.Hour).Unix(),
		})
		token.Header["kid"] = "test-kid"
		tokenStr, err := token.SignedString([]byte("any-secret-the-attacker-knows"))
		if err != nil {
			t.Fatalf("failed to sign forged token: %s", err)
		}

		_, err = verifyFirebaseIDToken(tokenStr, testProjectID, lookupKey)
		if err == nil {
			t.Fatal("expected a non-RS256 token to be rejected")
		}
	})
}

var errNoSuchKey = jwt.ErrTokenUnverifiable
