// Source: docklet-next/issues/492
// Revision: 54fd54b6bf259e09c1d4fa66ab1cd7ebdebf09e6
// Retrieved: 2026-11-20
// License: Apache-2.0
// Deviations: Original resolver execution, recovery and verification.

package resolver_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/proxy/resolver"
	dto "github.com/florianfinn/docklet-next/types/proxy"
	"github.com/stretchr/testify/require"
)

type fakeHost struct {
	target                                        resolver.Target
	identity                                      dto.Instance
	recreates, observes, settles                  int
	failStartup, failRestore, failRestoredStartup bool
	settleErr, preflightErr, replacementErr       error
	observing                                     func()
}

func (f *fakeHost) Prepare(context.Context, dto.ResolverApplyRequest) (resolver.Target, error) {
	return f.target, nil
}
func (f *fakeHost) Preflight(context.Context, resolver.Target) error { return f.preflightErr }
func (f *fakeHost) Recreate(context.Context, resolver.Target) error {
	f.recreates++
	if f.failRestore && f.recreates == 2 {
		return errors.New("sensitive compose text")
	}
	f.identity.Identity = dto.Identity{ContainerID: "replacement-" + string(rune('a'+f.recreates)), ContainerName: "example-gateway", StartedAt: time.Now().UTC().Format(time.RFC3339Nano)}
	return nil
}
func (f *fakeHost) Settle(context.Context, resolver.Target) error { f.settles++; return f.settleErr }
func (f *fakeHost) Replacement(context.Context, resolver.Target, dto.Instance) (dto.Instance, error) {
	return f.identity, f.replacementErr
}
func (f *fakeHost) Observe(_ context.Context, _ dto.Instance, _ string, present bool, _ time.Time) error {
	f.observes++
	if f.observing != nil {
		f.observing()
	}
	if present && f.failStartup || !present && f.failRestoredStartup {
		return dto.Fail(dto.ProxyRejected, 422)
	}
	return nil
}
func fixture(t *testing.T) (*resolver.Executor, *fakeHost, dto.ResolverApplyRequest, *dto.Operation) {
	t.Helper()
	return fixtureFor(t, request(), nil)
}

// fixtureFor builds the executor fixture for a request; prep may adjust the
// target directory before Build reads it.
func fixtureFor(t *testing.T, r dto.ResolverRequest, prep func(dir string)) (*resolver.Executor, *fakeHost, dto.ResolverApplyRequest, *dto.Operation) {
	t.Helper()
	v := target(t, true)
	if prep != nil {
		prep(v.Directory)
		for i := range v.Files {
			f, e := resolver.Read(v.Directory, v.Files[i].Path)
			require.NoError(t, e)
			f.Kind = v.Files[i].Kind
			v.Files[i] = f
		}
	}
	q := dto.ResolverApplyRequest{ID: "resolver-one", Generation: "generation-one", Registration: "local", Request: r, Instance: dto.Instance{ID: "instance-one", Kind: dto.Traefik, EnvironmentID: "0", Registration: "local", Identity: dto.Identity{ContainerID: "original", ContainerName: "example-gateway", StartedAt: "2026-11-20T01:00:00Z"}, Source: dto.Source{Kind: dto.ProjectSource, MountTarget: "/etc/traefik", Canonical: "/example/project/config"}}}
	f := &fakeHost{target: build(t, v, q.Request), identity: q.Instance}
	en := dto.ResolverEnvelope{Kind: dto.ResolverOperation, ID: q.ID, Generation: q.Generation, Registration: q.Registration, Instance: q.Instance, Intent: q.Request.Redact()}
	digest, e := en.Digest()
	require.NoError(t, e)
	o := &dto.Operation{ID: q.ID, InstanceID: q.Instance.ID, Kind: dto.ResolverOperation, Generation: q.Generation, Registration: q.Registration, Digest: digest, State: dto.Pending, Phase: dto.Admitted, Resolver: &dto.ResolverReceipt{Intent: q.Request.Redact(), Phase: dto.ResolverAdmitted, Files: []dto.ResolverFile{}}}
	executor := &resolver.Executor{Host: f, Backups: filepath.Join(t.TempDir(), "backups"), Save: func(_ context.Context, v dto.Operation) error {
		b, e := json.Marshal(v)
		if e != nil {
			return e
		}
		return json.Unmarshal(b, o)
	}}
	return executor, f, q, o
}
func envelope(q dto.ResolverApplyRequest) dto.ResolverEnvelope {
	return dto.ResolverEnvelope{Kind: dto.ResolverOperation, ID: q.ID, Generation: q.Generation, Registration: q.Registration, Instance: q.Instance, Intent: q.Request.Redact()}
}
func assertOriginal(t *testing.T, f *fakeHost) {
	t.Helper()
	for _, file := range f.target.Files {
		got, e := resolver.Read(f.target.Directory, file.Path)
		require.NoError(t, e)
		require.Equal(t, file.Old, got.Old, file.Path)
		require.Equal(t, file.Before, got.Before, file.Path)
		require.Equal(t, file.Mode, got.Mode, file.Path)
	}
}
func TestR2R3StartupRollbackAndRestoreFailure(t *testing.T) {
	for _, failure := range []string{"startup", "restore-deploy", "restore-startup"} {
		t.Run(failure, func(t *testing.T) {
			e, f, q, o := fixture(t)
			f.failStartup = true
			f.failRestore = failure == "restore-deploy"
			f.failRestoredStartup = failure == "restore-startup"
			result, err := e.Run(t.Context(), q, *o)
			require.NoError(t, err)
			assertOriginal(t, f)
			require.Equal(t, 2, f.recreates)
			require.Equal(t, 1, f.settles)
			if failure == "startup" {
				require.Equal(t, dto.RolledBack, result.State)
				require.NotNil(t, result.Resolver.Replacement)
				_, err = os.Stat(filepath.Join(e.Backups, q.ID))
				require.True(t, os.IsNotExist(err))
			} else {
				require.Equal(t, dto.RecoveryRequired, result.State)
				require.Equal(t, dto.RestoreFailed, result.Reason)
				_, err = os.Stat(filepath.Join(e.Backups, q.ID, "manifest.json"))
				require.NoError(t, err)
			}
		})
	}
}
func TestR1R4CrashEveryPhaseAndEffect(t *testing.T) {
	for _, phase := range []dto.ResolverPhase{dto.ResolverAdmitted, dto.ResolverBackup, dto.ResolverWriting, dto.ResolverRecreating, dto.ResolverObserving, dto.ResolverRestoring, dto.ResolverApplied, dto.ResolverRolledBack} {
		for _, after := range []bool{false, true} {
			t.Run(string(phase)+map[bool]string{false: "/commit", true: "/effect"}[after], func(t *testing.T) {
				e, f, q, o := fixture(t)
				if phase == dto.ResolverRestoring || phase == dto.ResolverRolledBack {
					f.failStartup = true
				}
				hook := func(p dto.Phase) error {
					if p == dto.Phase(phase) {
						return resolver.ErrInterrupted
					}
					return nil
				}
				if after {
					e.AfterEffect = hook
				} else {
					e.AfterCommit = hook
				}
				initial := *o
				if phase == dto.ResolverAdmitted {
					require.NoError(t, e.Save(t.Context(), initial))
				} else {
					_, err := e.Run(t.Context(), q, initial)
					if after && (phase == dto.ResolverApplied || phase == dto.ResolverRolledBack) {
						require.NoError(t, err)
					} else {
						require.ErrorIs(t, err, resolver.ErrInterrupted)
					}
				}
				e.AfterCommit = nil
				e.AfterEffect = nil
				f.failStartup = false
				result, err := e.Resume(t.Context(), envelope(q), *o)
				require.NoError(t, err)
				if phase == dto.ResolverApplied {
					require.Equal(t, dto.Applied, result.State)
				} else {
					require.NotEqual(t, dto.Applied, result.State)
					assertOriginal(t, f)
					require.Contains(t, []dto.State{dto.Refused, dto.RolledBack}, result.State)
				}
			})
		}
	}
}
func TestR6R14ThirdVersionAllFilePreflight(t *testing.T) {
	e, f, q, o := fixture(t)
	f.failStartup = true
	third := []byte("REUSED='operator-edit'\n")
	f.observing = func() {
		require.NoError(t, os.WriteFile(filepath.Join(f.target.Directory, ".env"), third, 0600))
		f.observing = nil
	}
	result, err := e.Run(t.Context(), q, *o)
	require.NoError(t, err)
	require.Equal(t, dto.RecoveryRequired, result.State)
	require.Equal(t, dto.ThirdVersion, result.Reason)
	require.Equal(t, 1, f.recreates)
	// The static file precedes .env; all-file preflight must keep even that file
	// untouched when the last file has a third version.
	static, err := resolver.Read(f.target.Directory, f.target.Files[0].Path)
	require.NoError(t, err)
	require.Equal(t, f.target.Files[0].New, static.Old)
	env, err := os.ReadFile(filepath.Join(f.target.Directory, ".env"))
	require.NoError(t, err)
	require.Equal(t, third, env)
}
func TestR6CompareBeforeFirstWrite(t *testing.T) {
	e, f, q, o := fixture(t)
	third := []byte("operator edit\n")
	e.AfterCommit = func(p dto.Phase) error {
		if p == dto.Phase(dto.ResolverWriting) {
			return os.WriteFile(filepath.Join(f.target.Directory, ".env"), third, 0600)
		}
		return nil
	}
	result, err := e.Run(t.Context(), q, *o)
	require.NoError(t, err)
	require.Equal(t, dto.Refused, result.State)
	require.Zero(t, f.recreates)
	actual, err := os.ReadFile(filepath.Join(f.target.Directory, ".env"))
	require.NoError(t, err)
	require.Equal(t, third, actual)
}
func TestR4RecoveryPreflightBranches(t *testing.T) {
	for _, branch := range []string{"old", "new", "mixed", "third", "backup-invalid", "unsettled", "target-unclear", "identity-unclear", "unreadable"} {
		t.Run(branch, func(t *testing.T) {
			e, f, q, o := fixture(t)
			e.AfterCommit = func(p dto.Phase) error {
				if p == dto.Phase(dto.ResolverRecreating) {
					return resolver.ErrInterrupted
				}
				return nil
			}
			_, err := e.Run(t.Context(), q, *o)
			require.ErrorIs(t, err, resolver.ErrInterrupted)
			e.AfterCommit = nil
			for j, file := range f.target.Files {
				if branch == "old" || branch == "mixed" && j%2 == 0 {
					require.NoError(t, resolver.Replace(f.target.Directory, file.Path, file.New, file.Before, file.Old.Present, file.Mode))
				}
			}
			switch branch {
			case "third":
				require.NoError(t, os.WriteFile(filepath.Join(f.target.Directory, ".env"), []byte("THIRD=1"), 0600))
			case "backup-invalid":
				require.NoError(t, os.WriteFile(filepath.Join(e.Backups, q.ID, "0"), []byte("corrupt"), 0600))
			case "unsettled":
				f.settleErr = dto.Fail(dto.EffectUnsettled, 409)
			case "target-unclear":
				f.preflightErr = dto.Fail(dto.TargetUnclear, 409)
			case "identity-unclear":
				f.replacementErr = dto.Fail(dto.IdentityUnclear, 409)
			case "unreadable":
				require.NoError(t, os.Remove(filepath.Join(f.target.Directory, ".env")))
				require.NoError(t, os.Symlink("missing", filepath.Join(f.target.Directory, ".env")))
			}
			result, err := e.Resume(t.Context(), envelope(q), *o)
			if branch == "unreadable" {
				// A symlinked target is a persistent third version, never a retry loop.
				require.NoError(t, err)
				require.Equal(t, dto.RecoveryRequired, result.State)
				require.Equal(t, dto.ThirdVersion, result.Reason)
				require.Zero(t, f.recreates)
				link, er := os.Readlink(filepath.Join(f.target.Directory, ".env"))
				require.NoError(t, er)
				require.Equal(t, "missing", link)
				return
			}
			require.NoError(t, err)
			if branch == "old" || branch == "new" || branch == "mixed" {
				require.Equal(t, dto.RolledBack, result.State)
				assertOriginal(t, f)
			} else {
				require.Equal(t, dto.RecoveryRequired, result.State)
				if branch != "identity-unclear" {
					require.Zero(t, f.recreates)
				}
			}
		})
	}
}
func TestR7HashOnlyJournalAndBackupLifecycle(t *testing.T) {
	e, f, q, o := fixture(t)
	e.AfterCommit = func(p dto.Phase) error {
		if p == dto.Phase(dto.ResolverWriting) {
			return resolver.ErrInterrupted
		}
		return nil
	}
	_, err := e.Run(t.Context(), q, *o)
	require.ErrorIs(t, err, resolver.ErrInterrupted)
	for _, v := range []any{envelope(q), o} {
		raw, err := json.Marshal(v)
		require.NoError(t, err)
		require.NotContains(t, string(raw), "existing-inline-sentinel")
		require.NotContains(t, string(raw), "existing-reused-sentinel")
	}
	dir, err := os.Stat(filepath.Join(e.Backups, q.ID))
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0700), dir.Mode().Perm())
	for _, name := range []string{"0", "1", "2", "manifest.json"} {
		st, err := os.Stat(filepath.Join(e.Backups, q.ID, name))
		require.NoError(t, err)
		require.Equal(t, os.FileMode(0600), st.Mode().Perm())
	}
	manifest, err := os.ReadFile(filepath.Join(e.Backups, q.ID, "manifest.json"))
	require.NoError(t, err)
	require.NotContains(t, string(manifest), "existing-inline-sentinel")
	e.AfterCommit = nil
	_, err = e.Resume(t.Context(), envelope(q), *o)
	require.NoError(t, err)
	assertOriginal(t, f)
}
func TestR19ReleaseConfirmationNoFileEffect(t *testing.T) {
	e, f, q, o := fixture(t)
	f.failStartup = true
	f.failRestore = true
	result, err := e.Run(t.Context(), q, *o)
	require.NoError(t, err)
	require.Equal(t, dto.RecoveryRequired, result.State)
	_, err = e.Release(t.Context(), envelope(q), result, dto.ReleaseRequest{})
	require.Error(t, err)
	current, err := resolver.Current(f.target)
	require.NoError(t, err)
	released, err := e.Release(t.Context(), envelope(q), result, dto.ReleaseRequest{Confirmed: current, Actor: "admin-one"})
	require.NoError(t, err)
	require.Equal(t, dto.Released, released.State)
	assertOriginal(t, f)
	require.Equal(t, 2, f.recreates)
	_, err = os.Stat(filepath.Join(e.Backups, q.ID))
	require.True(t, os.IsNotExist(err))
}

func TestR1AbsentEnvironmentRestoredAfterPartialWrite(t *testing.T) {
	e, f, q, o := fixture(t)
	for j := range f.target.Files {
		file := &f.target.Files[j]
		if file.Kind == "env" {
			require.NoError(t, os.Remove(filepath.Join(f.target.Directory, file.Path)))
			file.Old = dto.FileState{}
			file.Before = nil
			file.After = []byte("HTTPREQ_PASSWORD='example'\n")
			readDir := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(readDir, "env"), file.After, 0600))
			b, err := resolver.Read(readDir, "env")
			require.NoError(t, err)
			file.New = b.Old
		}
	}
	f.failStartup = true
	result, err := e.Run(t.Context(), q, *o)
	require.NoError(t, err)
	require.Equal(t, dto.RolledBack, result.State)
	_, err = os.Stat(filepath.Join(f.target.Directory, ".env"))
	require.True(t, os.IsNotExist(err))
}

// B3: a touched file whose metadata changes during observation is a third
// version; the operation must end releasable instead of retrying forever.
func TestRestoreMetadataChangeIsReleasable(t *testing.T) {
	e, f, q, o := fixture(t)
	f.failStartup = true
	static := filepath.Join(f.target.Directory, "config", "static.yaml")
	f.observing = func() { require.NoError(t, os.Chmod(static, 0664)) }
	result, err := e.Run(t.Context(), q, *o)
	require.NoError(t, err)
	require.Equal(t, dto.RecoveryRequired, result.State)
	require.Equal(t, dto.ThirdVersion, result.Reason)
	current, err := resolver.Current(f.target)
	require.NoError(t, err)
	released, err := e.Release(t.Context(), envelope(q), result, dto.ReleaseRequest{Confirmed: current, Actor: "admin-one"})
	require.NoError(t, err)
	require.Equal(t, dto.Released, released.State)
}

// B3: confirmation ignores the writer's metadata rules, so a group-writable or
// replaced file still yields a stable state that changes with the content.
func TestCurrentToleratesUnsupportedMetadata(t *testing.T) {
	_, f, _, _ := fixture(t)
	p := filepath.Join(f.target.Directory, "compose.yaml")
	require.NoError(t, os.Chmod(p, 0664))
	first, err := resolver.Current(f.target)
	require.NoError(t, err)
	again, err := resolver.Current(f.target)
	require.NoError(t, err)
	require.Equal(t, first, again)
	require.NoError(t, os.Remove(p))
	require.NoError(t, os.Symlink("elsewhere.yaml", p))
	linked, err := resolver.Current(f.target)
	require.NoError(t, err)
	require.NotEqual(t, first, linked)
}

// D-A1: a changed .env with Docklet's default mode (0644) is written owner-only;
// a rollback restores the previous bytes and mode.
func TestChangedEnvWrittenOwnerOnlyAndRestored(t *testing.T) {
	for _, rollback := range []bool{false, true} {
		t.Run(map[bool]string{false: "applied", true: "rolled-back"}[rollback], func(t *testing.T) {
			r := request()
			endpoint, value := "http://sink.example.test", "submitted-sentinel"
			r.Challenge = dto.ResolverChallenge{Kind: "dns", Provider: dto.HTTPRequest}
			r.Variables = []dto.ResolverVariable{{Name: "HTTPREQ_ENDPOINT", Value: &endpoint}, {Name: "HTTPREQ_PASSWORD", Value: &value}}
			e, f, q, o := fixtureFor(t, r, func(dir string) { require.NoError(t, os.Chmod(filepath.Join(dir, ".env"), 0644)) })
			f.failStartup = rollback
			result, err := e.Run(t.Context(), q, *o)
			require.NoError(t, err)
			st, err := os.Stat(filepath.Join(f.target.Directory, ".env"))
			require.NoError(t, err)
			if rollback {
				require.Equal(t, dto.RolledBack, result.State)
				require.Equal(t, os.FileMode(0644), st.Mode().Perm())
				assertOriginal(t, f)
				return
			}
			require.Equal(t, dto.Applied, result.State)
			require.Equal(t, os.FileMode(0600), st.Mode().Perm())
		})
	}
}
