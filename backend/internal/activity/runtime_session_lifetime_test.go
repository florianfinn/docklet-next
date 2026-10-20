// Source: docklet-next/docs/design/runtime-control.md
// Revision: f0447218f504f481d9e941df04bb6a19716ed6d7203060744204c44c3cc7cda8
// Retrieved: 2026-10-20
// License: Apache-2.0
// Deviations: Original session revocation proof against the accepted runtime owner.

package activity

import (
	"context"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/session"
	"github.com/florianfinn/docklet-next/backend/internal/user"
	"github.com/florianfinn/docklet-next/backend/pkg/sessionlife"
	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	"github.com/florianfinn/docklet-next/types/auth"
	"github.com/stretchr/testify/require"
)

func TestSessionRevocationKeepsAcceptedExecution(t *testing.T) {
	entered, release := make(chan context.Context, 1), make(chan struct{})
	unblock := sync.OnceFunc(func() { close(release) })
	defer unblock()
	f := b2Fixture(t, t.Context(), func(ctx context.Context, _ RuntimeCommand, _ io.Writer) error {
		entered <- ctx
		<-release
		return ctx.Err()
	})
	db := f.store.svc.db
	actor := &user.User{Username: "example-session-actor"}
	require.NoError(t, db.Create(actor).Error)
	sessions := session.NewSessionService(db)
	record, _, err := sessions.CreateSession(t.Context(), actor.ID, time.Now().Add(time.Hour), auth.SessionMeta{})
	require.NoError(t, err)
	other, _, err := sessions.CreateSession(t.Context(), actor.ID, time.Now().Add(time.Hour), auth.SessionMeta{})
	require.NoError(t, err)
	request, cancel := context.WithCancel(sessions.ViewerContext(t.Context(), record.ID, actor.ID))
	defer cancel()
	viewer, err := sessionlife.Register(request, cancel)
	require.NoError(t, err)
	defer viewer.Release()
	otherClosed := false
	otherViewer, err := sessionlife.Register(sessions.ViewerContext(t.Context(), other.ID, actor.ID), func() { otherClosed = true })
	require.NoError(t, err)
	defer otherViewer.Release()
	command := f.command()
	command.Actor.ID = actor.ID
	admitted, err := f.service.Admit(request, command)
	require.NoError(t, err)
	var work context.Context
	select {
	case work = <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("admitted execution did not start")
	}
	require.NoError(t, session.NewSessionService(db).RevokeSession(t.Context(), record.ID))
	require.ErrorIs(t, request.Err(), context.Canceled)
	require.NoError(t, work.Err(), "accepted executor must remain application-owned")
	require.False(t, otherClosed)
	b2Held(t, f, command, admitted.ActivityID)
	unblock()
	settled, err := b2Wait(t, f, command, admitted.ActivityID)
	require.NoError(t, err)
	require.Equal(t, activitytypes.StatusSuccess, settled.Status)
	b2FinalCount(t, f, admitted.ActivityID, 1)
}
