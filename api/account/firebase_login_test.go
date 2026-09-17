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
	"database/sql"
	"errors"
	"testing"
)

// fakeFirebaseStore is an in-memory stand-in for LoginWithFirebaseStore.
type fakeFirebaseStore struct {
	usernamesByUid map[string]string
	sessions       map[string][]byte
	cheatsEnabled  map[string]bool

	lookupErr error // if set, FetchUsernameByFirebaseUid always returns this
	addErr    error // if set, AddFirebaseAccountRecord always returns this
}

func newFakeFirebaseStore() *fakeFirebaseStore {
	return &fakeFirebaseStore{
		usernamesByUid: map[string]string{},
		sessions:       map[string][]byte{},
		cheatsEnabled:  map[string]bool{},
	}
}

func (s *fakeFirebaseStore) FetchUsernameByFirebaseUid(firebaseUid string) (string, error) {
	if s.lookupErr != nil {
		return "", s.lookupErr
	}
	username, ok := s.usernamesByUid[firebaseUid]
	if !ok {
		return "", sql.ErrNoRows
	}
	return username, nil
}

func (s *fakeFirebaseStore) AddFirebaseAccountRecord(uuid []byte, username string, key, salt []byte, firebaseUid string) error {
	if s.addErr != nil {
		return s.addErr
	}
	s.usernamesByUid[firebaseUid] = username
	return nil
}

func (s *fakeFirebaseStore) AddAccountSession(username string, token []byte) error {
	s.sessions[username] = token
	return nil
}

func (s *fakeFirebaseStore) SetCheatsEnabledByUsername(username string, enabled bool) error {
	s.cheatsEnabled[username] = enabled
	return nil
}

func TestLoginWithIdentity(t *testing.T) {
	t.Run("ExistingAccountLogsIn", func(t *testing.T) {
		store := newFakeFirebaseStore()
		store.usernamesByUid["existing-uid"] = "existingNickname"

		response, err := loginWithIdentity(store, &FirebaseIdentity{UID: "existing-uid", Email: "20260001@hanilgo.cnehs.kr"}, "")
		if err != nil {
			t.Fatalf("expected success, got error: %s", err)
		}
		if response.Token == "" {
			t.Error("expected a non-empty session token")
		}
		if _, ok := store.sessions["existingNickname"]; !ok {
			t.Error("expected a session to be added for the existing account's username")
		}
	})

	t.Run("FirstSignInRegistersWithNickname", func(t *testing.T) {
		store := newFakeFirebaseStore()

		response, err := loginWithIdentity(store, &FirebaseIdentity{UID: "new-uid", Email: "20260002@hanilgo.cnehs.kr"}, "newNickname")
		if err != nil {
			t.Fatalf("expected success, got error: %s", err)
		}
		if response.Token == "" {
			t.Error("expected a non-empty session token")
		}
		if got := store.usernamesByUid["new-uid"]; got != "newNickname" {
			t.Errorf("username registered for new-uid = %q, want %q", got, "newNickname")
		}
	})

	t.Run("CheatAccountEmailEnablesCheats", func(t *testing.T) {
		store := newFakeFirebaseStore()

		_, err := loginWithIdentity(store, &FirebaseIdentity{UID: "cheat-uid", Email: "20261230@hanilgo.cnehs.kr"}, "cheater")
		if err != nil {
			t.Fatalf("expected success, got error: %s", err)
		}
		if !store.cheatsEnabled["cheater"] {
			t.Error("expected cheatsEnabled to be set for the designated cheat account email")
		}
	})

	t.Run("SecondCheatAccountEmailEnablesCheats", func(t *testing.T) {
		store := newFakeFirebaseStore()

		_, err := loginWithIdentity(store, &FirebaseIdentity{UID: "second-cheat-uid", Email: "20261206@hanilgo.cnehs.kr"}, "secondCheater")
		if err != nil {
			t.Fatalf("expected success, got error: %s", err)
		}
		if !store.cheatsEnabled["secondCheater"] {
			t.Error("expected cheatsEnabled to be set for the second designated cheat account email")
		}
	})

	t.Run("OrdinarySchoolEmailDoesNotEnableCheats", func(t *testing.T) {
		store := newFakeFirebaseStore()

		_, err := loginWithIdentity(store, &FirebaseIdentity{UID: "new-uid", Email: "20260002@hanilgo.cnehs.kr"}, "newNickname")
		if err != nil {
			t.Fatalf("expected success, got error: %s", err)
		}
		if store.cheatsEnabled["newNickname"] {
			t.Error("expected cheatsEnabled to stay false for an ordinary school email")
		}
	})

	t.Run("CheatFlagIsRevokedIfTheAccountsEmailNoLongerMatches", func(t *testing.T) {
		store := newFakeFirebaseStore()
		store.usernamesByUid["existing-uid"] = "existingNickname"
		store.cheatsEnabled["existingNickname"] = true

		_, err := loginWithIdentity(store, &FirebaseIdentity{UID: "existing-uid", Email: "20260001@hanilgo.cnehs.kr"}, "")
		if err != nil {
			t.Fatalf("expected success, got error: %s", err)
		}
		if store.cheatsEnabled["existingNickname"] {
			t.Error("expected cheatsEnabled to be revoked once the email no longer matches")
		}
	})

	t.Run("InvalidNicknameIsRejected", func(t *testing.T) {
		store := newFakeFirebaseStore()

		_, err := loginWithIdentity(store, &FirebaseIdentity{UID: "new-uid", Email: "20260003@hanilgo.cnehs.kr"}, "bad nickname")
		if err == nil {
			t.Fatal("expected invalid nickname to be rejected")
		}
		if errors.Is(err, sql.ErrNoRows) {
			t.Fatal("expected nickname validation error, not account lookup error")
		}
	})
}
