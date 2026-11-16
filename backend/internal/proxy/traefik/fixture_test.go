// Source: docklet-next/issues/473
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-16
// License: Apache-2.0
// Deviations: Original Traefik v3 engine.

package traefik

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
	"github.com/moby/moby/api/pkg/stdcopy"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm/logger"
)

var epoch = time.Date(2026, 11, 16, 0, 0, 0, 0, time.UTC)

const tiny = "http:\n  routers:\n    app:\n      rule: Host(`app.example.com`)\n      entryPoints: [web]\n      service: app\n  services:\n    app:\n      loadBalancer:\n        servers: [{url: 'http://192.0.2.1:8080'}]\n"

type seam struct {
	inspectFailures int
	containerStatus int
	containers      []container.Summary
	archiveModified map[string]time.Time
	t               *testing.T
	mu              sync.Mutex
	root            string
	db              *database.DB
	d               *proxy.DockerRuntime
	e               *engine
	i               dto.Instance
	inspect         container.InspectResponse
	now             time.Time
	execs           map[string]hostio.ExecResult
	calls           [][]string
	archives        []string
	logs            string
	logSince        []string
	probeFunc       func([]string) hostio.ExecResult
	logStatus       int
	archiveStatus   map[string]int
	archiveFaults   int // archive requests answered from archiveStatus
}

func fixture(t *testing.T) *seam {
	t.Helper()
	x := &seam{t: t, root: t.TempDir(), now: epoch.Add(time.Hour), execs: map[string]hostio.ExecResult{}, logStatus: 200}
	db, err := database.Initialize(t.Context(), sqlitestore.URL(filepath.Join(t.TempDir(), "engine.db")), database.MigrationOptions{})
	require.NoError(t, err)
	x.db = db
	x.db.Logger = logger.Default.LogMode(logger.Silent)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	s, err := settings.NewSettingsService(t.Context(), db)
	require.NoError(t, err)
	require.NoError(t, s.EnsureDefaultSettings(t.Context()))
	require.NoError(t, s.UpdateSetting(t.Context(), "projectsDirectory", x.root))
	t.Cleanup(func() { require.NoError(t, s.Stop(context.WithoutCancel(t.Context()))) })
	x.i = dto.Instance{ID: "example-instance", Kind: dto.Traefik, Identity: dto.Identity{ContainerID: "example-container", ContainerName: "example-proxy", StartedAt: epoch.Format(time.RFC3339Nano)}, Source: dto.Source{Kind: dto.ProjectSource, Canonical: x.root, MountTarget: "/etc/traefik"}, RouteDirectory: "dynamic", Eligibility: dto.Eligibility{Writable: true}}
	x.inspect = container.InspectResponse{ID: x.i.Identity.ContainerID, Name: "/example-proxy", State: &container.State{Running: true, StartedAt: x.i.Identity.StartedAt}, Config: &container.Config{Cmd: []string{"--providers.file.directory=/etc/traefik/dynamic", "--entrypoints.web.address=:80", "--entrypoints.websecure.address=:443", "--providers.docker=true"}}, HostConfig: &container.HostConfig{}, Mounts: []container.MountPoint{{Source: x.root, Destination: "/etc/traefik", Type: mount.TypeBind, RW: true}}}
	server := httptest.NewServer(http.HandlerFunc(x.serve))
	t.Cleanup(server.Close)
	x.d = &proxy.DockerRuntime{Docker: dockerinternal.NewDockerClientService(t.Context(), nil, &config.Config{DockerHost: strings.Replace(server.URL, "http://", "tcp://", 1)}, nil), Settings: s, DB: db, Engines: proxy.Engines{}}
	x.e = New(x.d).(*engine)
	x.d.Engines[dto.Traefik] = x.e
	x.e.now = func() time.Time { return x.now }
	waits := 0
	x.e.wait = func(ctx context.Context, d time.Duration) error {
		waits++
		if waits > 50 {
			return fmt.Errorf("observation exceeded injected clock budget")
		}
		x.now = x.now.Add(max(d, 14*time.Second))
		return ctx.Err()
	}
	x.put("dynamic/routes.yml", tiny)
	return x
}
func (x *seam) put(p, s string) {
	x.t.Helper()
	full := filepath.Join(x.root, p)
	require.NoError(x.t, os.MkdirAll(filepath.Dir(full), 0755))
	require.NoError(x.t, os.WriteFile(full, []byte(s), 0644))
}
func (x *seam) read(p string) string {
	x.t.Helper()
	b, err := os.ReadFile(filepath.Join(x.root, p))
	require.NoError(x.t, err)
	return string(b)
}
func (x *seam) request(kind string) dto.ApplyRequest {
	x.t.Helper()
	var g map[string]string
	var err error
	require.NotPanics(x.t, func() { g, err = x.d.Graph(x.t.Context(), x.i) }, "graph reads must refuse input without crashing")
	require.NoError(x.t, err)
	b, err := x.d.Target(x.t.Context(), x.i, "dynamic/routes.yml")
	require.NoError(x.t, err)
	return dto.ApplyRequest{ID: "example-op", Instance: x.i, Target: "dynamic/routes.yml", Expected: b.FileState, GraphHashes: g, Generation: "example-generation", Registration: "example-registration", Change: dto.Change{Kind: kind, RouteID: "dynamic/routes.yml#app"}}
}
func (x *seam) prepare(r dto.ApplyRequest) proxy.Intent {
	x.t.Helper()
	var in proxy.Intent
	var err error
	require.NotPanics(x.t, func() { in, err = x.e.Prepare(x.t.Context(), r) }, "preparation must refuse input without crashing")
	require.NoError(x.t, err, "%+v", err)
	in.TempPath = "dynamic/.docklet-" + r.ID + ".tmp"
	in.MarkerPath = "dynamic/docklet-check-" + r.ID + ".yml"
	in.PublishedAt = x.now
	return in
}
func (x *seam) serve(w http.ResponseWriter, r *http.Request) {
	x.mu.Lock()
	defer x.mu.Unlock()
	p := regexp.MustCompile(`^/v[0-9.]+`).ReplaceAllString(r.URL.Path, "")
	switch {
	case p == "/_ping":
		w.Header().Set("API-Version", "1.54")
		_, _ = w.Write([]byte("OK"))
	case p == "/containers/json":
		if x.containerStatus != 0 {
			http.Error(w, `{"message":"list unavailable"}`, x.containerStatus)
			return
		}
		if x.containers != nil {
			_ = json.NewEncoder(w).Encode(x.containers)
			return
		}
		_ = json.NewEncoder(w).Encode([]container.Summary{{ID: "example-labelled", Names: []string{"/example-app"}, Labels: map[string]string{"traefik.http.routers.label.rule": "Host(`label.example.com`)", "traefik.http.routers.label.service": "label", "com.docker.compose.project": "example-stack"}}})
	case p == "/containers/example-proxy/json" || p == "/containers/example-container/json":
		if x.inspectFailures > 0 {
			x.inspectFailures--
			http.Error(w, `{"message":"container unavailable"}`, 503)
			return
		}
		_ = json.NewEncoder(w).Encode(x.inspect)
	case strings.HasSuffix(p, "/archive"):
		source := r.URL.Query().Get("path")
		if status := x.archiveStatus[source]; status != 0 {
			x.archiveFaults++
			http.Error(w, `{"message":"archive unavailable"}`, status)
			return
		}
		local := filepath.Join(x.root, strings.TrimPrefix(strings.TrimPrefix(source, "/etc/traefik"), "/"))
		st, err := os.Stat(local)
		if err != nil {
			http.Error(w, `{"message":"not found"}`, 404)
			return
		}
		raw, _ := json.Marshal(container.PathStat{Name: path.Base(source), Mode: st.Mode()})
		w.Header().Set("X-Docker-Container-Path-Stat", base64.StdEncoding.EncodeToString(raw))
		if r.Method == "HEAD" {
			return
		}
		x.archives = append(x.archives, source)
		tw := tar.NewWriter(w)
		defer tw.Close()
		_ = filepath.WalkDir(local, func(file string, de fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			info, err := de.Info()
			if err != nil {
				return err
			}
			rel, _ := filepath.Rel(local, file)
			if rel == "." {
				rel = path.Base(source)
			} else {
				rel = path.Join(path.Base(source), filepath.ToSlash(rel))
			}
			h, err := tar.FileInfoHeader(info, "")
			if err != nil {
				return err
			}
			h.Name = filepath.ToSlash(rel)
			h.ModTime = epoch
			rootRelative, _ := filepath.Rel(x.root, file)
			if changed, ok := x.archiveModified[filepath.ToSlash(rootRelative)]; ok {
				h.ModTime = changed
			}
			if hostio.Secret(h.Name) && !de.IsDir() {
				h.Typeflag = tar.TypeReg
				h.Size = 18
				_ = tw.WriteHeader(h)
				_, _ = tw.Write([]byte("opaque-payload-tag"))
				return nil
			}
			if err = tw.WriteHeader(h); err != nil {
				return err
			}
			if de.IsDir() {
				return nil
			}
			b, err := os.ReadFile(file)
			if err != nil {
				return err
			}
			_, err = tw.Write(b)
			return err
		})
	case strings.HasSuffix(p, "/exec"):
		var body struct{ Cmd []string }
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid", 400)
			return
		}
		x.calls = append(x.calls, body.Cmd)
		id := fmt.Sprintf("exec-%d", len(x.calls))
		res := response(200, "old")
		if x.probeFunc != nil {
			res = x.probeFunc(body.Cmd)
		}
		x.execs[id] = res
		_ = json.NewEncoder(w).Encode(map[string]string{"Id": id})
	case strings.HasPrefix(p, "/exec/") && strings.HasSuffix(p, "/start"):
		id := strings.Split(p, "/")[2]
		res := x.execs[id]
		conn, buf, err := w.(http.Hijacker).Hijack()
		if err != nil {
			return
		}
		defer conn.Close()
		_, _ = buf.WriteString("HTTP/1.1 101 UPGRADED\r\nContent-Type: application/vnd.docker.raw-stream\r\nConnection: Upgrade\r\nUpgrade: tcp\r\n\r\n")
		_ = buf.Flush()
		_, _ = frame(conn, byte(stdcopy.Stdout), res.Stdout)
		_, _ = frame(conn, byte(stdcopy.Stderr), res.Stderr)
	case strings.HasPrefix(p, "/exec/") && strings.HasSuffix(p, "/json"):
		id := strings.Split(p, "/")[2]
		_ = json.NewEncoder(w).Encode(map[string]any{"ID": id, "ExitCode": x.execs[id].ExitCode, "Running": false})
	case strings.HasSuffix(p, "/logs"):
		x.logSince = append(x.logSince, r.URL.Query().Get("since"))
		w.WriteHeader(x.logStatus)
		payload := x.logs
		if strings.HasPrefix(payload, "CURRENT ") {
			payload = time.Now().UTC().Format(time.RFC3339Nano) + strings.TrimPrefix(payload, "CURRENT")
		}
		_, _ = frame(w, byte(stdcopy.Stdout), []byte(payload))
	default:
		http.Error(w, `{"message":"not found"}`, 404)
	}
}

// response mirrors BusyBox wget -S: on 4xx/5xx it stops at the status line, so
// such answers have no headers and no body (measured B0, c-invalid step 28).
func response(status int, body string) hostio.ExecResult {
	if status >= 400 {
		line := fmt.Sprintf("HTTP/1.1 %d %s", status, http.StatusText(status))
		return hostio.ExecResult{ExitCode: 1, Stderr: []byte("Connecting to 127.0.0.1 (127.0.0.1:80)\n  " + line + "\nwget: server returned error: " + line + "\n")}
	}
	return hostio.ExecResult{Stdout: []byte(body), Stderr: []byte(fmt.Sprintf("  HTTP/1.1 %d Answer\n  Content-Type: text/plain\n\n", status))}
}

// measuredACME is the B0 record of an unrelated router's failing ACME issuance,
// logged again after every reload.
const measuredACME = `{"level":"error","providerName":"dns-primary.acme","acmeCA":"http://127.0.0.1:9/directory","routerName":"wildcard@file","rule":"HostRegexp(` + "`^[a-z0-9-]+\\\\.example\\\\.com$`" + `)","error":"cannot get ACME client get directory","domains":["example.com","*.example.com"],"message":"Unable to obtain ACME certificate for domains"}`

func requireCode(t *testing.T, err error, code dto.Reason) {
	t.Helper()
	var p *dto.Problem
	require.ErrorAs(t, err, &p)
	require.Equal(t, code, p.Code)
}

func frame(w io.Writer, stream byte, b []byte) (int, error) {
	h := make([]byte, 8)
	h[0] = stream
	binary.BigEndian.PutUint32(h[4:], uint32(len(b)))
	if _, err := w.Write(h); err != nil {
		return 0, err
	}
	return w.Write(b)
}

func (x *seam) inventory() (v dto.Inventory, err error) {
	x.t.Helper()
	require.NotPanics(x.t, func() { v, err = x.e.Inventory(x.t.Context(), x.i) }, "inventory rejects incomplete input without crashing")
	return v, err
}

func (x *seam) readResult() (v dto.ReadResult, err error) {
	x.t.Helper()
	require.NotPanics(x.t, func() { v, err = x.e.Read(x.t.Context(), x.i) }, "graph reads must refuse input without crashing")
	return v, err
}

func (x *seam) attempt(r dto.ApplyRequest) (in proxy.Intent, err error) {
	x.t.Helper()
	require.NotPanics(x.t, func() { in, err = x.e.Prepare(x.t.Context(), r) }, "preparation must handle invalid input without crashing")
	return in, err
}
func (x *seam) validate(in proxy.Intent) (err error) {
	x.t.Helper()
	require.NotPanics(x.t, func() { err = x.e.Validate(x.t.Context(), in) }, "validation must report bad graphs without crashing")
	return err
}
