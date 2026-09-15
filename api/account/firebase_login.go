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
)

// Interface for database operations needed for Firebase-based login.
type LoginWithFirebaseStore interface {
	FetchUsernameByFirebaseUid(firebaseUid string) (string, error)
	AddFirebaseAccountRecord(uuid []byte, username string, key, salt []byte, firebaseUid string) error
	AddAccountSession(username string, token []byte) error
}

// LoginWithFirebase verifies idToken as a Firebase ID token for an allowed
// school email (see VerifyFirebaseIDToken), then either logs into the
// account already linked to that Firebase account, or - if none exists yet -
// registers one using nickname as its username. nickname is only used the
// first time a given Firebase account is seen (a returning login ignores
// it); the client only has one to offer right after
// createUserWithEmailAndPassword succeeds, which is also the only time this
// path creates a new account. Returns a session token exactly like Login,
// so nothing downstream of login needs to change.
func LoginWithFirebase[T LoginWithFirebaseStore](store T, idToken, nickname string) (LoginResponse, error) {
	identity, err := VerifyFirebaseIDToken(idToken)
	if err != nil {
		var response LoginResponse
		return response, err
	}

	return loginWithIdentity(store, identity, nickname)
}

// loginWithIdentity is the testable core of LoginWithFirebase, taking an
// already-verified identity directly so tests can exercise the account
// lookup/registration logic without a real Firebase ID token.
func loginWithIdentity[T LoginWithFirebaseStore](store T, identity *FirebaseIdentity, nickname string) (LoginResponse, error) {
	var response LoginResponse

	username, err := store.FetchUsernameByFirebaseUid(identity.UID)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return response, err
		}

		username, err = registerFirebaseAccount(store, identity, nickname)
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

// registerFirebaseAccount creates the account record for a Firebase
// identity seen for the first time, using nickname as its username (the
// same account-identifying name this server already uses everywhere -
// savedata, rankings, admin search, etc.).
//
// This server does not verify that the registering user actually owns
// identity.Email (no confirmation link is sent) - VerifyFirebaseIDToken's
// allowedSchoolEmail pattern check is the only gate on who can create an
// account. That's an accepted tradeoff for this deployment (a single
// school's internal use), not an oversight; tightening it would mean
// requiring Firebase's email verification flow before allowing login.
func registerFirebaseAccount[T LoginWithFirebaseStore](store T, identity *FirebaseIdentity, nickname string) (string, error) {
	if !isValidUsername(nickname) {
		return "", fmt.Errorf("invalid nickname")
	}

	uuid := make([]byte, UUIDSize)
	if _, err := rand.Read(uuid); err != nil {
		return "", fmt.Errorf("failed to generate uuid: %s", err)
	}

	// Password login is disabled server-side for every account (see
	// handleAccountLogin) - Firebase is the sole credential store now, so
	// this key/salt is random, unused, and never revealed. It exists only
	// to satisfy the accounts table's NOT NULL columns, which predate
	// Firebase-based sign-in.
	key := make([]byte, ArgonKeySize)
	if _, err := rand.Read(key); err != nil {
		return "", fmt.Errorf("failed to generate account record: %s", err)
	}
	salt := make([]byte, ArgonSaltSize)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("failed to generate account record: %s", err)
	}

	if err := store.AddFirebaseAccountRecord(uuid, nickname, key, salt, identity.UID); err != nil {
		return "", fmt.Errorf("failed to add account record: %s", err)
	}

	return nickname, nil
}
