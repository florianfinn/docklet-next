// Source: docs/design/proxy-module.md
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-15
// License: Apache-2.0
// Deviations: Original S2 R4 guard proofs on the Docker file seam, recovery epochs, active operations, receipts
// and journal decoding; several cases adapted from the R4 review probes.

package proxy

import (
	"archive/tar"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/config"
	dockerinternal "github.com/florianfinn/docklet-next/backend/internal/docker"
	dto "github.com/florianfinn/docklet-next/types/proxy"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/stretchr/testify/require"
)

// fileSeam serves a project-source container whose archive endpoint reads the test directory.
func fileSeam(t *testing.T, x *fixture, root string) *DockerRuntime {
	t.Helper()
	c := container.InspectResponse{ID: x.request.Instance.Identity.ContainerID, Name: "/example-proxy", State: &container.State{Running: true, StartedAt: initialEpoch}, Mounts: []container.MountPoint{{Type: mount.TypeBind, Source: root, Destination: "/etc/proxy", RW: true}}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := regexp.MustCompile(`^/v[0-9.]+`).ReplaceAllString(r.URL.Path, "")
		switch {
		case p == "/_ping":
			w.Header().Set("API-Version", "1.54")
			_, _ = w.Write([]byte("OK"))
		case strings.HasSuffix(p, "/json"):
			_ = json.NewEncoder(w).Encode(c)
		case strings.HasSuffix(p, "/archive"):
			source := r.URL.Query().Get("path")
			mode := os.FileMode(0644)
			if source == "/etc/proxy" {
				mode |= os.ModeDir
			}
			raw, _ := json.Marshal(container.PathStat{Name: path.Base(source), Mode: mode})
			w.Header().Set("X-Docker-Container-Path-Stat", base64.StdEncoding.EncodeToString(raw))
			if r.Method == http.MethodHead {
				return
			}
			b, err := os.ReadFile(filepath.Join(root, strings.TrimPrefix(source, "/etc/proxy/")))
			if err != nil {
				w.WriteHeader(404)
				return
			}
			tw := tar.NewWriter(w)
			_ = tw.WriteHeader(&tar.Header{Name: path.Base(source), Typeflag: tar.TypeReg, Mode: 0644, Size: int64(len(b))})
			_, _ = tw.Write(b)
			_ = tw.Close()
		default:
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(server.Close)
	return &DockerRuntime{Docker: dockerinternal.NewDockerClientService(t.Context(), nil, &config.Config{DockerHost: server.URL}, nil), Settings: x.settings, DB: x.db, Engines: Engines{dto.Traefik: x.e}}
}
func fileIntent(t *testing.T, x *fixture, root string) Intent {
	t.Helper()
	require.NoError(t, x.settings.UpdateSetting(t.Context(), "projectsDirectory", root))
	q := x.request
	q.Instance.Source = dto.Source{Kind: dto.ProjectSource, Canonical: root, MountTarget: "/etc/proxy"}
	i := Intent{Request: q, Old: Snapshot([]byte("old"), true), New: Snapshot([]byte("new"), true), Mode: 0644, GraphHashes: map[string]string{}}
	paths(&i)
	return i
}

// Publish and Restore on the real file seam never overwrite a target in a third state.
func TestPublishAndRestorePreserveOperatorBytes(t *testing.T) {
	for _, op := range []string{"publish", "restore"} {
		t.Run(op, func(t *testing.T) {
			x := newFixture(t)
			root := t.TempDir()
			i := fileIntent(t, x, root)
			require.NoError(t, os.WriteFile(filepath.Join(root, i.Request.Target), []byte("operator"), 0644))
			require.NoError(t, os.WriteFile(filepath.Join(root, i.TempPath), i.New.Content, 0644))
			d := fileSeam(t, x, root)
			var err error
			if op == "publish" {
				err = d.Publish(t.Context(), i)
				requireCode(t, err, dto.StaleHash)
			} else {
				err = d.Restore(t.Context(), i)
				requireCode(t, err, dto.TargetChanged)
			}
			b, readErr := os.ReadFile(filepath.Join(root, i.Request.Target))
			require.NoError(t, readErr)
			require.Equal(t, "operator", string(b))
			temp, readErr := os.ReadFile(filepath.Join(root, i.TempPath))
			require.NoError(t, readErr)
			require.Equal(t, "new", string(temp), "a refused operation writes nothing")
		})
	}
}

// Restore on the real file seam: an old target needs nothing, a new target gets the old bytes back.
func TestRestoreFollowsOldAndNewTargets(t *testing.T) {
	for _, current := range []string{"old", "new"} {
		t.Run(current, func(t *testing.T) {
			x := newFixture(t)
			root := t.TempDir()
			i := fileIntent(t, x, root)
			require.NoError(t, os.WriteFile(filepath.Join(root, i.Request.Target), []byte(current), 0644))
			require.NoError(t, fileSeam(t, x, root).Restore(t.Context(), i))
			b, err := os.ReadFile(filepath.Join(root, i.Request.Target))
			require.NoError(t, err)
			require.Equal(t, "old", string(b))
		})
	}
}

// Cleanup returns an untyped Docker failure unchanged instead of trying a rebind.
func TestCleanupKeepsUntypedWriterError(t *testing.T) {
	x := newFixture(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if regexp.MustCompile(`^/v[0-9.]+`).ReplaceAllString(r.URL.Path, "") == "/_ping" {
			w.Header().Set("API-Version", "1.54")
			_, _ = w.Write([]byte("OK"))
			return
		}
		w.WriteHeader(500)
	}))
	defer server.Close()
	d := &DockerRuntime{Docker: dockerinternal.NewDockerClientService(t.Context(), nil, &config.Config{DockerHost: server.URL}, nil), Settings: x.settings, DB: x.db}
	i := Intent{Request: x.request}
	paths(&i)
	var err error
	require.NotPanics(t, func() { err = d.Cleanup(t.Context(), i) })
	require.Error(t, err)
	var p *dto.Problem
	require.False(t, errors.As(err, &p), "the original untyped error is returned")
}

// Publish refuses a changed configuration graph although the target itself is unchanged.
func TestPublishRefusesChangedGraph(t *testing.T) {
	x := newFixture(t)
	root := t.TempDir()
	i := fileIntent(t, x, root)
	require.NoError(t, os.WriteFile(filepath.Join(root, i.Request.Target), i.Old.Content, 0644))
	require.NoError(t, os.WriteFile(filepath.Join(root, i.TempPath), i.New.Content, 0644))
	i.GraphHashes = map[string]string{"example-other.yml": strings.Repeat("a", 64)}
	err := fileSeam(t, x, root).Publish(t.Context(), i)
	requireCode(t, err, dto.StaleHash)
	b, readErr := os.ReadFile(filepath.Join(root, i.Request.Target))
	require.NoError(t, readErr)
	require.Equal(t, "old", string(b))
}

// Rebind refuses a container without State and a partially decoded inspect answer.
func TestRebindRefusesIncompleteInspect(t *testing.T) {
	t.Run("missing state", func(t *testing.T) {
		x := newFixture(t)
		root := t.TempDir()
		require.NoError(t, x.settings.UpdateSetting(t.Context(), "projectsDirectory", root))
		c := container.InspectResponse{ID: "example-recreated", Name: "/example-proxy", Mounts: []container.MountPoint{{Type: mount.TypeBind, Source: root, Destination: "/etc/proxy", RW: true}}}
		d := dockerSeam(t, x, &c)
		i := x.request.Instance
		i.Source = dto.Source{Kind: dto.ProjectSource, Canonical: root, MountTarget: "/etc/proxy"}
		require.NotPanics(t, func() {
			_, err := d.Rebind(t.Context(), i)
			requireCode(t, err, dto.ContainerChanged)
		})
	})
	t.Run("partial decode", func(t *testing.T) {
		x := newFixture(t)
		root := t.TempDir()
		require.NoError(t, x.settings.UpdateSetting(t.Context(), "projectsDirectory", root))
		c := container.InspectResponse{ID: "example-recreated", Name: "/example-proxy", State: &container.State{Running: true, StartedAt: nextEpoch}, Mounts: []container.MountPoint{{Type: mount.TypeBind, Source: root, Destination: "/etc/proxy", RW: true}}}
		raw, err := json.Marshal(c)
		require.NoError(t, err)
		var payload map[string]any
		require.NoError(t, json.Unmarshal(raw, &payload))
		payload["RestartCount"] = "example-invalid-count"
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p := regexp.MustCompile(`^/v[0-9.]+`).ReplaceAllString(r.URL.Path, "")
			switch {
			case p == "/_ping":
				w.Header().Set("API-Version", "1.54")
				_, _ = w.Write([]byte("OK"))
			case strings.HasSuffix(p, "/json"):
				_ = json.NewEncoder(w).Encode(payload)
			case strings.HasSuffix(p, "/archive"):
				stat, _ := json.Marshal(container.PathStat{Name: "proxy", Mode: os.ModeDir | 0755})
				w.Header().Set("X-Docker-Container-Path-Stat", base64.StdEncoding.EncodeToString(stat))
			default:
				w.WriteHeader(404)
			}
		}))
		defer server.Close()
		d := &DockerRuntime{Docker: dockerinternal.NewDockerClientService(t.Context(), nil, &config.Config{DockerHost: server.URL}, nil), Settings: x.settings, DB: x.db}
		partial, err := d.inspect(t.Context(), "example-proxy")
		require.Error(t, err)
		require.NotNil(t, partial.State, "inspect returns an error together with a partially decoded container")
		i := x.request.Instance
		i.Source = dto.Source{Kind: dto.ProjectSource, Canonical: root, MountTarget: "/etc/proxy"}
		_, err = d.Rebind(t.Context(), i)
		// The partial container is never followed; the decode failure stays an error (C-B2).
		require.Error(t, err)
	})
}

// Recovery after a restart needs parseable epochs and a newer start than before.
func TestRestartRecoveryEpochGuards(t *testing.T) {
	for _, tt := range []struct{ name, before, after string }{
		{"unparseable before", "example-invalid-timestamp", nextEpoch},
		{"unparseable after", initialEpoch, "example-invalid-timestamp"},
		// Year 0000 parses and sorts before the zero time a failed parse returns.
		{"unparseable after an early before", "0000-01-01T00:00:00Z", "example-invalid-timestamp"},
		{"not newer", initialEpoch, initialEpoch},
	} {
		t.Run(tt.name, func(t *testing.T) {
			x := newFixture(t)
			x.request.Instance.Kind = dto.Caddy
			x.request.Acknowledgements.Restart = true
			i := Intent{Request: x.request, Old: Snapshot([]byte("old"), true), New: Snapshot([]byte("new"), true), Mode: 0644, Restart: true, StartedBefore: tt.before, GraphHashes: map[string]string{}}
			paths(&i)
			x.f.files[x.request.Target] = i.New
			x.f.epoch = tt.after
			d, err := Digest(x.request)
			require.NoError(t, err)
			x.persist(t, i, dto.Operation{ID: x.request.ID, InstanceID: x.request.Instance.ID, Generation: x.a.Generation(), Registration: x.request.Registration, Digest: d, State: dto.Pending, Phase: dto.Restarting, Target: x.request.Target, Old: i.Old.FileState, New: i.New.FileState, CreatedAt: time.Now().UTC()})
			got, err := x.a.Query(t.Context(), x.query())
			require.NoError(t, err)
			require.Equal(t, dto.RecoveryRequired, got.State)
			require.Equal(t, dto.ContainerChanged, got.Reason)
		})
	}
}

// An epoch error after observing is container-changed also when the recorded epoch is empty.
func TestObservationEpochErrorWithEmptyEpoch(t *testing.T) {
	x := newFixture(t)
	x.request.Instance.Identity.StartedAt = ""
	x.f.epoch = ""
	x.e.observe = func() Observation {
		x.f.mu.Lock()
		x.f.epochErr = os.ErrNotExist
		x.f.mu.Unlock()
		return Observation{State: dto.Verified, Evidence: dto.Routing}
	}
	o, err := x.a.Apply(t.Context(), x.request)
	require.NoError(t, err)
	require.Equal(t, dto.AppliedUnverified, o.State)
	require.Equal(t, dto.NotObservable, o.Reason) // unread identity is unconfirmed, not changed (C-B2)
}

// latchValidate holds Apply in Validate until release; it reports when Validate was reached.
func latchValidate(t *testing.T, x *fixture) (inside chan struct{}, release func(), done chan dto.Operation) {
	t.Helper()
	inside, resume := make(chan struct{}), make(chan struct{})
	var once sync.Once
	release = func() { once.Do(func() { close(resume) }) }
	t.Cleanup(release)
	x.e.validate = func() error { close(inside); <-resume; return nil }
	done = make(chan dto.Operation, 1)
	go func() {
		o, err := x.a.Apply(t.Context(), x.request)
		require.NoError(t, err)
		done <- o
	}()
	select {
	case <-inside:
	case <-time.After(5 * time.Second):
		require.Fail(t, "Apply did not reach Validate")
	}
	return inside, release, done
}

// Query and Release leave an active operation alone; it finishes on its own.
func TestActiveOperationIsNotRecoveredOrReleased(t *testing.T) {
	x := newFixture(t)
	_, release, done := latchValidate(t, x)
	got, err := x.a.Query(t.Context(), x.query())
	require.NoError(t, err)
	require.Equal(t, dto.Pending, got.State, "Query must not recover an active operation")
	_, err = x.a.Release(t.Context(), dto.ReleaseRequest{Query: x.query(), Confirmed: x.f.files["route.yml"].FileState, Actor: "example-admin"})
	requireCode(t, err, dto.OperationConflict)
	release()
	require.Equal(t, dto.Verified, (<-done).State)
}

// Release of a recovery-required journal is refused while its operation is still active.
func TestReleaseRefusesActiveRecoveryRequired(t *testing.T) {
	x := newFixture(t)
	recoveryRequired(t, x)
	x.a.mu.Lock()
	x.a.active[x.request.ID] = true
	x.a.mu.Unlock()
	_, err := x.a.Release(t.Context(), dto.ReleaseRequest{Query: x.query(), Confirmed: x.f.files["route.yml"].FileState, Actor: "example-admin"})
	requireCode(t, err, dto.OperationConflict)
}

// A restore that finds a third target state stops with target-changed and keeps the bytes.
func TestRestoreRefusesThirdTargetState(t *testing.T) {
	x := newFixture(t)
	// Observation asks for a restore after publication; the operator edited the file meanwhile.
	x.e.observe = func() Observation {
		x.f.mu.Lock()
		x.f.files["route.yml"] = Snapshot([]byte("operator"), true)
		x.f.mu.Unlock()
		return Observation{State: dto.Rejected, Reason: dto.ProxyRejected, Restore: true}
	}
	o, err := x.a.Apply(t.Context(), x.request)
	require.NoError(t, err)
	require.Equal(t, dto.RecoveryRequired, o.State)
	require.Equal(t, dto.TargetChanged, o.Reason)
	require.Equal(t, "operator", string(x.f.files["route.yml"].Content))
}

// The hub keeps its record when Release fails for another reason than a lost agent lock.
func TestHubReleaseKeepsRecordOnOtherErrors(t *testing.T) {
	for _, err := range []error{dto.Fail(dto.OperationConflict, 409), errors.New("example transport failure")} {
		t.Run(err.Error(), func(t *testing.T) {
			x := newFixture(t)
			recoveryRequired(t, x)
			persistRecord(t, x, dto.RecoveryRequired)
			h := &Hub{DB: x.db.DB, Modules: x.mods, Transport: &localTransport{a: x.a, reg: x.request.Registration, releaseErr: err}}
			var got error
			require.NotPanics(t, func() {
				_, got = h.Release(t.Context(), x.request.ID, x.f.files["route.yml"].FileState, "example-admin")
			})
			require.ErrorIs(t, got, err)
			var row Record
			require.NoError(t, x.db.First(&row, "id = ?", x.request.ID).Error)
			require.Equal(t, dto.RecoveryRequired, row.State)
		})
	}
}

// accept: a dispatch answer from another generation or with an unknown state is not recorded.
func TestHubAcceptRefusesGenerationAndState(t *testing.T) {
	for _, tt := range []struct {
		name   string
		mutate func(*dto.Operation)
		reason dto.Reason
	}{
		{"generation", func(o *dto.Operation) { o.Generation = "example-other-generation" }, dto.GenerationMismatch},
		{"state", func(o *dto.Operation) { o.State = "example-state" }, dto.JournalLost},
	} {
		t.Run(tt.name, func(t *testing.T) {
			x := newFixture(t)
			h := &Hub{DB: x.db.DB, Modules: x.mods, Transport: &localTransport{a: x.a, reg: x.request.Registration, applyMutate: tt.mutate}}
			_, _ = h.Apply(t.Context(), x.request)
			var row Record
			require.NoError(t, x.db.First(&row, "id = ?", x.request.ID).Error)
			require.Equal(t, dto.RecoveryRequired, row.State)
			require.Equal(t, tt.reason, row.Reason)
		})
	}
}

// Release refuses a foreign receipt also when a reconciler finalised the record meanwhile.
func TestReleaseReceiptGuardAfterConcurrentFinalization(t *testing.T) {
	for _, field := range []string{"generation", "registration"} {
		t.Run(field, func(t *testing.T) {
			x := newFixture(t)
			_, err := x.a.Apply(t.Context(), x.request)
			require.NoError(t, err)
			persistRecord(t, x, dto.RecoveryRequired)
			tr := &localTransport{a: x.a, reg: x.request.Registration}
			h := &Hub{DB: x.db.DB, Modules: x.mods, Transport: tr}
			tr.releaseMutate = func(o *dto.Operation) {
				if field == "generation" {
					o.Generation = "example-other-generation"
				} else {
					o.Registration = "example-other-registration"
				}
			}
			tr.afterRelease = func() {
				o, er := h.Query(t.Context(), x.request.ID)
				require.NoError(t, er)
				require.Equal(t, dto.Verified, o.State)
			}
			_, err = h.Release(t.Context(), x.request.ID, x.f.files["route.yml"].FileState, "example-admin")
			requireCode(t, err, dto.OperationConflict)
		})
	}
}

// Malformed intent and result payloads remain visible for explicit recovery.
func TestJournalDecodeErrorsAreIndependent(t *testing.T) {
	for _, field := range []string{"intent", "result"} {
		t.Run(field, func(t *testing.T) {
			x := newFixture(t)
			_, err := x.a.Apply(t.Context(), x.request)
			require.NoError(t, err)
			var j Journal
			require.NoError(t, x.db.First(&j, "id = ?", x.request.ID).Error)
			bad := []byte("{")
			if field == "result" {
				// Identity, state and phase still decode; a later field fails.
				bad = append(append([]byte{}, j.Result[:len(j.Result)-1]...), []byte(`,"releasedBy":false}`)...)
			}
			require.NoError(t, x.db.Table("proxy_journal").Where("id = ?", x.request.ID).Update(field, bad).Error)
			result, queryErr := x.a.Query(t.Context(), x.query())
			require.NoError(t, queryErr)
			require.Equal(t, dto.RecoveryRequired, result.State)
			require.Equal(t, dto.UnknownOperationKind, result.Reason)
		})
	}
}

// An engine observation outside verified and applied-unverified is not recorded as final.
func TestObservationOutsideContractIsRecoveryRequired(t *testing.T) {
	for _, state := range []dto.State{dto.Rejected, dto.Released, dto.Pending} {
		t.Run(string(state), func(t *testing.T) {
			x := newFixture(t)
			x.e.observe = func() Observation { return Observation{State: state, Evidence: dto.Routing} }
			o, err := x.a.Apply(t.Context(), x.request)
			require.NoError(t, err)
			require.Equal(t, dto.RecoveryRequired, o.State)
			require.Equal(t, dto.InvalidInput, o.Reason)
		})
	}
}

// Release is refused for an inactive journal that is not recovery-required yet.
func TestReleaseRefusesPendingJournal(t *testing.T) {
	x := newFixture(t)
	x.a.afterCommit = func(p dto.Phase) error {
		if p == dto.Observing {
			return errCrash
		}
		return nil
	}
	_, err := x.a.Apply(t.Context(), x.request)
	require.ErrorIs(t, err, errCrash)
	x.a.afterCommit = nil
	_, err = x.a.Release(t.Context(), dto.ReleaseRequest{Query: x.query(), Confirmed: x.f.files["route.yml"].FileState, Actor: "example-admin"})
	requireCode(t, err, dto.OperationConflict)
	var j Journal
	require.NoError(t, x.db.First(&j, "id = ?", x.request.ID).Error)
	require.Equal(t, dto.Pending, j.State)
}

// Cleanup keeps a typed source refusal: after a transient directory-stat failure the
// same container reads fine again, but Cleanup must not rebind and delete anything.
// Adapted from the R5 review probe.
func TestCleanupKeepsTypedSourceRefusal(t *testing.T) {
	x := newFixture(t)
	root := t.TempDir()
	i := fileIntent(t, x, root)
	require.NoError(t, os.WriteFile(filepath.Join(root, i.TempPath), i.New.Content, 0644))
	var stats atomic.Int32
	c := container.InspectResponse{ID: i.Request.Instance.Identity.ContainerID, Name: "/example-proxy", State: &container.State{Running: true, StartedAt: initialEpoch}, Mounts: []container.MountPoint{{Type: mount.TypeBind, Source: root, Destination: "/etc/proxy", RW: true}}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/_ping"):
			w.Header().Set("API-Version", "1.54")
			_, _ = w.Write([]byte("OK"))
		case strings.HasSuffix(r.URL.Path, "/json"):
			_ = json.NewEncoder(w).Encode(c)
		case strings.HasSuffix(r.URL.Path, "/archive"):
			if stats.Add(1) == 1 {
				w.WriteHeader(503)
				return
			}
			b, _ := json.Marshal(container.PathStat{Name: "proxy", Mode: os.ModeDir | 0755})
			w.Header().Set("X-Docker-Container-Path-Stat", base64.StdEncoding.EncodeToString(b))
		default:
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	d := &DockerRuntime{Docker: dockerinternal.NewDockerClientService(t.Context(), nil, &config.Config{DockerHost: server.URL}, nil), Settings: x.settings, DB: x.db}
	err := d.Cleanup(t.Context(), i)
	// The transient stat error stays an error (not a classification) and is not a container change.
	require.Error(t, err)
	p, typed := errors.AsType[*dto.Problem](err)
	require.False(t, typed && p.Code == dto.ContainerChanged)
	b, err := os.ReadFile(filepath.Join(root, i.TempPath))
	require.NoError(t, err)
	require.Equal(t, "new", string(b))
}
