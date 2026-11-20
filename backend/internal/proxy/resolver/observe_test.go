// Source: docklet-next/issues/492
// Revision: f359750cd5e74f2ac4cde843de904d4d6dd31359
// Retrieved: 2026-11-20
// License: Apache-2.0
// Deviations: Original confirmation and transient-I/O guards; scenarios from the S5 delta review.

package resolver_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"
	"unsafe"

	"github.com/florianfinn/docklet-next/backend/internal/proxy/resolver"
	dto "github.com/florianfinn/docklet-next/types/proxy"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

func requireCode(t *testing.T, err error, want dto.Reason) {
	t.Helper()
	p, ok := errors.AsType[*dto.Problem](err)
	require.True(t, ok, "error %v", err)
	require.Equal(t, want, p.Code)
}

// A stale confirmation must not release a replaced unsupported path.
func TestStaleUnsupportedConfirmationRefused(t *testing.T) {
	for _, replace := range []string{"unchanged", "symlink", "directory"} {
		t.Run(replace, func(t *testing.T) {
			e, f, q, o := fixture(t)
			f.failStartup = true
			p := filepath.Join(f.target.Directory, ".env")
			f.observing = func() {
				require.NoError(t, os.Remove(p))
				require.NoError(t, os.Symlink("operator-version-one", p))
			}
			result, err := e.Run(t.Context(), q, *o)
			require.NoError(t, err)
			require.Equal(t, dto.RecoveryRequired, result.State)
			require.NotNil(t, result.Current)
			confirmed := *result.Current
			switch replace {
			case "symlink":
				require.NoError(t, os.Remove(p))
				require.NoError(t, os.Symlink("operator-version-two", p))
			case "directory":
				require.NoError(t, os.Remove(p))
				require.NoError(t, os.Mkdir(p, 0700))
			}
			released, err := e.Release(t.Context(), envelope(q), result, dto.ReleaseRequest{Confirmed: confirmed, Actor: "admin-one"})
			if replace == "unchanged" {
				require.NoError(t, err)
				require.Equal(t, dto.Released, released.State)
				return
			}
			requireCode(t, err, dto.TargetChanged)
		})
	}
}

// Regular readable files keep the content-hash confirmation; mode changes keep
// it stable and a FIFO does not block.
func TestCurrentRegularHashAndFIFO(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "file")
	require.NoError(t, os.WriteFile(p, []byte("example\n"), 0600))
	f, err := resolver.Read(dir, "file")
	require.NoError(t, err)
	target := resolver.Target{Directory: dir, Files: []resolver.File{f}}
	before, err := resolver.Current(target)
	require.NoError(t, err)
	vector, _ := json.Marshal([]struct {
		Path  string        `json:"path"`
		State dto.FileState `json:"state"`
	}{{"file", f.Old}})
	hash := sha256.Sum256(vector)
	require.Equal(t, dto.FileState{Present: true, Hash: hex.EncodeToString(hash[:])}, before)
	require.NoError(t, os.Chmod(p, 0664))
	after, err := resolver.Current(target)
	require.NoError(t, err)
	require.Equal(t, before, after)
	require.NoError(t, os.Remove(p))
	require.NoError(t, unix.Mkfifo(p, 0600))
	done := make(chan error, 1)
	go func() { _, e := resolver.Current(target); done <- e }()
	select {
	case e := <-done:
		require.NoError(t, e)
	case <-time.After(time.Second):
		t.Fatal("FIFO blocked")
	}
}

// An oversized file is confirmed by a bounded descriptor that still changes
// when the file grows. The helper runs in a subprocess so an unbounded read can be killed.
func TestCurrentOversizedFileBounded(t *testing.T) {
	if dir := os.Getenv("RESOLVER_HUGE_TARGET"); dir != "" {
		target := resolver.Target{Directory: dir, Files: []resolver.File{{ResolverFile: dto.ResolverFile{Path: "file"}}}}
		first, err := resolver.Current(target)
		require.NoError(t, err)
		f, err := os.OpenFile(filepath.Join(dir, "file"), os.O_WRONLY|os.O_APPEND, 0)
		require.NoError(t, err)
		_, err = f.WriteString("grown")
		require.NoError(t, err)
		require.NoError(t, f.Close())
		second, err := resolver.Current(target)
		require.NoError(t, err)
		require.NotEqual(t, first, second)
		return
	}
	dir := t.TempDir()
	f, err := os.Create(filepath.Join(dir, "file"))
	require.NoError(t, err)
	require.NoError(t, f.Truncate(1<<40))
	require.NoError(t, f.Close())
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestCurrentOversizedFileBounded$")
	cmd.Env = append(os.Environ(), "RESOLVER_HUGE_TARGET="+dir)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "oversized confirmation did not finish: %s", out)
}

// Observation never follows a symlinked parent directory out of the project.
func TestCurrentDoesNotFollowParentSymlink(t *testing.T) {
	dir, outside := t.TempDir(), t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(outside, "static.yaml"), []byte("outside-version-one"), 0600))
	require.NoError(t, os.Symlink(outside, filepath.Join(dir, "config")))
	target := resolver.Target{Directory: dir, Files: []resolver.File{{ResolverFile: dto.ResolverFile{Path: "config/static.yaml"}}}}
	first, err := resolver.Current(target)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(outside, "static.yaml"), []byte("outside-version-two"), 0600))
	second, err := resolver.Current(target)
	require.NoError(t, err)
	require.Equal(t, first, second)
}

// injectOpenEIO makes every non-directory openat on the calling thread fail with EIO.
func injectOpenEIO() error {
	filter := []unix.SockFilter{
		{Code: unix.BPF_LD | unix.BPF_W | unix.BPF_ABS, K: 0},
		{Code: unix.BPF_JMP | unix.BPF_JEQ | unix.BPF_K, K: unix.SYS_OPENAT, Jt: 0, Jf: 4},
		{Code: unix.BPF_LD | unix.BPF_W | unix.BPF_ABS, K: 16 + 2*8},
		{Code: unix.BPF_JMP | unix.BPF_JSET | unix.BPF_K, K: unix.O_DIRECTORY, Jt: 2, Jf: 0},
		{Code: unix.BPF_RET | unix.BPF_K, K: unix.SECCOMP_RET_ERRNO | uint32(unix.EIO)},
		{Code: unix.BPF_RET | unix.BPF_K, K: unix.SECCOMP_RET_ALLOW},
		{Code: unix.BPF_RET | unix.BPF_K, K: unix.SECCOMP_RET_ALLOW},
	}
	if err := unix.Prctl(unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0); err != nil {
		return err
	}
	program := unix.SockFprog{Len: uint16(len(filter)), Filter: &filter[0]}
	return unix.Prctl(unix.PR_SET_SECCOMP, unix.SECCOMP_MODE_FILTER, uintptr(unsafe.Pointer(&program)), 0, 0)
}

type eioHost struct {
	*fakeHost
	calls int
}

// The second Preflight is the restore preflight; the fault starts after it.
func (h *eioHost) Preflight(context.Context, resolver.Target) error {
	h.calls++
	if h.calls == 2 {
		return injectOpenEIO()
	}
	return nil
}

// A transient read error during restore keeps the journal and stays retryable.
func TestRestoreTransientOpenErrorRetries(t *testing.T) {
	if os.Getenv("RESOLVER_EIO_HELPER") != "1" {
		cmd := exec.Command(os.Args[0], "-test.run=^TestRestoreTransientOpenErrorRetries$")
		cmd.Env = append(os.Environ(), "RESOLVER_EIO_HELPER=1")
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "%s", out)
		return
	}
	e, f, q, o := fixture(t)
	f.failStartup = true
	e.Host = &eioHost{fakeHost: f}
	type outcome struct {
		op  dto.Operation
		err error
	}
	done := make(chan outcome, 1)
	go func() {
		// The locked thread ends with this goroutine, so the filter never
		// reaches test cleanup or other runtime threads.
		runtime.LockOSThread()
		op, err := e.Run(t.Context(), q, *o)
		done <- outcome{op, err}
	}()
	select {
	case r := <-done:
		requireCode(t, r.err, dto.AgentUnavailable)
		require.Equal(t, dto.Pending, r.op.State)
		require.Equal(t, dto.ResolverObserving, r.op.Resolver.Phase)
	case <-time.After(5 * time.Second):
		t.Fatal("helper did not finish")
	}
}

// A parent without search permission is fingerprinted, so replacing it refuses
// a stale confirmation.
func TestReplacedSearchDeniedParentRefused(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("permission bits do not restrict root")
	}
	e, f, q, o := fixture(t)
	f.failStartup = true
	parent := filepath.Join(f.target.Directory, "config")
	moved := filepath.Join(f.target.Directory, "old-config")
	t.Cleanup(func() { _ = os.Chmod(parent, 0700); _ = os.Chmod(moved, 0700) })
	f.observing = func() { require.NoError(t, os.Chmod(parent, 0400)) }
	held, err := e.Run(t.Context(), q, *o)
	require.NoError(t, err)
	require.Equal(t, dto.RecoveryRequired, held.State)
	unchanged, err := resolver.Current(f.target)
	require.NoError(t, err)
	require.Equal(t, *held.Current, unchanged)
	require.NoError(t, os.Rename(parent, moved))
	require.NoError(t, os.Mkdir(parent, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(parent, "static.yaml"), []byte("operator-version-two"), 0600))
	require.NoError(t, os.Chmod(parent, 0400))
	_, err = e.Release(t.Context(), envelope(q), held, dto.ReleaseRequest{Confirmed: unchanged, Actor: "admin-one"})
	requireCode(t, err, dto.TargetChanged)
}

// An oversized target confirms and releases unchanged; growth refuses the
// earlier confirmation.
func TestOversizedTargetRelease(t *testing.T) {
	for _, grown := range []bool{false, true} {
		t.Run(map[bool]string{false: "unchanged", true: "grown"}[grown], func(t *testing.T) {
			e, f, q, o := fixture(t)
			f.failStartup = true
			p := filepath.Join(f.target.Directory, ".env")
			f.observing = func() { require.NoError(t, os.Truncate(p, 1<<40)) }
			held, err := e.Run(t.Context(), q, *o)
			require.NoError(t, err)
			require.Equal(t, dto.RecoveryRequired, held.State)
			if grown {
				require.NoError(t, os.Truncate(p, 1<<40+1))
			}
			released, err := e.Release(t.Context(), envelope(q), held, dto.ReleaseRequest{Confirmed: *held.Current, Actor: "admin-one"})
			if grown {
				requireCode(t, err, dto.TargetChanged)
				return
			}
			require.NoError(t, err)
			require.Equal(t, dto.Released, released.State)
		})
	}
}

// denySyscall makes one syscall fail with errno on the calling (locked) thread.
func denySyscall(syscall uint32, errno unix.Errno) error {
	filter := []unix.SockFilter{
		{Code: unix.BPF_LD | unix.BPF_W | unix.BPF_ABS, K: 0},
		{Code: unix.BPF_JMP | unix.BPF_JEQ | unix.BPF_K, K: syscall, Jt: 0, Jf: 1},
		{Code: unix.BPF_RET | unix.BPF_K, K: unix.SECCOMP_RET_ERRNO | uint32(errno)},
		{Code: unix.BPF_RET | unix.BPF_K, K: unix.SECCOMP_RET_ALLOW},
	}
	if err := unix.Prctl(unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0); err != nil {
		return err
	}
	program := unix.SockFprog{Len: uint16(len(filter)), Filter: &filter[0]}
	return unix.Prctl(unix.PR_SET_SECCOMP, unix.SECCOMP_MODE_FILTER, uintptr(unsafe.Pointer(&program)), 0, 0)
}

// subprocess reruns one test in a child so a seccomp fault cannot leak.
func subprocess(t *testing.T, name, env string) bool {
	if os.Getenv(env) == "1" {
		return false
	}
	cmd := exec.Command(os.Args[0], "-test.run=^"+name+"$")
	cmd.Env = append(os.Environ(), env+"=1")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "%s", out)
	return true
}

// A persistent read refusal after a successful open stays confirmable.
func TestPersistentReadErrorConfirmable(t *testing.T) {
	if subprocess(t, "TestPersistentReadErrorConfirmable", "RESOLVER_READ_HELPER") {
		return
	}
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "file"), []byte("example"), 0600))
	target := resolver.Target{Directory: dir, Files: []resolver.File{{ResolverFile: dto.ResolverFile{Path: "file"}}}}
	type result struct {
		first, second dto.FileState
		err           error
	}
	done := make(chan result, 1)
	go func() {
		runtime.LockOSThread()
		if err := denySyscall(unix.SYS_READ, unix.EACCES); err != nil {
			done <- result{err: err}
			return
		}
		first, err := resolver.Current(target)
		if err != nil {
			done <- result{err: err}
			return
		}
		second, err := resolver.Current(target)
		done <- result{first, second, err}
	}()
	select {
	case r := <-done:
		require.NoError(t, r.err)
		require.True(t, r.first.Present)
		require.Equal(t, r.first, r.second)
	case <-time.After(5 * time.Second):
		t.Fatal("helper did not finish")
	}
}

type deniedReadHost struct {
	*fakeHost
	calls int
}

func (h *deniedReadHost) Preflight(context.Context, resolver.Target) error {
	h.calls++
	if h.calls == 2 {
		return denySyscall(unix.SYS_READ, unix.EACCES)
	}
	return nil
}

// A persistent read refusal during restore is a third version, not a retry.
func TestRestorePersistentReadErrorIsThirdVersion(t *testing.T) {
	if subprocess(t, "TestRestorePersistentReadErrorIsThirdVersion", "RESOLVER_RESTORE_HELPER") {
		return
	}
	e, f, q, o := fixture(t)
	f.failStartup = true
	e.Host = &deniedReadHost{fakeHost: f}
	type outcome struct {
		op  dto.Operation
		err error
	}
	done := make(chan outcome, 1)
	go func() {
		runtime.LockOSThread()
		op, err := e.Run(t.Context(), q, *o)
		done <- outcome{op, err}
	}()
	select {
	case r := <-done:
		require.NoError(t, r.err)
		require.Equal(t, dto.RecoveryRequired, r.op.State)
		require.Equal(t, dto.ThirdVersion, r.op.Reason)
	case <-time.After(5 * time.Second):
		t.Fatal("helper did not finish")
	}
}

// confirmationHost confirms against the fixture target without backup reads, so
// a syscall fault reaches only the observation path.
type confirmationHost struct{ *fakeHost }

func (h *confirmationHost) Confirmation(context.Context, dto.Instance, []dto.ResolverFile) (resolver.Target, error) {
	return h.target, nil
}

// A persistent Fstat refusal of a search-denied parent still yields a
// confirmation (descriptor-relative stat), so Release stays possible.
func TestParentStatRefusalStaysReleasable(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("permission bits do not restrict root")
	}
	if subprocess(t, "TestParentStatRefusalStaysReleasable", "RESOLVER_PARENT_FSTAT_HELPER") {
		return
	}
	e, f, q, o := fixture(t)
	f.failStartup = true
	e.Host = &confirmationHost{fakeHost: f}
	parent := filepath.Join(f.target.Directory, "config")
	t.Cleanup(func() { _ = os.Chmod(parent, 0700) })
	f.observing = func() { require.NoError(t, os.Chmod(parent, 0400)) }
	held, err := e.Run(t.Context(), q, *o)
	require.NoError(t, err)
	require.Equal(t, dto.RecoveryRequired, held.State)
	type result struct {
		current dto.FileState
		err     error
		release error
	}
	done := make(chan result, 1)
	go func() {
		runtime.LockOSThread()
		if err := denySyscall(unix.SYS_FSTAT, unix.EACCES); err != nil {
			done <- result{err: err}
			return
		}
		// Under the fault touched files fall back to descriptors, so the operator
		// confirms the state they see now.
		current, err := resolver.Current(f.target)
		_, release := e.Release(t.Context(), envelope(q), held, dto.ReleaseRequest{Confirmed: current, Actor: "admin-one"})
		done <- result{current, err, release}
	}()
	select {
	case r := <-done:
		require.NoError(t, r.err)
		require.True(t, r.current.Present)
		require.NoError(t, r.release)
	case <-time.After(5 * time.Second):
		t.Fatal("helper did not finish")
	}
}

// A persistent refusal to open "/" is typed for the writer and confirmable for
// observation (root metadata stays observable without open).
func TestRootOpenRefusalIsClassified(t *testing.T) {
	if subprocess(t, "TestRootOpenRefusalIsClassified", "RESOLVER_ROOT_OPEN_HELPER") {
		return
	}
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "file"), []byte("example"), 0600))
	target := resolver.Target{Directory: dir, Files: []resolver.File{{ResolverFile: dto.ResolverFile{Path: "file"}}}}
	type result struct {
		read          error
		first, second dto.FileState
		current       error
	}
	done := make(chan result, 1)
	go func() {
		runtime.LockOSThread()
		if err := denySyscall(unix.SYS_OPENAT, unix.EACCES); err != nil {
			done <- result{read: err, current: err}
			return
		}
		_, read := resolver.Read(dir, "file")
		first, current := resolver.Current(target)
		second, _ := resolver.Current(target)
		done <- result{read, first, second, current}
	}()
	select {
	case r := <-done:
		requireCode(t, r.read, dto.UnsupportedMetadata)
		require.NoError(t, r.current)
		require.True(t, r.first.Present)
		require.Equal(t, r.first, r.second)
	case <-time.After(5 * time.Second):
		t.Fatal("helper did not finish")
	}
}

// denyEmptyPathStat refuses only descriptor-relative (AT_EMPTY_PATH) stats on
// the calling thread; named no-follow stats stay available.
func denyEmptyPathStat() error {
	filter := []unix.SockFilter{
		{Code: unix.BPF_LD | unix.BPF_W | unix.BPF_ABS, K: 0},
		{Code: unix.BPF_JMP | unix.BPF_JEQ | unix.BPF_K, K: unix.SYS_NEWFSTATAT, Jf: 3},
		{Code: unix.BPF_LD | unix.BPF_W | unix.BPF_ABS, K: 40},
		{Code: unix.BPF_JMP | unix.BPF_JSET | unix.BPF_K, K: unix.AT_EMPTY_PATH, Jf: 1},
		{Code: unix.BPF_RET | unix.BPF_K, K: unix.SECCOMP_RET_ERRNO | uint32(unix.EACCES)},
		{Code: unix.BPF_RET | unix.BPF_K, K: unix.SECCOMP_RET_ALLOW},
	}
	program := unix.SockFprog{Len: uint16(len(filter)), Filter: &filter[0]}
	return unix.Prctl(unix.PR_SET_SECCOMP, unix.SECCOMP_MODE_FILTER, uintptr(unsafe.Pointer(&program)), 0, 0)
}

// When both fresh stats of a search-denied parent are refused, the walk's own
// observation of it still makes a replacement refuse a stale confirmation.
func TestRefusedParentStatsKeepWalkedIdentity(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("permission bits do not restrict root")
	}
	if subprocess(t, "TestRefusedParentStatsKeepWalkedIdentity", "RESOLVER_WALKED_PARENT_HELPER") {
		return
	}
	e, f, q, o := fixture(t)
	e.Host = &confirmationHost{fakeHost: f}
	f.failStartup = true
	parent := filepath.Join(f.target.Directory, "config")
	t.Cleanup(func() { _ = os.Chmod(parent, 0700); _ = os.Chmod(parent+"-previous", 0700) })
	f.observing = func() { require.NoError(t, os.Chmod(parent, 0400)) }
	held, err := e.Run(t.Context(), q, *o)
	require.NoError(t, err)
	require.Equal(t, dto.RecoveryRequired, held.State)
	type result struct {
		err               error
		before, unchanged dto.FileState
		release           error
	}
	done := make(chan result, 1)
	go func() {
		runtime.LockOSThread()
		r := result{}
		step := func(err error) bool { r.err = err; return err == nil }
		if !step(denySyscall(unix.SYS_FSTAT, unix.EACCES)) || !step(denyEmptyPathStat()) {
			done <- r
			return
		}
		if r.before, r.err = resolver.Current(f.target); r.err != nil {
			done <- r
			return
		}
		if r.unchanged, r.err = resolver.Current(f.target); r.err != nil {
			done <- r
			return
		}
		if !step(os.Rename(parent, parent+"-previous")) || !step(os.Mkdir(parent, 0700)) ||
			!step(os.WriteFile(filepath.Join(parent, "static.yaml"), []byte("operator-version-two"), 0600)) || !step(os.Chmod(parent, 0400)) {
			done <- r
			return
		}
		_, r.release = e.Release(t.Context(), envelope(q), held, dto.ReleaseRequest{Confirmed: r.before, Actor: "admin-one"})
		done <- r
	}()
	select {
	case r := <-done:
		require.NoError(t, r.err)
		require.Equal(t, r.before, r.unchanged)
		requireCode(t, r.release, dto.TargetChanged)
	case <-time.After(5 * time.Second):
		t.Fatal("helper did not finish")
	}
}
