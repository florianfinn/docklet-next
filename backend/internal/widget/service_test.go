// Source: docklet-next/issues/332
// Revision: 14e11ba9c022d57c29bd36c8ed24780763327d0c
// Retrieved: 2026-11-03
// License: Apache-2.0
// Deviations: Original widget cache, encryption and generation regression tests.

package widget

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/testsqlite"
	activitylib "github.com/florianfinn/docklet-next/backend/pkg/libarcane/activity"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	activitydto "github.com/florianfinn/docklet-next/types/activity"
	"github.com/florianfinn/docklet-next/types/user"
	dto "github.com/florianfinn/docklet-next/types/widget"
	"github.com/stretchr/testify/require"
	"go.getarcane.app/sys/crypto"
	"gorm.io/gorm"
)

func testService(t *testing.T) *Service {
	t.Helper()
	db, err := gorm.Open(testsqlite.Dialector(t, filepath.Join(t.TempDir(), "widgets.db")), &gorm.Config{Logger: silentLogger})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&row{}))
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	crypto.InitEncryption(&crypto.Config{Environment: "test", EncryptionKey: "widget-test-key-with-more-than-32-bytes"})
	return NewService(&database.DB{DB: db}, &auditStub{})
}
func configure(t *testing.T, s *Service, k dto.Kind, address string) dto.Config {
	t.Helper()
	c, err := s.Save(t.Context(), k, dto.ConfigUpdate{Enabled: true, Address: address, CredentialAction: "keep"})
	require.NoError(t, err)
	return c
}

func reopen(t *testing.T, s *Service) *Service {
	t.Helper()
	var files []struct {
		Name string
		File string
	}
	require.NoError(t, s.db.Raw("PRAGMA database_list").Scan(&files).Error)
	var path string
	for _, file := range files {
		if file.Name == "main" {
			path = file.File
		}
	}
	require.NotEmpty(t, path)
	old, err := s.db.DB.DB()
	require.NoError(t, err)
	require.NoError(t, old.Close())
	pool, err := sqlitestore.Open(t.Context(), path, true, false)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, pool.Close()) })
	db, err := gorm.Open(sqlitestore.Dialector(pool), &gorm.Config{Logger: silentLogger})
	require.NoError(t, err)
	return NewService(&database.DB{DB: db}, s.activities)
}
func plexBody(title string) string {
	return fmt.Sprintf(`{"MediaContainer":{"size":1,"Metadata":[{"title":%q,"type":"movie","viewOffset":50,"duration":100,"User":{"title":"Viewer"},"Player":{"product":"Plex","title":"TV"},"Media":[{"videoResolution":"1080"}]}]}}`, title)
}
func respond(w http.ResponseWriter, body string) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = io.WriteString(w, body)
}
func wait[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(10 * time.Second):
		t.Fatal("synchronized effect did not arrive")
		var v T
		return v
	}
}
func asyncGet(s *Service, k dto.Kind) <-chan dto.Result {
	ch := make(chan dto.Result, 1)
	go func() { ch <- s.Get(context.Background(), k) }()
	return ch
}

func TestSW3IndependentFlights(t *testing.T) {
	s := testService(t)
	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	var calls atomic.Int32
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		entered <- struct{}{}
		<-release
		respond(w, plexBody("Slow"))
	}))
	defer slow.Close()
	defer close(release)
	fast := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/request/count" {
			respond(w, `{"pending":0,"approved":0,"available":0}`)
		} else {
			respond(w, `{"results":[]}`)
		}
	}))
	defer fast.Close()
	configure(t, s, dto.Plex, slow.URL)
	configure(t, s, dto.Overseerr, fast.URL)
	a := asyncGet(s, dto.Plex)
	wait(t, entered)
	b := asyncGet(s, dto.Plex)
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	result := s.Get(ctx, dto.Overseerr)
	require.Nil(t, result.Error, "a slow Plex flight must not block Overseerr")
	require.NotNil(t, result.Overseerr)
	release <- struct{}{}
	require.Nil(t, wait(t, a).Error)
	require.Nil(t, wait(t, b).Error)
	require.EqualValues(t, 1, calls.Load())
}

func TestSW4BackoffAndStale(t *testing.T) {
	s := testService(t)
	var calls atomic.Int32
	var fail atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if fail.Load() {
			w.WriteHeader(502)
			return
		}
		respond(w, plexBody("Retained"))
	}))
	defer server.Close()
	configure(t, s, dto.Plex, server.URL)
	now := time.Date(2026, 11, 3, 0, 0, 0, 0, time.UTC)
	var tick atomic.Int64
	tick.Store(now.UnixNano())
	s.now = func() time.Time { return time.Unix(0, tick.Load()).UTC() }
	good := s.Get(t.Context(), dto.Plex)
	require.Nil(t, good.Error)
	fail.Store(true)
	tick.Add(int64(15 * time.Second))
	for i, delay := range []time.Duration{30 * time.Second, 60 * time.Second, 2 * time.Minute, 5 * time.Minute, 5 * time.Minute} {
		r := s.Get(t.Context(), dto.Plex)
		require.NotNil(t, r.Error)
		require.Equal(t, dto.BadResponse, r.Error.Code)
		require.True(t, r.Stale)
		require.Equal(t, "Retained", r.Plex.Sessions[0].Title)
		require.Equal(t, good.FetchedAt, r.FetchedAt)
		s.mu.Lock()
		next := s.cache[dto.Plex].Next
		s.mu.Unlock()
		require.Equal(t, s.now().Add(delay), next)
		tick.Add(int64(delay - time.Nanosecond))
		require.True(t, s.Get(t.Context(), dto.Plex).Stale)
		require.EqualValues(t, i+2, calls.Load(), "no retry before the backoff edge")
		tick.Add(1)
	}
	fail.Store(false)
	recovered := s.Get(t.Context(), dto.Plex)
	require.Nil(t, recovered.Error)
	require.False(t, recovered.Stale)
	fresh := testService(t)
	configure(t, fresh, dto.Plex, server.URL)
	fail.Store(true)
	first := fresh.Get(t.Context(), dto.Plex)
	require.NotNil(t, first.Error)
	require.Nil(t, first.Plex)
	require.False(t, first.Stale)
}

func TestSW5SaveFencesOldWaiter(t *testing.T) {
	s := testService(t)
	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	old := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		entered <- struct{}{}
		<-release
		respond(w, plexBody("Old source"))
	}))
	defer old.Close()
	current := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { respond(w, plexBody("New source")) }))
	defer current.Close()
	configure(t, s, dto.Plex, old.URL)
	oldWaiter := asyncGet(s, dto.Plex)
	wait(t, entered)
	configure(t, s, dto.Plex, current.URL)
	newResult := s.Get(t.Context(), dto.Plex)
	require.Nil(t, newResult.Error)
	require.Equal(t, "New source", newResult.Plex.Sessions[0].Title)
	close(release)
	late := wait(t, oldWaiter)
	require.Nil(t, late.Error)
	require.Equal(t, "New source", late.Plex.Sessions[0].Title, "old waiter must restart under the current key")
	require.Equal(t, "New source", s.Get(t.Context(), dto.Plex).Plex.Sessions[0].Title)
}

func TestSW6DeepCopiesAndConcurrentReaders(t *testing.T) {
	s := testService(t)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); respond(w, plexBody("Immutable")) }))
	defer server.Close()
	configure(t, s, dto.Plex, server.URL)
	var wg sync.WaitGroup
	for range 24 {
		wg.Go(func() {
			for range 5 {
				r := s.Get(t.Context(), dto.Plex)
				if r.Error != nil {
					t.Errorf("unexpected error: %v", r.Error)
					return
				}
				r.Plex.Sessions[0].Title = "Consumer mutation"
				*r.FetchedAt = time.Time{}
			}
		})
	}
	wg.Wait()
	a := s.Get(t.Context(), dto.Plex)
	a.Plex.Sessions[0].Title = "Changed again"
	b := s.Get(t.Context(), dto.Plex)
	require.Equal(t, "Immutable", b.Plex.Sessions[0].Title)
	require.False(t, b.FetchedAt.IsZero())
	require.EqualValues(t, 1, calls.Load())
}

func TestSW7EncryptedCredentialsAndRestart(t *testing.T) {
	s := testService(t)
	ctx := t.Context()
	c, err := s.Save(ctx, dto.Plex, dto.ConfigUpdate{Enabled: true, Address: "https://plex.example.com", CredentialAction: "replace", Credential: "synthetic-private-token"})
	require.NoError(t, err)
	require.True(t, c.HasCredential)
	r, err := load(ctx, s.db, dto.Plex)
	require.NoError(t, err)
	require.NotNil(t, r.CredentialEnc)
	require.NotContains(t, *r.CredentialEnc, "synthetic-private-token")
	b, err := json.Marshal(c)
	require.NoError(t, err)
	require.NotContains(t, string(b), "synthetic-private-token")
	require.NotContains(t, string(b), "credentialEnc")
	require.Contains(t, string(b), `"hasCredential":true`)
	kept, err := s.Save(ctx, dto.Plex, dto.ConfigUpdate{Enabled: true, Address: c.Address, CredentialAction: "keep"})
	require.NoError(t, err)
	require.True(t, kept.HasCredential)
	after, err := load(ctx, s.db, dto.Plex)
	require.NoError(t, err)
	require.Equal(t, r.CredentialEnc, after.CredentialEnc)
	// Close the original connection pool and reopen the persisted file.
	restarted := reopen(t, s)
	s = restarted
	after, err = load(ctx, s.db, dto.Plex)
	require.NoError(t, err)
	value, err := secret(after)
	require.NoError(t, err)
	require.Equal(t, "synthetic-private-token", value)
	crypto.InitEncryption(&crypto.Config{Environment: "test", EncryptionKey: "a-different-widget-test-key-more-than-32-bytes"})
	result := restarted.Get(ctx, dto.Plex)
	require.Equal(t, dto.CredentialUnavailable, result.Error.Code)
	crypto.InitEncryption(&crypto.Config{Environment: "test", EncryptionKey: "widget-test-key-with-more-than-32-bytes"})
	raw := "unrecognized-plaintext"
	after.CredentialEnc = &raw
	require.NoError(t, save(ctx, s.db, after))
	_, err = secret(after)
	require.Equal(t, dto.CredentialUnavailable, problem(err).Code)
	cleared, err := s.Save(ctx, dto.Plex, dto.ConfigUpdate{Address: c.Address, CredentialAction: "clear"})
	require.NoError(t, err)
	require.False(t, cleared.HasCredential)
	after, err = load(ctx, s.db, dto.Plex)
	require.NoError(t, err)
	require.Nil(t, after.CredentialEnc)
}

func TestSW7UninitializedEncryptor(t *testing.T) {
	if os.Getenv("WIDGET_UNINITIALIZED_KEY_TEST") == "1" {
		stored := "synthetic-ciphertext"
		_, err := secret(row{CredentialEnc: &stored})
		require.Equal(t, dto.CredentialUnavailable, problem(err).Code)
		return
	}
	executable, err := os.Executable()
	require.NoError(t, err)
	cmd := exec.CommandContext(t.Context(), executable, "-test.run=^TestSW7UninitializedEncryptor$")
	cmd.Env = append(os.Environ(), "WIDGET_UNINITIALIZED_KEY_TEST=1")
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, string(output))
}

type auditStub struct {
	mu       sync.Mutex
	starts   []activitylib.StartRequest
	statuses []activitydto.Status
	fail     bool
}

func (a *auditStub) StartActivity(_ context.Context, r activitylib.StartRequest) (*activitydto.Activity, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.fail {
		return nil, fmt.Errorf("audit unavailable")
	}
	a.starts = append(a.starts, r)
	return &activitydto.Activity{ID: "test-widget-activity"}, nil
}
func (a *auditStub) CompleteActivity(_ context.Context, _ string, status activitydto.Status, _ string, _ *string, _ ...string) (*activitydto.Activity, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.statuses = append(a.statuses, status)
	return &activitydto.Activity{ID: "test-widget-activity", Status: status}, nil
}

func TestSW5ActionGenerationFencesFlight(t *testing.T) {
	s := testService(t)
	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	var requests atomic.Int32
	var approved atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/request/1/approve":
			approved.Store(true)
			respond(w, `{}`)
		case "/api/v1/request/count":
			respond(w, `{"pending":1,"approved":0,"available":0}`)
		case "/api/v1/request":
			if requests.Add(1) == 1 {
				entered <- struct{}{}
				<-release
				respond(w, overseerrList("movie", 1))
				return
			}
			if approved.Load() {
				respond(w, `{"results":[]}`)
			} else {
				respond(w, overseerrList("movie", 1))
			}
		case "/api/v1/movie/1":
			respond(w, `{"title":"Old pending"}`)
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	configure(t, s, dto.Overseerr, server.URL)
	old := asyncGet(s, dto.Overseerr)
	wait(t, entered)
	_, err := s.Action(t.Context(), 1, "approve", &user.Actor{})
	require.NoError(t, err)
	close(release)
	r := wait(t, old)
	require.Nil(t, r.Error)
	require.Empty(t, r.Overseerr.Requests, "pre-action waiter must not receive the old pending list")
	require.GreaterOrEqual(t, requests.Load(), int32(2))
}

func TestSW13ActionsRecordedAndInvalidated(t *testing.T) {
	s := testService(t)
	var posts atomic.Int32
	var denied atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "POST", r.Method)
		require.Contains(t, []string{"/api/v1/request/7/approve", "/api/v1/request/7/decline"}, r.URL.Path)
		posts.Add(1)
		if denied.Load() {
			w.WriteHeader(401)
			respond(w, "never return this upstream error")
			return
		}
		respond(w, `{}`)
	}))
	defer server.Close()
	configure(t, s, dto.Overseerr, server.URL)
	initial := s.generation[dto.Overseerr]
	audit := s.activities.(*auditStub)
	for _, action := range []string{"approve", "decline"} {
		out, err := s.Action(t.Context(), 7, action, &user.Actor{ID: "admin"})
		require.NoError(t, err)
		require.Equal(t, "test-widget-activity", out.ActivityID)
	}
	require.EqualValues(t, 2, posts.Load())
	require.Equal(t, initial+2, s.generation[dto.Overseerr])
	require.Len(t, audit.starts, 2)
	require.Equal(t, "admin", audit.starts[0].StartedBy.ID)
	require.Equal(t, "approve", audit.starts[0].Metadata["action"])
	require.Equal(t, []activitydto.Status{activitydto.StatusSuccess, activitydto.StatusSuccess}, audit.statuses)
	denied.Store(true)
	_, err := s.Action(t.Context(), 7, "decline", &user.Actor{})
	require.Equal(t, dto.AuthFailed, problem(err).Code)
	require.NotContains(t, err.Error(), "upstream")
	require.Equal(t, activitydto.StatusFailed, audit.statuses[2])
	audit.fail = true
	_, err = s.Action(t.Context(), 7, "approve", &user.Actor{})
	require.Equal(t, dto.PersistenceFailed, problem(err).Code)
	require.EqualValues(t, 3, posts.Load(), "audit failure must stop the upstream mutation")
}
