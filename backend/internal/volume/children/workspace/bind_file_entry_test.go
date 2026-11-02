// Source: docklet-next/issues/327
// Revision: 24b4de71db45699547c23815b5c17bc8ca547758
// Retrieved: 2026-11-02
// License: Apache-2.0
// Deviations: Original M16 single-file bind writes through the public update entry.

package workspace

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/florianfinn/docklet-next/backend/internal/common"
	"github.com/florianfinn/docklet-next/backend/internal/docker"
	"github.com/florianfinn/docklet-next/backend/pkg/utils"
	"github.com/florianfinn/docklet-next/types/user"
	"github.com/florianfinn/docklet-next/types/volume"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/stretchr/testify/require"
	"go.getarcane.app/acfs"
	"go.getarcane.app/acfs/types"
)

// bindFileHelper stands in for the helper container: /volume and /tmp/arcane-workspace are temp
// directories, shell commands run the shipped scripts with only shipped applets on PATH.
type bindFileHelper struct {
	t           *testing.T
	vol, stage  string
	inject      string
	scriptRuns  int
	applyCalled bool
}

func (h *bindFileHelper) mapPath(arg string) string {
	arg = strings.ReplaceAll(arg, "/tmp/arcane-workspace", h.stage)
	return strings.ReplaceAll(arg, "/volume", h.vol)
}

func (h *bindFileHelper) exec(ctx context.Context, _, _ string, cmd []string) (string, string, error) {
	switch cmd[0] {
	case "acfs":
		if cmd[1] != "read" {
			h.applyCalled = true
			return "", "", errors.New("generic ACFS path used for a file bind")
		}
		limit, err := strconv.ParseInt(cmd[len(cmd)-1], 10, 64)
		require.NoError(h.t, err)
		reader, size, err := acfs.OpenRead(ctx, h.vol, cmd[5], limit)
		if err != nil {
			return "", "", err
		}
		defer reader.Close()
		var out bytes.Buffer
		require.NoError(h.t, acfs.WriteStreamHeader(&out, uint64(size)))
		_, err = io.Copy(&out, reader)
		return out.String(), "", err
	case "sh", "stat":
		args := make([]string, len(cmd))
		for i, arg := range cmd {
			args[i] = h.mapPath(arg)
		}
		if cmd[0] == "sh" && strings.Contains(cmd[2], "live-backup") {
			h.scriptRuns++
			args[2] = h.inject + args[2]
		}
		command := exec.CommandContext(ctx, "/bin/sh", append([]string{"-c", `exec "$@"`, "sh"}, args...)...)
		command.Env = []string{"PATH=" + bindAppletPath(h.t)}
		var stdout, stderr bytes.Buffer
		command.Stdout, command.Stderr = &stdout, &stderr
		err := command.Run()
		return stdout.String(), stderr.String(), err
	}
	return "", "", errors.New("unexpected helper command " + cmd[0])
}

func (h *bindFileHelper) docker(source string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/info"):
			_, _ = io.WriteString(w, `{"DockerRootDir":"/var/lib/docker"}`)
		case strings.HasSuffix(r.URL.Path, "/containers/json"):
			require.NoError(h.t, json.NewEncoder(w).Encode([]container.Summary{{ID: "app", Mounts: []container.MountPoint{{Type: mount.TypeBind, Source: source}}}}))
		case strings.HasSuffix(r.URL.Path, "/containers/app/json"):
			_, _ = io.WriteString(w, `{"Id":"app","Config":{"User":""}}`)
		case strings.HasSuffix(r.URL.Path, "/containers/helper/exec"):
			_, _ = io.WriteString(w, `{"Id":"walk"}`)
		case strings.HasSuffix(r.URL.Path, "/exec/walk/start"):
			var output bytes.Buffer
			count := 0
			require.NoError(h.t, acfs.Walk(r.Context(), h.vol, "/", func(entry types.Entry) error {
				count++
				return json.NewEncoder(&output).Encode(types.WalkRecord{Version: types.ProtocolVersion, Entry: &entry})
			}))
			require.NoError(h.t, json.NewEncoder(&output).Encode(types.WalkRecord{Version: types.ProtocolVersion, End: true, Count: count}))
			writeBindAttachResponse(h.t, w, output.String())
		case strings.HasSuffix(r.URL.Path, "/exec/walk/json"):
			_, _ = io.WriteString(w, `{"Running":false,"ExitCode":0}`)
		case strings.HasSuffix(r.URL.Path, "/containers/helper/archive"):
			archive := tar.NewReader(r.Body)
			for {
				header, err := archive.Next()
				if err == io.EOF {
					break
				}
				require.NoError(h.t, err)
				data, err := io.ReadAll(archive)
				require.NoError(h.t, err)
				require.NoError(h.t, os.WriteFile(filepath.Join(h.stage, filepath.Base(header.Name)), data, 0600))
			}
		default:
			h.t.Errorf("unexpected Docker request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(500)
		}
	}
}

func writeBindAttachResponse(t *testing.T, w http.ResponseWriter, stdout string) {
	connection, buffer, err := w.(http.Hijacker).Hijack()
	require.NoError(t, err)
	defer func() { _ = connection.Close() }()
	_, _ = buffer.WriteString("HTTP/1.1 101 UPGRADED\r\nContent-Type: application/vnd.docker.raw-stream\r\nConnection: Upgrade\r\nUpgrade: tcp\r\n\r\n")
	header := make([]byte, 8)
	header[0] = 1
	binary.BigEndian.PutUint32(header[4:], uint32(len(stdout)))
	_, _ = buffer.Write(header)
	_, _ = buffer.WriteString(stdout)
	require.NoError(t, buffer.Flush())
}

// Shell functions that shadow head inside the write script: the staged copy writes two bytes and
// fails; "backup" additionally fails the write-back from the backup copy.
func headFailure(stage string, backup bool) string {
	script := "head() {\nif test \"$3\" = \"" + stage + "/change-0\"; then command head -c 2 \"$3\"; return 1; fi\n"
	if backup {
		script += "if test \"$3\" = \"" + stage + "/live-backup\"; then return 1; fi\n"
	}
	return script + "command head \"$@\"\n}\n"
}

func TestBindFileUpdateThroughPublicEntry(t *testing.T) {
	source := "/srv/example/app.conf"
	key := volume.BindKey(source)
	original := []byte("original contents")
	for _, tc := range []struct {
		name               string
		revision, baseline string
		inject             func(stage string) string
		status             int
		code               string
	}{
		{name: "success"},
		{name: "partial-write", inject: func(stage string) string { return headFailure(stage, false) }, status: 500},
		{name: "write-back-fails", inject: func(stage string) string { return headFailure(stage, true) }, status: 500, code: "volume.bind.recovery-failed"},
		{name: "stale-revision", revision: "stale", status: 409},
		{name: "stale-baseline", baseline: "changed elsewhere", status: 409},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := &bindFileHelper{t: t, vol: t.TempDir(), stage: t.TempDir()}
			if tc.inject != nil {
				h.inject = tc.inject(h.stage)
			}
			live := filepath.Join(h.vol, "app.conf")
			require.NoError(t, os.WriteFile(live, original, 0640))
			before, err := os.Stat(live)
			require.NoError(t, err)
			server := httptest.NewServer(h.docker(source))
			t.Cleanup(server.Close)
			svc := NewService(Dependencies{
				Docker:        docker.NewDockerClientService(t.Context(), nil, nil, nil).WithClient(newVolumeWorkspaceTestDockerClientInternal(t, server)),
				Locks:         &utils.KeyedMutex{},
				BindKind:      func(string) string { return "file" },
				AcquireHelper: func(context.Context, string) (string, func(), error) { return "helper", func() {}, nil },
				RequireACFS:   func(context.Context, string, string) error { return nil },
				Exec:          h.exec,
			})
			tree, err := svc.GetVolumeWorkspace(t.Context(), key)
			require.NoError(t, err)
			revision := tree.FileTreeRevision
			if tc.revision != "" {
				revision = tc.revision
			}
			baseline := string(original)
			if tc.baseline != "" {
				baseline = tc.baseline
			}
			upload, base := 0, 1
			manifest := volume.WorkspaceUpdateManifest{FileTreeRevision: revision, FileChanges: []volume.WorkspaceFileChange{{Operation: volume.FileOpUpdateFile, RelativePath: "app.conf", UploadIndex: &upload, BaselineIndex: &base}}}
			_, err = svc.UpdateVolumeWorkspace(t.Context(), key, manifest, map[int][]byte{0: []byte("replacement"), 1: []byte(baseline)}, user.SystemUser)

			actual, readErr := os.ReadFile(live)
			require.NoError(t, readErr)
			after, statErr := os.Stat(live)
			require.NoError(t, statErr)
			require.Equal(t, before.Sys().(*syscall.Stat_t).Ino, after.Sys().(*syscall.Stat_t).Ino, "the mounted file keeps its inode")
			require.False(t, h.applyCalled, "a file bind never takes the rename-based ACFS path")
			if tc.status == 0 {
				require.NoError(t, err)
				require.Equal(t, "replacement", string(actual))
				return
			}
			require.Error(t, err)
			var status huma.StatusError
			require.True(t, errors.As(volumeWorkspaceHTTPErrorInternal(err), &status))
			require.Equal(t, tc.status, status.GetStatus(), err.Error())
			if tc.code != "" {
				require.EqualError(t, err, tc.code)
			} else {
				require.NotContains(t, err.Error(), "recovery-failed")
			}
			if tc.status == 409 {
				require.ErrorIs(t, err, common.ErrVolumeWorkspaceConflict)
				require.Zero(t, h.scriptRuns, "a refused write never reaches the live file")
				require.Equal(t, original, actual)
			} else if tc.code == "" {
				require.Equal(t, 1, h.scriptRuns)
				require.Equal(t, original, actual, "a failed write restores the original content")
			}
		})
	}
}
