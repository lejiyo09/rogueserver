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
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// Interface for database operations needed for Google-based login.
type LoginWithGoogleStore interface {
	FetchUsernameByFirebaseUid(firebaseUid string) (string, error)
	AddFirebaseAccountRecord(uuid []byte, username string, key, salt []byte, firebaseUid string) error
	AddAccountSession(username string, token []byte) error
}

// LoginWithGoogle verifies idToken as a Firebase ID token for an allowed
// school Google account (see VerifyFirebaseIDToken), then either logs into
// the account already linked to that Google account, or - on a first sign-in -
// auto-registers one, since a fresh Google sign-in and a first-time
// registration are the same action for this login method. Returns a session
// token exactly like Login, so nothing downstream of login needs to change.
func LoginWithGoogle[T LoginWithGoogleStore](store T, idToken string) (LoginResponse, error) {
	identity, err := VerifyFirebaseIDToken(idToken)
	if err != nil {
		var response LoginResponse
		return response, err
	}

	return loginWithIdentity(store, identity)
}

// loginWithIdentity is the testable core of LoginWithGoogle, taking an
// already-verified identity directly so tests can exercise the account
// lookup/auto-registration logic without a real Firebase ID token.
func loginWithIdentity[T LoginWithGoogleStore](store T, identity *FirebaseIdentity) (LoginResponse, error) {
	var response LoginResponse

	username, err := store.FetchUsernameByFirebaseUid(identity.UID)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return response, err
		}

		username, err = registerGoogleAccount(store, identity)
		if err != nil {
			return response, err
		}
	}

	response.Token, err = GenerateTokenForUsername(store, username)
	if err != nil {
		return response, fmt.Errorf("failed to generate token: %s", err)
	}

	return response, nil
}

func registerGoogleAccount[T LoginWithGoogleStore](store T, identity *FirebaseIdentity) (string, error) {
	username := strings.SplitN(identity.Email, "@", 2)[0]

	uuid := make([]byte, UUIDSize)
	if _, err := rand.Read(uuid); err != nil {
		return "", fmt.Errorf("failed to generate uuid: %s", err)
	}

	// Password login is disabled server-side for every account (see
	// handleAccountLogin), so this key/salt is random, unused, and never
	// revealed - it exists only to satisfy the accounts table's NOT NULL
	// columns, which predate Google-only sign-in.
	key := make([]byte, ArgonKeySize)
	if _, err := rand.Read(key); err != nil {
		return "", fmt.Errorf("failed to generate account record: %s", err)
	}
	salt := make([]byte, ArgonSaltSize)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("failed to generate account record: %s", err)
	}

	if err := store.AddFirebaseAccountRecord(uuid, username, key, salt, identity.UID); err != nil {
		return "", fmt.Errorf("failed to add account record: %s", err)
	}

	return username, nil
}
