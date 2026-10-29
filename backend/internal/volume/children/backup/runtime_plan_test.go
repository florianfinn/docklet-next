// Source: docklet-next/issues/283
// Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original command-plan proofs for mount witness, bounded reads and owned downloads.

package backup

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/florianfinn/docklet-next/backend/internal/event"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/backup"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/volume/children/backup/children/browser"
	"github.com/florianfinn/docklet-next/backend/internal/volume/children/backup/children/restore"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/rustic"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/volumehelper"
	"github.com/florianfinn/docklet-next/backend/pkg/pagination"
	"github.com/florianfinn/docklet-next/backend/pkg/utils"
	dto "github.com/florianfinn/docklet-next/types/backup"
	"github.com/florianfinn/docklet-next/types/volume"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/require"
)

type plannedContainer struct {
	container.Config
	HostConfig container.HostConfig
}
type runtimePlan struct {
	cleanupHeld    *atomic.Bool
	cleanupInspect chan struct{}
	volumeState    map[string]map[string]string
	foreignCreate  bool
	t              *testing.T
	service        *Service
	creates        []plannedContainer
	steps          []string
	scratch        map[string]string
	removed        []string
	packaged       int64
	witness        string
	stdout         string
	stderr         string
	exit           int64
	execBudget     time.Duration
	forgetEmpties  bool
}

func newRuntimePlan(t *testing.T) *runtimePlan {
	f := &runtimePlan{t: t, packaged: 6, witness: "42\n1", stdout: "[]"}
	restarted := []string{}
	f.service = startupService(t, http.HandlerFunc(f.serve), &restarted)
	require.NoError(t, f.service.deps.DB.AutoMigrate(&checkSetting{}))
	f.service.deps.Engine = backup.NewEngine(t.Context(), newVolumeAdmissionForTestInternal(t), nil)
	f.service.deps.Locks = &utils.KeyedMutex{}
	f.service.deps.HelperImage = func(context.Context, *client.Client) (string, error) { return volumehelper.DefaultToolsImage, nil }
	f.service.deps.Exec = func(ctx context.Context, id, _ string, command []string) (string, string, error) {
		f.steps = append(f.steps, strings.Join(command, " "))
		if deadline, ok := ctx.Deadline(); ok {
			f.execBudget = time.Until(deadline)
		}
		if command[0] == "stat" {
			return f.witness, "", nil
		}
		return "", "", nil
	}
	return f
}
func (f *runtimePlan) serve(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	path := r.URL.Path
	switch {
	case strings.Contains(path, "/images/") && strings.HasSuffix(path, "/json"):
		_, _ = w.Write([]byte(`{"Id":"engine-image"}`))
	case strings.HasSuffix(path, "/volumes"):
		_, _ = w.Write([]byte(`{"Volumes":[{"Name":"data"}]}`))
	case strings.HasSuffix(path, "/containers/json"):
		_, _ = w.Write([]byte(`[]`))
	case strings.HasSuffix(path, "/volumes/create"):
		var input client.VolumeCreateOptions
		require.NoError(f.t, json.NewDecoder(r.Body).Decode(&input))
		if strings.HasPrefix(input.Name, "docklet-backup-download-") {
			f.scratch = input.Labels
		}
		if f.volumeState != nil {
			labels := input.Labels
			if f.foreignCreate && input.Name == "recovered-data" {
				labels = map[string]string{restore.RestoreLabel: "foreign-operation"}
			}
			f.volumeState[input.Name] = labels
			_ = json.NewEncoder(w).Encode(map[string]any{"Name": input.Name, "Labels": labels})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"Name": input.Name})
	case strings.Contains(path, "/volumes/") && r.Method == http.MethodGet && f.volumeState != nil:
		name := path[strings.LastIndex(path, "/")+1:]
		labels, found := f.volumeState[name]
		if !found {
			w.WriteHeader(404)
			_, _ = w.Write([]byte(`{"message":"not found"}`))
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"Name": name, "Labels": labels})
	case strings.Contains(path, "/volumes/") && r.Method == http.MethodDelete:
		f.removed = append(f.removed, "scratch")
		w.WriteHeader(204)
	case strings.HasSuffix(path, "/containers/create"):
		var input plannedContainer
		require.NoError(f.t, json.NewDecoder(r.Body).Decode(&input))
		f.creates = append(f.creates, input)
		id := fmt.Sprintf("worker-%d", len(f.creates))
		f.steps = append(f.steps, "create:"+input.Image)
		_ = json.NewEncoder(w).Encode(map[string]string{"Id": id})
	case strings.HasSuffix(path, "/start"):
		w.WriteHeader(204)
	case strings.HasSuffix(path, "/wait"):
		_ = json.NewEncoder(w).Encode(map[string]any{"StatusCode": f.exit})
	case strings.HasSuffix(path, "/logs"):
		w.Header().Set("Content-Type", "application/vnd.docker.raw-stream")
		for index, text := range []string{f.stdout, f.stderr} {
			if text == "" {
				continue
			}
			header := make([]byte, 8)
			header[0] = byte(index + 1)
			binary.BigEndian.PutUint32(header[4:], uint32(len(text)))
			_, _ = w.Write(header)
			_, _ = w.Write([]byte(text))
		}
		if f.forgetEmpties {
			for i, created := range f.creates {
				if strings.Contains(path, fmt.Sprintf("/containers/worker-%d/", i+1)) && len(created.Cmd) > 0 && created.Cmd[0] == "forget" {
					f.stdout = "[]"
				}
			}
		}
	case strings.HasSuffix(path, "/archive") && r.Method == http.MethodPut:
		_, err := io.Copy(io.Discard, r.Body)
		require.NoError(f.t, err)
		w.WriteHeader(200)
	case strings.HasSuffix(path, "/archive"):
		stat, _ := json.Marshal(map[string]any{"name": "download.tar.gz", "size": f.packaged, "mode": 420})
		w.Header().Set("X-Docker-Container-Path-Stat", base64.StdEncoding.EncodeToString(stat))
		w.Header().Set("Content-Type", "application/x-tar")
		var buffer bytes.Buffer
		writer := tar.NewWriter(&buffer)
		require.NoError(f.t, writer.WriteHeader(&tar.Header{Name: "download.tar.gz", Mode: 0600, Size: f.packaged}))
		if f.packaged == 6 {
			_, _ = writer.Write([]byte("packed"))
			require.NoError(f.t, writer.Close())
		}
		_, _ = w.Write(buffer.Bytes())
	case strings.Contains(path, "/containers/") && r.Method == http.MethodDelete:
		f.removed = append(f.removed, "helper")
		w.WriteHeader(204)
	case strings.Contains(path, "/containers/") && strings.HasSuffix(path, "/json"):
		if f.cleanupHeld != nil && f.cleanupHeld.Load() {
			if f.cleanupInspect != nil {
				select {
				case f.cleanupInspect <- struct{}{}:
				default:
				}
			}
			w.WriteHeader(503)
			_, _ = w.Write([]byte(`{"message":"helper removal unconfirmed"}`))
			return
		}
		w.WriteHeader(404)
		_, _ = w.Write([]byte(`{"message":"not found"}`))
	default:
		f.t.Errorf("unexpected Docker request: %s %s", r.Method, path)
		w.WriteHeader(500)
	}
}
func (f *runtimePlan) targetContext(kind string) context.Context {
	host := "0"
	return context.WithValue(f.t.Context(), admittedTargetKey{}, dto.SyncedTarget{ID: "archive", TargetLocation: dto.TargetLocation{Kind: kind, HostID: &host, Path: "/srv/backups"}, RecoveryKey: "synthetic-key"})
}
func TestMountWitnessImmediatelyPrecedesEngineWithIdenticalPrivateBinds(t *testing.T) {
	for _, witness := range []string{"42\n1", "1\n1"} {
		t.Run(witness, func(t *testing.T) {
			f := newRuntimePlan(t)
			f.witness = witness
			ctx := f.targetContext("mount")
			cli, err := f.service.deps.Docker.GetClient(ctx)
			require.NoError(t, err)
			repository, err := f.service.repositoryFor(ctx, cli, "archive", true)
			require.NoError(t, err)
			_, err = f.service.deps.Engine.ListSnapshots(ctx, cli, repository, "synthetic-key")
			if witness == "1\n1" {
				require.ErrorContains(t, err, "backup.target.not-mounted")
				require.Len(t, f.creates, 1, "failed witness must prevent engine creation")
				return
			}
			require.NoError(t, err)
			require.Len(t, f.creates, 2)
			require.Equal(t, volumehelper.DefaultToolsImage, f.creates[0].Image)
			require.Equal(t, rustic.DefaultImage, f.creates[1].Image)
			binds := func(plan plannedContainer) []mount.Mount {
				var result []mount.Mount
				for _, m := range plan.HostConfig.Mounts {
					if m.Type == mount.TypeBind {
						result = append(result, m)
					}
				}
				return result
			}
			require.Equal(t, binds(f.creates[0]), binds(f.creates[1]))
			require.Len(t, binds(f.creates[1]), 2)
			for _, m := range binds(f.creates[1]) {
				require.Equal(t, mount.PropagationRPrivate, m.BindOptions.Propagation)
			}
			require.Equal(t, []string{"create:" + volumehelper.DefaultToolsImage, "stat -c %d /target /parent", "create:" + rustic.DefaultImage}, f.steps)
			require.Equal(t, []string{"helper", "helper"}, f.removed)
			f.witness = "1\n1"
			before := len(f.creates)
			_, err = f.service.deps.Engine.ListSnapshots(ctx, cli, repository, "synthetic-key")
			require.ErrorContains(t, err, "backup.target.not-mounted", "an earlier passing witness cannot authorize another use")
			require.Len(t, f.creates, before+1, "only the failed witness helper can be created")
		})
	}
}
func TestDownloadOwnsVolumeAndEnforcesEdgeBeforeAndAfterPackaging(t *testing.T) {
	for _, tc := range []struct {
		name                      string
		edge                      bool
		size, packaged            int64
		refuseBefore, refuseAfter bool
	}{
		{"unknown", true, -1, 6, true, false}, {"empty", true, 0, 6, false, false}, {"snapshot-large", true, dto.MaxEdgeDownloadBytes + 1, 6, true, false},
		{"package-large", true, 1, dto.MaxEdgeDownloadBytes + 1, false, true}, {"edge", true, 1, 6, false, false},
		{"direct-uncapped", false, dto.MaxEdgeDownloadBytes + 1, dto.MaxEdgeDownloadBytes + 1, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newRuntimePlan(t)
			f.packaged = tc.packaged
			entry := &volume.Backup{ID: "snapshot-row", VolumeName: "data", Destination: "archive", Format: volume.BackupFormatRustic, LocalSnapshotID: strings.Repeat("a", 64), Size: tc.size}
			f.service.deps.Store.Run = func(context.Context, string) (*volume.Backup, error) { return entry, nil }
			ctx := context.WithValue(f.targetContext("local"), edgeDownloadKey{}, tc.edge)
			reader, size, err := f.service.DownloadBackup(ctx, entry.ID, nil)
			if tc.refuseBefore {
				require.ErrorContains(t, err, "volume.backup.download-too-large")
				require.Empty(t, f.creates)
				require.Empty(t, f.scratch)
				return
			}
			require.NotEmpty(t, f.scratch[restore.RestoreLabel])
			require.Equal(t, "data", f.scratch[rustic.OperationVolumeLabel])
			require.Equal(t, "true", f.scratch[volumehelper.ContainerLabel])
			if tc.refuseAfter {
				require.ErrorContains(t, err, "volume.backup.download-too-large")
				require.Nil(t, reader)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.packaged, size)
				unlock, ok := f.service.deps.Locks.TryLock("data")
				if ok {
					unlock()
				}
				require.False(t, ok, "streaming still owns the volume")
				_, next, err := f.service.acquireOperation(ctx, "data", "")
				if next != nil {
					next.Release(ctx)
				}
				require.ErrorContains(t, err, "volume.backup.busy")
				require.NoError(t, reader.Close())
			}
			unlock, ok := f.service.deps.Locks.TryLock("data")
			require.True(t, ok)
			unlock()
			require.Contains(t, f.removed, "scratch")
			require.Contains(t, f.steps, "tar -czf /scratch/download.tar.gz -C /scratch/data .")
			if tc.edge {
				require.InDelta(t, (3 * time.Minute).Seconds(), f.execBudget.Seconds(), 3)
			} else {
				require.InDelta(t, (4 * time.Minute).Seconds(), f.execBudget.Seconds(), 3)
			}
		})
	}
}
func TestEdgeAgentDownloadHandlerAppliesHostTransportCap(t *testing.T) {
	f := newRuntimePlan(t)
	f.service.deps.Store.Run = func(context.Context, string) (*volume.Backup, error) {
		return &volume.Backup{ID: "row", Format: volume.BackupFormatRustic, LocalSnapshotID: strings.Repeat("a", 64), Size: dto.MaxEdgeDownloadBytes + 1}, nil
	}
	h := NewHandler(f.service, nil, nil, nil, t.Context(), &config.Config{EdgeAgent: true})
	_, err := h.DownloadBackup(t.Context(), &DownloadBackupInput{BackupID: "row"})
	require.ErrorContains(t, err, "volume.backup.download-too-large")
	require.Empty(t, f.creates)
}

func TestBoundedSinglePathAndNonrecursiveBrowseUseRealEngineCommands(t *testing.T) {
	f := newRuntimePlan(t)
	ctx := f.targetContext("local")
	cli, err := f.service.deps.Docker.GetClient(ctx)
	require.NoError(t, err)
	repository, err := f.service.repositoryFor(ctx, cli, "archive", true)
	require.NoError(t, err)
	snapshot := strings.Repeat("a", 64)
	f.stdout = "[]"
	exists, err := f.service.deps.Engine.SnapshotHasPath(ctx, cli, repository, "synthetic", snapshot, "empty-folder")
	require.NoError(t, err)
	require.True(t, exists, "a successful empty listing is an existing empty directory")
	require.Equal(t, []string{"ls", "--json", "--", snapshot + ":/empty-folder"}, f.creates[0].Cmd)
	f.exit = 1
	f.stdout = ""
	f.stderr = "Node `absent.txt` not found in tree."
	exists, err = f.service.deps.Engine.SnapshotHasPath(ctx, cli, repository, "synthetic", snapshot, "absent.txt")
	require.NoError(t, err)
	require.False(t, exists)
	f.exit = 0
	f.stderr = ""
	f.stdout = "[]"
	b := browser.NewService(browser.Dependencies{Docker: f.service.deps.Docker, Engine: f.service.deps.Engine,
		Run: func(context.Context, string) (*volume.Backup, error) {
			return &volume.Backup{Format: volume.BackupFormatRustic, LocalSnapshotID: snapshot}, nil
		},
		Repository: func(context.Context, *client.Client, *volume.Backup) (backup.Repository, string, error) {
			return repository, snapshot, nil
		},
		Password: func(context.Context, *client.Client, ...backup.Repository) (string, error) { return "synthetic", nil }})
	_, _, err = b.BrowseBackupFiles(ctx, "row", "folder", pagination.QueryParams{Search: "nested", Limit: 20})
	require.NoError(t, err)
	require.Equal(t, []string{"ls", "--json", "--", snapshot + ":/folder/"}, f.creates[len(f.creates)-1].Cmd)
	require.LessOrEqual(t, f.execBudget, time.Minute)
}
func TestExistingRepositoryWrongKeyProducesTypedMismatchWithoutInit(t *testing.T) {
	f := newRuntimePlan(t)
	f.exit = 1
	f.stdout = ""
	f.stderr = "No suitable key found for the given password."
	ctx := f.targetContext("local")
	cli, err := f.service.deps.Docker.GetClient(ctx)
	require.NoError(t, err)
	repository, err := f.service.repositoryFor(ctx, cli, "archive", true)
	require.NoError(t, err)
	_, err = f.service.deps.Engine.CheckRepository(ctx, cli, repository, "different-synthetic-key", false)
	require.ErrorContains(t, err, "backup.target.key-mismatch")
	require.Len(t, f.creates, 1)
	require.Equal(t, []string{"snapshots", "--json"}, f.creates[0].Cmd)
}

func TestForgetThenDatabaseFailureRetriesMissingSnapshotOnOriginalTarget(t *testing.T) {
	f := newRuntimePlan(t)
	f.forgetEmpties = true
	require.NoError(t, f.service.deps.DB.AutoMigrate(&event.Event{}))
	f.service.deps.Events = event.NewEventService(f.service.deps.DB, nil, nil)
	snapshot := strings.Repeat("a", 64)
	f.stdout = `[{"id":"` + snapshot + `"}]`
	entry := &volume.Backup{ID: "row", VolumeName: "data", Format: volume.BackupFormatRustic, Destination: "archive", LocalSnapshotID: snapshot, Status: "succeeded", CreatedAt: time.Now().UTC()}
	require.NoError(t, f.service.deps.DB.Table("volume_backups").Create(entry).Error)
	f.service.deps.Store.Run = func(ctx context.Context, id string) (*volume.Backup, error) {
		var row volume.Backup
		err := f.service.deps.DB.WithContext(ctx).Table("volume_backups").First(&row, "id = ?", id).Error
		return &row, err
	}
	failed := false
	f.service.deps.Store.Delete = func(ctx context.Context, id string) error {
		if !failed {
			failed = true
			return errors.New("synthetic database deletion failure")
		}
		return f.service.deps.DB.WithContext(ctx).Table("volume_backups").Where("id = ?", id).Delete(&volume.Backup{}).Error
	}
	ctx := f.targetContext("local")
	require.ErrorContains(t, f.service.DeleteBackup(ctx, entry.ID, nil), "database deletion failure")
	persisted, err := f.service.deps.Store.Run(ctx, entry.ID)
	require.NoError(t, err)
	require.Equal(t, snapshot, persisted.LocalSnapshotID)
	require.NoError(t, f.service.DeleteBackup(ctx, entry.ID, nil))
	var rows int64
	require.NoError(t, f.service.deps.DB.Table("volume_backups").Count(&rows).Error)
	require.Zero(t, rows)
	forgets, prunes := 0, 0
	for _, created := range f.creates {
		if created.Cmd[0] == "forget" {
			forgets++
			require.Contains(t, created.Cmd, snapshot)
		}
		if created.Cmd[0] == "prune" {
			prunes++
		}
		require.Contains(t, created.Env, "RUSTIC_REPOSITORY=/target/docklet-backups/0/volumes")
		require.Equal(t, "/srv/backups", created.HostConfig.Mounts[0].Source)
	}
	require.Equal(t, 1, forgets)
	require.Equal(t, 1, prunes)
}
