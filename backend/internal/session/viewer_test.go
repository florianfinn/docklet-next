// Source: docklet-next/docs/design/runtime-control.md
// Revision: f0447218f504f481d9e941df04bb6a19716ed6d7203060744204c44c3cc7cda8
// Retrieved: 2026-10-20
// License: Apache-2.0
// Deviations: Original session-bound viewer and browser identity lifecycle implementation.

package session

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/user"
	"github.com/florianfinn/docklet-next/backend/pkg/sessionlife"
	"github.com/florianfinn/docklet-next/types/auth"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestSessionViewerSeparateServicesAndDurableBypasses(t *testing.T) {
	for _, mutation := range []string{"revoke", "all", "except", "transaction", "rollback", "session-delete", "user-delete", "expiry", "unreadable"} {
		t.Run(mutation, func(t *testing.T) {
			db := setupAuthServiceTestDB(t)
			u := &user.User{Username: "example-user"}
			require.NoError(t, db.Create(u).Error)
			owner, other := NewSessionService(db), NewSessionService(db)
			record, _, err := owner.CreateSession(t.Context(), u.ID, time.Now().Add(time.Hour), auth.SessionMeta{})
			require.NoError(t, err)
			spared, _, err := owner.CreateSession(t.Context(), u.ID, time.Now().Add(time.Hour), auth.SessionMeta{})
			require.NoError(t, err)
			ctx := owner.ViewerContext(t.Context(), record.ID, u.ID)
			closed := false
			l, err := sessionlife.Register(ctx, func() { closed = true })
			require.NoError(t, err)
			defer l.Release()
			second := false
			l2, err := sessionlife.Register(ctx, func() { second = true })
			require.NoError(t, err)
			defer l2.Release()
			survivor := false
			l3, err := sessionlife.Register(owner.ViewerContext(t.Context(), spared.ID, u.ID), func() { survivor = true })
			require.NoError(t, err)
			defer l3.Release()
			immediate := false
			switch mutation {
			case "revoke":
				require.NoError(t, other.RevokeSession(t.Context(), record.ID))
				immediate = true
			case "all":
				require.NoError(t, other.RevokeAllSessions(t.Context()))
				immediate = true
			case "except":
				require.NoError(t, other.RevokeAllUserSessionsExcept(t.Context(), u.ID, spared.ID))
				immediate = true
			case "transaction", "rollback":
				err := db.Transaction(func(tx *gorm.DB) error {
					if err := RevokeAllUserSessionsExceptInDB(t.Context(), tx, u.ID, spared.ID); err != nil {
						return err
					}
					if mutation == "rollback" {
						return errors.New("rollback")
					}
					return nil
				})
				require.Equal(t, mutation == "rollback", err != nil)
			case "session-delete":
				require.NoError(t, db.Delete(record).Error)
			case "user-delete":
				require.NoError(t, db.Delete(u).Error)
			case "expiry":
				require.NoError(t, db.Model(record).Update("expires_at", time.Now().Add(-time.Hour)).Error)
			case "unreadable":
				require.NoError(t, db.Exec("DROP TABLE user_sessions").Error)
			}
			if immediate {
				require.True(t, closed)
				require.True(t, second)
			} else {
				require.False(t, closed, "uncommitted/bypassing writes never claim immediate revocation")
			}
			err = sessionlife.Check(ctx)
			if mutation == "rollback" {
				require.NoError(t, err)
				require.False(t, closed)
			} else {
				require.Error(t, err)
				l.Close()
				require.True(t, closed)
			}
			require.Equal(t, mutation == "all", survivor)
		})
	}
}

func TestSessionViewerFailedDurableRevokeDoesNotClose(t *testing.T) {
	db := setupAuthServiceTestDB(t)
	u := &user.User{Username: "example-failure"}
	require.NoError(t, db.Create(u).Error)
	s := NewSessionService(db)
	record, _, err := s.CreateSession(t.Context(), u.ID, time.Now().Add(time.Hour), auth.SessionMeta{})
	require.NoError(t, err)
	closed := false
	l, err := sessionlife.Register(s.ViewerContext(context.Background(), record.ID, u.ID), func() { closed = true })
	require.NoError(t, err)
	defer l.Release()
	require.NoError(t, db.Exec("CREATE TRIGGER refuse_revoke BEFORE UPDATE ON user_sessions BEGIN SELECT RAISE(ABORT, 'example refusal'); END").Error)
	require.Error(t, s.RevokeSession(t.Context(), record.ID))
	require.False(t, closed)
	require.NoError(t, sessionlife.Check(s.ViewerContext(t.Context(), record.ID, u.ID)))
}

func TestSessionViewerRegisterRevokeRaceUsesDurableState(t *testing.T) {
	db := setupAuthServiceTestDB(t)
	u := &user.User{Username: "example-register-race"}
	require.NoError(t, db.Create(u).Error)
	owner := NewSessionService(db)
	record, _, err := owner.CreateSession(t.Context(), u.ID, time.Now().Add(time.Hour), auth.SessionMeta{})
	require.NoError(t, err)
	entered, proceed, closed := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var once sync.Once
	require.NoError(t, db.Callback().Query().Before("gorm:query").Register("example-session-barrier", func(tx *gorm.DB) {
		if tx.Statement.Table == "user_sessions" {
			once.Do(func() { close(entered); <-proceed })
		}
	}))
	done := make(chan error, 1)
	go func() {
		_, err := sessionlife.Register(owner.ViewerContext(t.Context(), record.ID, u.ID), func() { close(closed) })
		done <- err
	}()
	<-entered
	require.NoError(t, NewSessionService(db).RevokeSession(t.Context(), record.ID))
	select {
	case <-closed:
	default:
		t.Error("revoke must see a viewer before its durable read")
	}
	close(proceed)
	require.Error(t, <-done, "revocation winning acceptance must reject before any data")
}
