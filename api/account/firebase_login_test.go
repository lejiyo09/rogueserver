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

	lookupErr error // if set, FetchUsernameByFirebaseUid always returns this
	addErr    error // if set, AddFirebaseAccountRecord always returns this
}

func newFakeFirebaseStore() *fakeFirebaseStore {
	return &fakeFirebaseStore{
		usernamesByUid: map[string]string{},
		sessions:       map[string][]byte{},
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

	t.Run("FirstSignInWithInvalidNicknameIsRejected", func(t *testing.T) {
		store := newFakeFirebaseStore()

		_, err := loginWithIdentity(store, &FirebaseIdentity{UID: "new-uid", Email: "20260002@hanilgo.cnehs.kr"}, "")
		if err == nil {
			t.Fatal("expected an error for a first-time sign-in with no nickname")
		}
		if _, ok := store.usernamesByUid["new-uid"]; ok {
			t.Error("expected no account to be registered")
		}
	})

	t.Run("UnexpectedLookupErrorPropagates", func(t *testing.T) {
		store := newFakeFirebaseStore()
		store.lookupErr = errors.New("db is on fire")

		_, err := loginWithIdentity(store, &FirebaseIdentity{UID: "some-uid", Email: "20260003@hanilgo.cnehs.kr"}, "nickname")
		if err == nil {
			t.Fatal("expected the lookup error to propagate")
		}
	})

	t.Run("RegistrationFailurePropagates", func(t *testing.T) {
		store := newFakeFirebaseStore()
		store.addErr = errors.New("duplicate key")

		_, err := loginWithIdentity(store, &FirebaseIdentity{UID: "new-uid", Email: "20260004@hanilgo.cnehs.kr"}, "nickname")
		if err == nil {
			t.Fatal("expected the registration error to propagate")
		}
	})
}

func TestRegisterFirebaseAccount(t *testing.T) {
	t.Run("ValidNickname", func(t *testing.T) {
		store := newFakeFirebaseStore()

		username, err := registerFirebaseAccount(store, &FirebaseIdentity{UID: "new-uid", Email: "20260005@hanilgo.cnehs.kr"}, "validNick")
		if err != nil {
			t.Fatalf("expected success, got error: %s", err)
		}
		if username != "validNick" {
			t.Errorf("username = %q, want %q", username, "validNick")
		}
	})

	t.Run("EmptyNicknameIsRejected", func(t *testing.T) {
		store := newFakeFirebaseStore()

		_, err := registerFirebaseAccount(store, &FirebaseIdentity{UID: "new-uid", Email: "20260005@hanilgo.cnehs.kr"}, "")
		if err == nil {
			t.Fatal("expected an error for an empty nickname")
		}
	})

	t.Run("OverlongNicknameIsRejected", func(t *testing.T) {
		store := newFakeFirebaseStore()

		_, err := registerFirebaseAccount(store, &FirebaseIdentity{UID: "new-uid", Email: "20260005@hanilgo.cnehs.kr"}, "thisNicknameIsFarTooLong")
		if err == nil {
			t.Fatal("expected an error for a nickname over the length limit")
		}
	})
}
