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

type mockGoogleStore struct {
	FetchUsernameFunc func(firebaseUid string) (string, error)
	AddRecordFunc     func(uuid []byte, username string, key, salt []byte, firebaseUid string) error
	AddSessionFunc    func(username string, token []byte) error
}

func (m *mockGoogleStore) FetchUsernameByFirebaseUid(firebaseUid string) (string, error) {
	return m.FetchUsernameFunc(firebaseUid)
}

func (m *mockGoogleStore) AddFirebaseAccountRecord(uuid []byte, username string, key, salt []byte, firebaseUid string) error {
	return m.AddRecordFunc(uuid, username, key, salt, firebaseUid)
}

func (m *mockGoogleStore) AddAccountSession(username string, token []byte) error {
	return m.AddSessionFunc(username, token)
}

func TestLoginWithIdentity(t *testing.T) {
	identity := &FirebaseIdentity{UID: "firebase-uid-1", Email: "20260001@hanilgo.cnehs.kr"}

	t.Run("ExistingAccountLogsIn", func(t *testing.T) {
		addRecordCalled := false
		store := &mockGoogleStore{
			FetchUsernameFunc: func(firebaseUid string) (string, error) {
				if firebaseUid != identity.UID {
					t.Errorf("FetchUsernameByFirebaseUid called with %q, want %q", firebaseUid, identity.UID)
				}
				return "20260001", nil
			},
			AddRecordFunc: func(uuid []byte, username string, key, salt []byte, firebaseUid string) error {
				addRecordCalled = true
				return nil
			},
			AddSessionFunc: func(username string, token []byte) error { return nil },
		}

		resp, err := loginWithIdentity(store, identity)
		if err != nil {
			t.Fatalf("expected success, got error: %s", err)
		}
		if resp.Token == "" {
			t.Error("expected a token to be set")
		}
		if addRecordCalled {
			t.Error("expected no new account record for an already-linked Google account")
		}
	})

	t.Run("FirstSignInAutoRegisters", func(t *testing.T) {
		var addedUsername, addedFirebaseUid string
		store := &mockGoogleStore{
			FetchUsernameFunc: func(firebaseUid string) (string, error) {
				return "", sql.ErrNoRows
			},
			AddRecordFunc: func(uuid []byte, username string, key, salt []byte, firebaseUid string) error {
				addedUsername = username
				addedFirebaseUid = firebaseUid
				if len(uuid) != UUIDSize {
					t.Errorf("uuid length = %d, want %d", len(uuid), UUIDSize)
				}
				if len(key) != ArgonKeySize || len(salt) != ArgonSaltSize {
					t.Errorf("placeholder key/salt have unexpected lengths: %d/%d", len(key), len(salt))
				}
				return nil
			},
			AddSessionFunc: func(username string, token []byte) error { return nil },
		}

		resp, err := loginWithIdentity(store, identity)
		if err != nil {
			t.Fatalf("expected success, got error: %s", err)
		}
		if resp.Token == "" {
			t.Error("expected a token to be set")
		}
		if addedUsername != "20260001" {
			t.Errorf("registered username = %q, want %q (the email's local-part)", addedUsername, "20260001")
		}
		if addedFirebaseUid != identity.UID {
			t.Errorf("registered firebaseUid = %q, want %q", addedFirebaseUid, identity.UID)
		}
	})

	t.Run("UnexpectedLookupErrorPropagates", func(t *testing.T) {
		wantErr := errors.New("connection reset")
		store := &mockGoogleStore{
			FetchUsernameFunc: func(firebaseUid string) (string, error) {
				return "", wantErr
			},
		}

		_, err := loginWithIdentity(store, identity)
		if !errors.Is(err, wantErr) {
			t.Errorf("expected the lookup error to propagate, got: %v", err)
		}
	})

	t.Run("RegistrationFailurePropagates", func(t *testing.T) {
		store := &mockGoogleStore{
			FetchUsernameFunc: func(firebaseUid string) (string, error) {
				return "", sql.ErrNoRows
			},
			AddRecordFunc: func(uuid []byte, username string, key, salt []byte, firebaseUid string) error {
				return errors.New("username already taken")
			},
		}

		_, err := loginWithIdentity(store, identity)
		if err == nil {
			t.Fatal("expected an error when account creation fails")
		}
	})
}
