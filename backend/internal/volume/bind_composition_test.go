// Source: docklet-next/issues/327
// Revision: 603c791b31a21c99323a2c3c88db36217589315f
// Retrieved: 2026-11-02
// License: Apache-2.0
// Deviations: Original M16 bind admission, layout and write identity through the production volume service.

package volume

import (
	"archive/tar"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"

	"github.com/florianfinn/docklet-next/backend/internal/docker"
	"github.com/florianfinn/docklet-next/backend/internal/volume/bindpath"
	"github.com/florianfinn/docklet-next/types/user"
	volumetypes "github.com/florianfinn/docklet-next/types/volume"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/stretchr/testify/require"
	"go.getarcane.app/acfs"
	"go.getarcane.app/acfs/types"
)

// helperDaemon is a fake Docker daemon whose helper commands run locally: host paths live below
// hostRoot, each container gets its own root, and bind mounts map onto the host paths (file
// mounts as hard links, so writes reach the same inode).
type helperDaemon struct {
	t                   *testing.T
	hostRoot, dockerDir string
	consumerUser        string
	source              string
	mu                  sync.Mutex
	created             []container.HostConfig
	roots               map[string]string
	mounts              map[string][]mount.Mount
	execs               map[string][]string
	execOn              map[string]string
	exits               map[string]int
	commands            [][]string
	stagedUIDs          []int
	applets             string
}

func newHelperDaemon(t *testing.T, source string, directory bool) *helperDaemon {
	d := &helperDaemon{t: t, hostRoot: t.TempDir(), dockerDir: "/var/lib/docker", source: source, roots: map[string]string{}, mounts: map[string][]mount.Mount{}, execs: map[string][]string{}, execOn: map[string]string{}, exits: map[string]int{}, applets: appletPath(t)}
	host := d.host(source)
	if directory {
		require.NoError(t, os.MkdirAll(host, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(host, "a.txt"), []byte("original"), 0o644))
	} else {
		require.NoError(t, os.MkdirAll(filepath.Dir(host), 0o755))
		require.NoError(t, os.WriteFile(host, []byte("original"), 0o644))
	}
	return d
}

func (d *helperDaemon) host(p string) string { return filepath.Join(d.hostRoot, p) }

// mapper rewrites container paths of one container to local paths in a single pass.
func (d *helperDaemon) mapper(id string) func(string) string {
	forward, _ := d.mappers(id)
	return forward
}

// mappers returns the container-to-local rewrite and its inverse for command output.
func (d *helperDaemon) mappers(id string) (func(string) string, func(string) string) {
	root := d.roots[id]
	rules := map[string]string{"/tmp/arcane-workspace": filepath.Join(root, "tmp/arcane-workspace"), "/volume": filepath.Join(root, "volume"), "/source": filepath.Join(root, "source"), "/scratch": filepath.Join(root, "scratch")}
	for _, m := range d.mounts[id] {
		if info, err := os.Stat(d.host(m.Source)); err == nil && info.IsDir() {
			rules[m.Target] = d.host(m.Source)
		}
	}
	inverse := map[string]string{}
	for k, v := range rules {
		inverse[v] = k
	}
	return replacer(rules), replacer(inverse)
}

func replacer(rules map[string]string) func(string) string {
	keys := make([]string, 0, len(rules))
	for k := range rules {
		keys = append(keys, regexp.QuoteMeta(k))
	}
	sort.Slice(keys, func(i, j int) bool { return len(keys[i]) > len(keys[j]) })
	pattern := regexp.MustCompile(strings.Join(keys, "|"))
	return func(arg string) string {
		return pattern.ReplaceAllStringFunc(arg, func(m string) string { return rules[m] })
	}
}

func (d *helperDaemon) create(r *http.Request, w http.ResponseWriter) {
	var body struct {
		container.Config
		HostConfig container.HostConfig
	}
	require.NoError(d.t, json.NewDecoder(r.Body).Decode(&body))
	d.mu.Lock()
	defer d.mu.Unlock()
	id := fmt.Sprintf("helper-%d", len(d.created))
	d.created = append(d.created, body.HostConfig)
	root := d.t.TempDir()
	require.NoError(d.t, os.MkdirAll(filepath.Join(root, "tmp"), 0o755))
	require.NoError(d.t, os.MkdirAll(filepath.Join(root, "volume"), 0o755))
	for _, m := range body.HostConfig.Mounts {
		if info, err := os.Stat(d.host(m.Source)); err == nil && !info.IsDir() {
			target := filepath.Join(root, m.Target)
			require.NoError(d.t, os.MkdirAll(filepath.Dir(target), 0o755))
			require.NoError(d.t, os.Link(d.host(m.Source), target))
		}
	}
	d.roots[id], d.mounts[id] = root, body.HostConfig.Mounts
	_, _ = fmt.Fprintf(w, `{"Id":%q}`, id)
}

func (d *helperDaemon) run(id string, cmd []string) (string, int) {
	d.mu.Lock()
	m, back := d.mappers(id)
	d.commands = append(d.commands, cmd)
	d.mu.Unlock()
	if cmd[0] == "acfs" {
		return d.acfs(m, cmd)
	}
	args := make([]string, len(cmd))
	for i, arg := range cmd {
		args[i] = m(arg)
	}
	command := exec.CommandContext(d.t.Context(), "/bin/sh", append([]string{"-c", `exec "$@"`, "sh"}, args...)...)
	command.Env = []string{"PATH=" + d.applets}
	var stdout bytes.Buffer
	command.Stdout = &stdout
	if err := command.Run(); err != nil {
		code := 1
		if exitErr, ok := err.(*exec.ExitError); ok {
			code = exitErr.ExitCode()
		}
		return back(stdout.String()), code
	}
	return back(stdout.String()), 0
}

func (d *helperDaemon) acfs(m func(string) string, cmd []string) (string, int) {
	flag := func(name string) string {
		for i := range cmd[:len(cmd)-1] {
			if cmd[i] == name {
				return cmd[i+1]
			}
		}
		return ""
	}
	var out bytes.Buffer
	switch cmd[1] {
	case "version":
		require.NoError(d.t, json.NewEncoder(&out).Encode(types.VersionResponse{Protocol: types.ProtocolVersion}))
	case "walk":
		count := 0
		require.NoError(d.t, acfs.Walk(d.t.Context(), m(flag("--root")), "/", func(entry types.Entry) error {
			count++
			return json.NewEncoder(&out).Encode(types.WalkRecord{Version: types.ProtocolVersion, Entry: &entry})
		}))
		require.NoError(d.t, json.NewEncoder(&out).Encode(types.WalkRecord{Version: types.ProtocolVersion, End: true, Count: count}))
	case "read":
		limit, err := strconv.ParseInt(flag("--limit"), 10, 64)
		if err != nil {
			limit = 1 << 20
		}
		reader, size, err := acfs.OpenRead(d.t.Context(), m(flag("--root")), flag("--path"), limit)
		if err != nil {
			return "", 1
		}
		defer reader.Close()
		require.NoError(d.t, acfs.WriteStreamHeader(&out, uint64(size)))
		_, _ = io.Copy(&out, reader)
	case "apply":
		staging := m(flag("--staging"))
		data, err := os.ReadFile(filepath.Join(staging, flag("--manifest")))
		require.NoError(d.t, err)
		var manifest types.ApplyManifest
		require.NoError(d.t, json.Unmarshal(data, &manifest))
		count, err := acfs.Apply(d.t.Context(), m(flag("--root")), staging, manifest)
		if err != nil {
			return "", 1
		}
		require.NoError(d.t, json.NewEncoder(&out).Encode(types.ApplyResponse{Version: types.ProtocolVersion, Applied: count}))
	default:
		return "", 127
	}
	return out.String(), 0
}

func (d *helperDaemon) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	p := r.URL.Path
	switch {
	case strings.HasSuffix(p, "/info"):
		_, _ = fmt.Fprintf(w, `{"DockerRootDir":%q}`, d.dockerDir)
	case strings.HasSuffix(p, "/containers/json"):
		require.NoError(d.t, json.NewEncoder(w).Encode([]container.Summary{{ID: "app", Mounts: []container.MountPoint{{Type: mount.TypeBind, Source: d.source, Destination: "/data"}}}}))
	case strings.HasSuffix(p, "/containers/app/json"):
		_, _ = fmt.Fprintf(w, `{"Id":"app","Config":{"User":%q}}`, d.consumerUser)
	case strings.Contains(p, "/images/"):
		_, _ = io.WriteString(w, `{"Id":"tools"}`)
	case strings.HasSuffix(p, "/containers/create"):
		d.create(r, w)
	case strings.HasSuffix(p, "/start") && strings.Contains(p, "/containers/"), r.Method == "DELETE":
		w.WriteHeader(204)
	case strings.Contains(p, "/containers/") && strings.HasSuffix(p, "/json"):
		_, _ = io.WriteString(w, `{"Id":"helper","State":{"Running":true}}`)
	case strings.Contains(p, "/containers/") && strings.HasSuffix(p, "/exec"):
		var body struct{ Cmd []string }
		require.NoError(d.t, json.NewDecoder(r.Body).Decode(&body))
		d.mu.Lock()
		id := fmt.Sprintf("exec-%d", len(d.execs))
		d.execs[id], d.execOn[id] = body.Cmd, strings.Split(p, "/")[len(strings.Split(p, "/"))-2]
		d.mu.Unlock()
		_, _ = fmt.Fprintf(w, `{"Id":%q}`, id)
	case strings.Contains(p, "/exec/") && strings.HasSuffix(p, "/start"):
		_, _ = io.Copy(io.Discard, r.Body)
		id := strings.Split(p, "/")[len(strings.Split(p, "/"))-2]
		d.mu.Lock()
		cmd, on := d.execs[id], d.execOn[id]
		d.mu.Unlock()
		out, code := d.run(on, cmd)
		d.mu.Lock()
		d.exits[id] = code
		d.mu.Unlock()
		writeDockerExecAttachResponseInternal(d.t, w, out)
	case strings.Contains(p, "/exec/") && strings.HasSuffix(p, "/json"):
		id := strings.Split(p, "/")[len(strings.Split(p, "/"))-2]
		d.mu.Lock()
		code := d.exits[id]
		d.mu.Unlock()
		_, _ = fmt.Fprintf(w, `{"Running":false,"ExitCode":%d}`, code)
	case strings.HasSuffix(p, "/archive") && r.Method == "PUT":
		id := strings.Split(p, "/")[len(strings.Split(p, "/"))-2]
		d.mu.Lock()
		dest := d.mapper(id)(r.URL.Query().Get("path"))
		d.mu.Unlock()
		archive := tar.NewReader(r.Body)
		for {
			header, err := archive.Next()
			if err == io.EOF {
				break
			}
			require.NoError(d.t, err)
			// Like Docker, copyUIDGID replaces archive ownership with the container user, which the image sets.
			uid := header.Uid
			if r.URL.Query().Get("copyUIDGID") == "true" {
				uid = toolsImageUID(d.t)
			}
			d.mu.Lock()
			d.stagedUIDs = append(d.stagedUIDs, uid)
			d.mu.Unlock()
			data, err := io.ReadAll(archive)
			require.NoError(d.t, err)
			require.NoError(d.t, os.MkdirAll(dest, 0o755))
			require.NoError(d.t, os.WriteFile(filepath.Join(dest, filepath.Base(header.Name)), data, 0o600))
		}
	default:
		d.t.Errorf("unexpected Docker request %s %s", r.Method, p)
		w.WriteHeader(500)
	}
}

// toolsImageUID is the USER of the pinned tools image (deploy/tools/inputs.env RUNTIME_USER).
func toolsImageUID(t *testing.T) int {
	t.Helper()
	data, err := os.ReadFile("../../../deploy/tools/inputs.env")
	require.NoError(t, err)
	for _, line := range strings.Split(string(data), "\n") {
		if user, ok := strings.CutPrefix(line, "RUNTIME_USER="); ok {
			uid, err := strconv.Atoi(strings.Split(user, ":")[0])
			require.NoError(t, err)
			return uid
		}
	}
	t.Fatal("RUNTIME_USER missing")
	return 0
}

func appletPath(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile("../../../deploy/tools/applets.txt")
	require.NoError(t, err)
	dir := t.TempDir()
	for _, line := range strings.Split(string(data), "\n") {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		binary, err := exec.LookPath(line)
		require.NoError(t, err)
		require.NoError(t, os.Symlink(binary, filepath.Join(dir, line)))
	}
	return dir
}

func (d *helperDaemon) service(t *testing.T) *VolumeService {
	server := httptest.NewServer(d)
	t.Cleanup(server.Close)
	return NewVolumeService(nil, docker.NewDockerClientService(t.Context(), nil, nil, nil).WithClient(newVolumeWorkspaceTestDockerClientInternal(t, server)), nil, nil, nil, nil, nil, nil, nil, nil)
}

func (d *helperDaemon) ran(script string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, cmd := range d.commands {
		if len(cmd) > 2 && cmd[0] == "sh" && cmd[2] == script {
			return true
		}
	}
	return false
}

// BM2/BM5/BM17: the daemon-reported Docker root, the witnessed file layout and the parent's
// BindKind wiring all decide a single-file write through the production volume service.
func TestBindFileWriteThroughVolumeService(t *testing.T) {
	source := "/srv/example/app.conf"
	key := volumetypes.BindKey(source)
	d := newHelperDaemon(t, source, false)
	svc := d.service(t)
	live := d.host(source)
	before, err := os.Stat(live)
	require.NoError(t, err)
	tree, err := svc.workspace.GetVolumeWorkspace(t.Context(), key)
	require.NoError(t, err)
	upload, base := 0, 1
	manifest := volumetypes.WorkspaceUpdateManifest{FileTreeRevision: tree.FileTreeRevision, FileChanges: []volumetypes.WorkspaceFileChange{{Operation: volumetypes.FileOpUpdateFile, RelativePath: "app.conf", UploadIndex: &upload, BaselineIndex: &base}}}
	_, err = svc.workspace.UpdateVolumeWorkspace(t.Context(), key, manifest, map[int][]byte{0: []byte("replacement"), 1: []byte("original")}, user.SystemUser)
	require.NoError(t, err)

	content, err := os.ReadFile(live)
	require.NoError(t, err)
	after, err := os.Stat(live)
	require.NoError(t, err)
	require.Equal(t, "replacement", string(content))
	require.Equal(t, before.Sys().(*syscall.Stat_t).Ino, after.Sys().(*syscall.Stat_t).Ino)
	require.True(t, d.ran(bindpath.FileWriteScript), "the parent's BindKind routes the file key to the in-place write")
	helperMounts := []mount.Mount{}
	for _, created := range d.created {
		helperMounts = append(helperMounts, created.Mounts...)
	}
	require.Contains(t, helperMounts, mount.Mount{Type: mount.TypeBind, Source: source, Target: "/volume/app.conf"}, "the helper uses the witnessed file layout")
	require.Equal(t, "file", svc.bindKind(key))

	// A daemon whose Docker root contains the source refuses before any helper exists.
	d2 := newHelperDaemon(t, source, false)
	d2.dockerDir = "/srv/example"
	_, err = d2.service(t).workspace.GetVolumeWorkspace(t.Context(), key)
	require.EqualError(t, err, "volume.bind.path-denied")
	require.Empty(t, d2.created)
}

// BM15: a directory bind's generic write stages uploads with the consumer's declared identity.
func TestBindDirectoryWriteUsesConsumerIdentity(t *testing.T) {
	source := "/srv/example/data"
	key := volumetypes.BindKey(source)
	d := newHelperDaemon(t, source, true)
	d.consumerUser = "1000:1000"
	svc := d.service(t)
	tree, err := svc.workspace.GetVolumeWorkspace(t.Context(), key)
	require.NoError(t, err)
	upload := 0
	manifest := volumetypes.WorkspaceUpdateManifest{FileTreeRevision: tree.FileTreeRevision, FileChanges: []volumetypes.WorkspaceFileChange{{Operation: volumetypes.FileOpCreateFile, RelativePath: "b.txt", UploadIndex: &upload}}}
	_, err = svc.workspace.UpdateVolumeWorkspace(t.Context(), key, manifest, map[int][]byte{0: []byte("new")}, user.SystemUser)
	require.NoError(t, err)
	content, err := os.ReadFile(filepath.Join(d.host(source), "b.txt"))
	require.NoError(t, err)
	require.Equal(t, "new", string(content))
	require.NotEmpty(t, d.stagedUIDs)
	for _, uid := range d.stagedUIDs {
		require.Equal(t, 1000, uid, "staged uploads carry the consumer identity")
	}
	require.Equal(t, "directory", svc.bindKind(key))
}

// BM19: a directory size scan counts each regular file once through find, stat and aggregation.
func TestBindDirectorySizeThroughVolumeService(t *testing.T) {
	source := "/srv/example/data"
	d := newHelperDaemon(t, source, true)
	host := d.host(source)
	require.NoError(t, os.MkdirAll(filepath.Join(host, "nested"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(host, "nested", "b.bin"), bytes.Repeat([]byte{1}, 100), 0o644))
	require.NoError(t, os.Link(filepath.Join(host, "nested", "b.bin"), filepath.Join(host, "b-link.bin")))
	result, err := d.service(t).MeasureBind(t.Context(), volumetypes.BindKey(source))
	require.NoError(t, err)
	require.Equal(t, [2]any{int64(len("original") + 100), false}, [2]any{result.MeasuredSize, result.MeasuredPartial}, "a.txt plus one copy of the hard-linked b.bin")
}
