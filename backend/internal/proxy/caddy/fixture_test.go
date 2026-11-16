// Source: docklet-next/issues/474
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-16
// License: Apache-2.0
// Deviations: Original Docker HTTP seam fixture; no real service required.

package caddy

import (
	"archive/tar"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	dockerinternal "github.com/florianfinn/docklet-next/backend/internal/docker"
	"github.com/florianfinn/docklet-next/backend/internal/proxy"
	"github.com/florianfinn/docklet-next/backend/internal/proxy/hostio"
	"github.com/florianfinn/docklet-next/backend/internal/settings"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	dto "github.com/florianfinn/docklet-next/types/proxy"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm/logger"
)

const epoch = "2026-11-16T01:00:00Z"
const nextEpoch = "2026-11-16T02:00:00Z"
const rootText = "{\n admin localhost:2019\n}\n# existing comment\nlegacy.example.net {\n respond ok\n}\nimport /etc/caddy/routes/*.caddy\n"

type dockerFixture struct {
	t         *testing.T
	mu        sync.Mutex
	root      string
	c         container.InspectResponse
	d         *proxy.DockerRuntime
	e         *engine
	a         *proxy.Agent
	i         dto.Instance
	execs     map[string][]string
	result    map[string]hostio.ExecResult
	commands  [][]string
	execFn    func([]string) hostio.ExecResult
	restartFn func()
	restarts  int
	logs      string
	since     []string
	now       time.Time
	waits     int
	fault     func(http.ResponseWriter, *http.Request) bool
}

var fixtureSeedOnce sync.Once
var fixtureSeedBytes []byte

// Copy a closed, migrated test database rather than repeat unrelated settings
// inserts in every Docker seam case. Each fixture still opens its own real DB.
func seededDatabase(t *testing.T) string {
	t.Helper()
	fixtureSeedOnce.Do(func() {
		database.SetGormLogger(logger.Default.LogMode(logger.Silent))
		seed := filepath.Join(t.TempDir(), "seed.db")
		db, err := database.Initialize(t.Context(), sqlitestore.URL(seed), database.MigrationOptions{})
		require.NoError(t, err)
		settingsService, err := settings.NewSettingsService(t.Context(), db)
		require.NoError(t, err)
		require.NoError(t, settingsService.EnsureDefaultSettings(t.Context()))
		require.NoError(t, settingsService.Stop(context.Background()))
		require.NoError(t, db.Close())
		fixtureSeedBytes, err = os.ReadFile(seed)
		require.NoError(t, err)
	})
	target := filepath.Join(t.TempDir(), "proxy.db")
	require.NoError(t, os.WriteFile(target, fixtureSeedBytes, 0600))
	return target
}

func fixture(t *testing.T) *dockerFixture {
	t.Helper()
	root := t.TempDir()
	db, err := database.Initialize(t.Context(), sqlitestore.URL(seededDatabase(t)), database.MigrationOptions{})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	s, err := settings.NewSettingsService(t.Context(), db)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, s.Stop(context.Background())) })
	require.NoError(t, s.UpdateSetting(t.Context(), "projectsDirectory", root))
	x := &dockerFixture{t: t, root: root, execs: map[string][]string{}, result: map[string]hostio.ExecResult{}, now: time.Date(2026, 11, 16, 1, 0, 0, 0, time.UTC)}
	x.c = container.InspectResponse{ID: "example-caddy", Name: "/example-caddy", Config: &container.Config{Entrypoint: []string{"caddy"}, Cmd: []string{"run", "--config", "/etc/caddy/Caddyfile", "--adapter", "caddyfile"}}, HostConfig: &container.HostConfig{}, State: &container.State{Running: true, StartedAt: epoch}, Mounts: []container.MountPoint{{Type: mount.TypeBind, Source: root, Destination: "/etc/caddy", RW: true}}}
	x.put("Caddyfile", rootText)
	require.NoError(t, os.MkdirAll(filepath.Join(root, "routes"), 0755))
	server := httptest.NewServer(http.HandlerFunc(x.serve))
	t.Cleanup(server.Close)
	x.d = &proxy.DockerRuntime{Docker: dockerinternal.NewDockerClientService(t.Context(), nil, &config.Config{DockerHost: "tcp://" + strings.TrimPrefix(server.URL, "http://")}, nil), Settings: s, DB: db}
	x.e = New(x.d).(*engine)
	x.e.now = func() time.Time { x.mu.Lock(); defer x.mu.Unlock(); return x.now }
	x.e.wait = func(ctx context.Context, d time.Duration) error {
		x.mu.Lock()
		defer x.mu.Unlock()
		x.waits++
		x.now = x.now.Add(d)
		return ctx.Err()
	}
	x.d.Engines = proxy.Engines{dto.Caddy: x.e}
	x.a, err = proxy.NewAgent(db.DB, x.d, x.d.Engines)
	require.NoError(t, err)
	x.i = dto.Instance{ID: "example-instance", Kind: dto.Caddy, Identity: dto.Identity{ContainerID: x.c.ID, ContainerName: "example-caddy", StartedAt: epoch}, Source: dto.Source{Kind: dto.ProjectSource, Canonical: root, MountTarget: "/etc/caddy"}, RootFile: "/etc/caddy/Caddyfile", RouteDirectory: "/etc/caddy/routes", Eligibility: dto.Eligibility{Writable: true}, Registration: "example-registration", Generation: x.a.Generation()}
	return x
}
func (x *dockerFixture) put(p, text string) {
	x.t.Helper()
	require.NoError(x.t, os.MkdirAll(filepath.Dir(filepath.Join(x.root, p)), 0755))
	require.NoError(x.t, os.WriteFile(filepath.Join(x.root, p), []byte(text), 0644))
}
func (x *dockerFixture) bytes(p string) string {
	x.t.Helper()
	b, err := os.ReadFile(filepath.Join(x.root, p))
	require.NoError(x.t, err)
	return string(b)
}
func (x *dockerFixture) request(kind string, s Site) dto.ApplyRequest {
	x.t.Helper()
	target := "routes/app.caddy"
	var g map[string]string
	var err error
	require.NotPanics(x.t, func() { g, err = x.d.Graph(x.t.Context(), x.i) })
	require.NoError(x.t, err)
	old, err := x.d.Target(x.t.Context(), x.i, target)
	require.NoError(x.t, err)
	text, err := renderChecked(x.t, s)
	require.NoError(x.t, err)
	return dto.ApplyRequest{ID: fmt.Sprintf("example-operation-%d", len(x.commands)), Instance: x.i, Generation: x.a.Generation(), Registration: x.i.Registration, Target: target, Expected: old.FileState, GraphHashes: g, Change: dto.Change{Kind: kind, RouteID: target, Advanced: text}}
}
func baseSite() Site {
	return Site{Name: "example-app", Domain: "app.example.com", TLS: "off", Upstreams: []string{"http://192.0.2.80:8080"}, Middlewares: []string{}}
}
func httpResult(code int, body string) hostio.ExecResult {
	return hostio.ExecResult{Stdout: []byte(body), Stderr: []byte(fmt.Sprintf("  HTTP/1.1 %d Example\n  Server: Caddy\n\n", code))}
}
func (x *dockerFixture) output(args []string) hostio.ExecResult {
	x.commands = append(x.commands, append([]string(nil), args...))
	if x.execFn != nil {
		return x.execFn(args)
	}
	if args[0] == "wget" {
		if strings.Contains(args[len(args)-1], "/config/") {
			return httpResult(200, "{}")
		}
		return httpResult(404, "")
	}
	if r, ok := x.result[args[1]]; ok {
		return r
	}
	if args[1] == "adapt" {
		return hostio.ExecResult{Stdout: []byte("{}")}
	}
	return hostio.ExecResult{}
}

var apiVersion = regexp.MustCompile(`^/v[0-9.]+`)

func (x *dockerFixture) serve(w http.ResponseWriter, r *http.Request) {
	x.mu.Lock()
	defer x.mu.Unlock()
	if x.fault != nil && x.fault(w, r) {
		return
	}
	p := apiVersion.ReplaceAllString(r.URL.Path, "")
	switch {
	case p == "/_ping":
		w.Header().Set("API-Version", "1.54")
		_, _ = w.Write([]byte("OK"))
	case strings.HasPrefix(p, "/exec/") && strings.HasSuffix(p, "/json"):
		id := strings.Split(p, "/")[2]
		res := x.result[id]
		_ = json.NewEncoder(w).Encode(map[string]any{"ID": id, "Running": false, "ExitCode": res.ExitCode})
	case strings.HasPrefix(p, "/exec/") && strings.HasSuffix(p, "/start"):
		id := strings.Split(p, "/")[2]
		res := x.output(x.execs[id])
		x.result[id] = res
		conn, rw, err := w.(http.Hijacker).Hijack()
		if err != nil {
			x.t.Error(err)
			return
		}
		defer conn.Close()
		_, _ = rw.WriteString("HTTP/1.1 101 Switching Protocols\r\nConnection: Upgrade\r\nUpgrade: tcp\r\n\r\n")
		_ = rw.Flush()
		writeStream(conn, 1, res.Stdout)
		writeStream(conn, 2, res.Stderr)
	case strings.HasSuffix(p, "/exec"):
		var v struct{ Cmd []string }
		if json.NewDecoder(r.Body).Decode(&v) != nil {
			w.WriteHeader(400)
			return
		}
		id := fmt.Sprintf("exec-%d", len(x.execs))
		x.execs[id] = v.Cmd
		_ = json.NewEncoder(w).Encode(map[string]any{"Id": id})
	case strings.HasSuffix(p, "/logs"):
		x.since = append(x.since, r.URL.Query().Get("since"))
		writeStream(w, 1, []byte(x.logs))
	case strings.HasSuffix(p, "/restart"):
		x.restarts++
		if x.restartFn != nil {
			x.restartFn()
		} else {
			x.c.State.StartedAt = fmt.Sprintf("2026-11-16T%02d:00:00Z", x.restarts+1)
			x.c.State.Running = true
		}
		w.WriteHeader(204)
	case strings.HasSuffix(p, "/json") && strings.HasPrefix(p, "/containers/"):
		_ = json.NewEncoder(w).Encode(x.c)
	case strings.HasSuffix(p, "/archive"):
		abs := r.URL.Query().Get("path")
		rel := strings.TrimPrefix(abs, "/etc/caddy/")
		if abs == "/etc/caddy" {
			rel = "."
		}
		disk := filepath.Join(x.root, rel)
		info, err := os.Stat(disk)
		if err != nil {
			http.Error(w, `{"message":"not found"}`, 404)
			return
		}
		mode := info.Mode()
		for _, m := range x.c.Mounts {
			if abs == m.Destination && strings.HasSuffix(m.Source, "Caddyfile") {
				mode = 0644
			}
		}
		raw, _ := json.Marshal(container.PathStat{Name: path.Base(abs), Mode: mode, Size: info.Size()})
		w.Header().Set("X-Docker-Container-Path-Stat", base64.StdEncoding.EncodeToString(raw))
		if r.Method == "HEAD" {
			return
		}
		tw := tar.NewWriter(w)
		defer tw.Close()
		if info.IsDir() {
			_ = filepath.WalkDir(disk, func(p string, d fs.DirEntry, err error) error {
				if err != nil {
					return err
				}
				st, err := d.Info()
				if err != nil {
					return err
				}
				suffix, err := filepath.Rel(filepath.Dir(disk), p)
				if err != nil {
					return err
				}
				return tarFile(tw, p, filepath.ToSlash(suffix), st)
			})
		} else {
			_ = tarFile(tw, disk, path.Base(abs), info)
		}
	default:
		http.Error(w, `{"message":"not found"}`, 404)
	}
}
func tarFile(tw *tar.Writer, disk, name string, info os.FileInfo) error {
	h, err := tar.FileInfoHeader(info, "")
	if err != nil {
		return err
	}
	h.Name = name
	if err = tw.WriteHeader(h); err != nil {
		return err
	}
	if info.IsDir() {
		return nil
	}
	f, err := os.Open(disk)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(tw, f)
	return err
}
func (x *dockerFixture) noTransient() {
	x.t.Helper()
	_ = filepath.WalkDir(x.root, func(p string, d fs.DirEntry, err error) error {
		require.NoError(x.t, err)
		require.False(x.t, strings.HasPrefix(d.Name(), ".docklet-"), "transient left: %s", d.Name())
		return nil
	})
}
func (x *dockerFixture) owned(p, text string) {
	x.put(p, text)
	b := proxy.Snapshot([]byte(text), true)
	require.NoError(x.t, x.d.DB.Table("proxy_verified_files").Create(map[string]any{"source": "host\x00" + x.i.Source.Canonical, "path": p, "hash": b.Hash, "content": []byte(text)}).Error)
}

func writeStream(w io.Writer, stream byte, b []byte) {
	header := make([]byte, 8)
	header[0] = stream
	binary.BigEndian.PutUint32(header[4:], uint32(len(b)))
	_, _ = w.Write(header)
	_, _ = w.Write(b)
}
