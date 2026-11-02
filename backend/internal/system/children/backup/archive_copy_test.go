// Source: docklet-next/issues/286
// Revision: b9b59ae92f1b09eb828940860971002eab9267b6
// Retrieved: 2026-11-01
// License: Apache-2.0
// Deviations: Original M16 bind identity and admission rules; M16 bind mounts and backup completion.

package backup

import (
	"archive/tar"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/florianfinn/docklet-next/backend/internal/backuptarget"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	dockerservice "github.com/florianfinn/docklet-next/backend/internal/docker"
	"github.com/florianfinn/docklet-next/backend/internal/testsqlite"
	dto "github.com/florianfinn/docklet-next/types/backup"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/require"
	"go.getarcane.app/sys/crypto"
	"gorm.io/gorm"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

type copySetting struct {
	Key   string `gorm:"primaryKey"`
	Value string
}

func (copySetting) TableName() string { return "settings" }

type copyRunRow struct {
	ID                       string `gorm:"primaryKey"`
	CopyState, CopyErrorCode string
}

func (copyRunRow) TableName() string { return "system_backup_runs" }

type copyTransport func(*http.Request) (*http.Response, error)

func (f copyTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type copyFixture struct {
	service     *Service
	run         *dto.SystemBackupRun
	root        string
	mu          sync.Mutex
	commands    map[string][]string
	outputs     map[string]string
	codes       map[string]int
	members     []string
	fail        string
	copyEntered chan context.Context
	blockCopy   bool
}

func newCopyFixture(t *testing.T) *copyFixture {
	t.Helper()
	f := &copyFixture{root: t.TempDir(), commands: map[string][]string{}, outputs: map[string]string{}, codes: map[string]int{}}
	db, err := gorm.Open(testsqlite.Dialector(t, ":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&backuptarget.Target{}, &copySetting{}, &copyRunRow{}))
	pool, err := db.DB()
	require.NoError(t, err)
	pool.SetMaxOpenConns(1)
	t.Cleanup(func() { require.NoError(t, pool.Close()) })
	wrapped := &database.DB{DB: db}
	crypto.InitEncryption(&crypto.Config{Environment: "test", EncryptionKey: "synthetic-encryption-key-at-least-32-bytes"})
	target, err := backuptarget.NewService(backuptarget.Dependencies{Store: &backuptarget.Store{DB: wrapped}}).Create(t.Context(), dto.TargetInput{TargetLocation: dto.TargetLocation{Kind: "local", HostID: new("0"), Path: "/srv/archive"}, Name: "Archive"})
	require.NoError(t, err)
	dir := t.TempDir()
	f.run = &dto.SystemBackupRun{ID: "saved", ArchiveName: "saved.tar", Size: 7, Status: dto.SystemBackupStatusSucceeded, CopyTargetID: target.ID, CopyState: dto.SystemBackupCopyNone}
	require.NoError(t, os.WriteFile(filepath.Join(dir, "saved.tar"), []byte("archive"), 0600))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { f.serve(t, w, r) }))
	t.Cleanup(server.Close)
	cli, err := client.New(client.WithHost("tcp://"+server.Listener.Addr().String()), client.WithAPIVersion("1.41"), client.WithHTTPClient(&http.Client{Transport: copyTransport(func(r *http.Request) (*http.Response, error) {
		if strings.HasSuffix(r.URL.Path, "/archive") && r.Method == "PUT" && f.copyEntered != nil {
			f.copyEntered <- r.Context()
			if f.blockCopy {
				<-r.Context().Done()
				return nil, r.Context().Err()
			}
		}
		return server.Client().Transport.RoundTrip(r)
	})}))
	require.NoError(t, err)
	t.Cleanup(func() { _ = cli.Close() })
	f.service = &Service{db: wrapped, docker: dockerservice.NewDockerClientService(t.Context(), wrapped, nil, nil).WithClient(cli), config: &config.Config{BackupDir: dir}, maintenance: NewMaintenanceGate(), store: Store{Run: func(context.Context, string) (*dto.SystemBackupRun, error) { return f.run, nil }, SaveRun: func(_ context.Context, r *dto.SystemBackupRun) error {
		return db.Save(&copyRunRow{ID: r.ID, CopyState: string(r.CopyState), CopyErrorCode: r.CopyErrorCode}).Error
	}}}
	return f
}
func (f *copyFixture) serve(t *testing.T, w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	p := r.URL.Path
	switch {
	case strings.HasSuffix(p, "/volumes/create"):
		http.Error(w, `{"message":"test-only"}`, 500)
	case strings.Contains(p, "/images/"):
		_, _ = w.Write([]byte(`{"Id":"tools"}`))
	case strings.HasSuffix(p, "/containers/create"):
		if f.fail == "mount" {
			http.Error(w, `{"message":"mount failed"}`, 500)
			return
		}
		_, _ = w.Write([]byte(`{"Id":"helper"}`))
	case strings.HasSuffix(p, "/start") && strings.Contains(p, "/containers/"):
		w.WriteHeader(204)
	case r.Method == "DELETE":
		w.WriteHeader(204)
	case strings.HasSuffix(p, "/containers/helper/archive"):
		if f.fail == "space" {
			http.Error(w, `{"message":"no space left on device"}`, 500)
			return
		}
		if f.fail == "write" {
			http.Error(w, `{"message":"synthetic write failure"}`, 500)
			return
		}
		reader := tar.NewReader(r.Body)
		for {
			header, err := reader.Next()
			if errors.Is(err, io.EOF) {
				break
			}
			require.NoError(t, err)
			f.mu.Lock()
			f.members = append(f.members, header.Name)
			f.mu.Unlock()
			dir := strings.ReplaceAll(r.URL.Query().Get("path"), "/target", f.root)
			require.NoError(t, os.MkdirAll(dir, 0700))
			file, err := os.Create(filepath.Join(dir, header.Name))
			require.NoError(t, err)
			_, err = io.Copy(file, reader)
			require.NoError(t, err)
			require.NoError(t, file.Close())
		}
		w.WriteHeader(200)
	case strings.HasSuffix(p, "/containers/helper/exec"):
		var body struct{ Cmd []string }
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		f.mu.Lock()
		id := fmt.Sprintf("exec-%d", len(f.commands))
		f.commands[id] = body.Cmd
		f.mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]string{"Id": id})
	case strings.Contains(p, "/exec/") && strings.HasSuffix(p, "/start"):
		id := strings.Split(p, "/exec/")[1]
		id = strings.TrimSuffix(id, "/start")
		f.mu.Lock()
		args := append([]string(nil), f.commands[id]...)
		f.mu.Unlock()
		for i := range args {
			args[i] = strings.ReplaceAll(args[i], "/target", f.root)
		}
		command := exec.CommandContext(r.Context(), args[0], args[1:]...)
		out, err := command.CombinedOutput()
		code := 0
		if err != nil {
			code = 1
		}
		f.mu.Lock()
		f.outputs[id] = string(out)
		f.codes[id] = code
		f.mu.Unlock()
		conn, buffer, err := w.(http.Hijacker).Hijack()
		require.NoError(t, err)
		defer conn.Close()
		_, err = fmt.Fprint(buffer, "HTTP/1.1 101 UPGRADED\r\nContent-Type: application/vnd.docker.raw-stream\r\nConnection: Upgrade\r\nUpgrade: tcp\r\n\r\n")
		require.NoError(t, err)
		if len(out) > 0 {
			frame := make([]byte, 8)
			frame[0] = 1
			binary.BigEndian.PutUint32(frame[4:], uint32(len(out)))
			_, _ = buffer.Write(frame)
			_, _ = buffer.Write(out)
		}
		require.NoError(t, buffer.Flush())
	case strings.Contains(p, "/exec/") && strings.HasSuffix(p, "/json"):
		id := strings.Split(p, "/exec/")[1]
		id = strings.TrimSuffix(id, "/json")
		f.mu.Lock()
		code := f.codes[id]
		f.mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]any{"Running": false, "ExitCode": code})
	default:
		t.Errorf("unexpected Docker request %s %s", r.Method, p)
		w.WriteHeader(500)
	}
}
func TestHubArchiveCopyAtomicityAndFailures(t *testing.T) {
	for _, fail := range []string{"", "mount", "write", "space", "pending-save", "terminal-save"} {
		t.Run(fail, func(t *testing.T) {
			f := newCopyFixture(t)
			f.fail = fail
			before, err := os.ReadFile(filepath.Join(f.service.backupDir(), f.run.ArchiveName))
			require.NoError(t, err)
			hash := sha256.Sum256(before)
			if fail == "terminal-save" || fail == "pending-save" {
				save := f.service.store.SaveRun
				f.service.store.SaveRun = func(ctx context.Context, r *dto.SystemBackupRun) error {
					if r.CopyState == dto.SystemBackupCopySucceeded && fail == "terminal-save" || r.CopyState == dto.SystemBackupCopyPending && fail == "pending-save" {
						return errors.New("save failed")
					}
					return save(ctx, r)
				}
			}
			err = f.service.copyArchive(t.Context(), f.run)
			if fail == "" {
				require.NoError(t, err)
				require.Equal(t, dto.SystemBackupCopySucceeded, f.run.CopyState)
				copy, err := os.ReadFile(filepath.Join(f.root, "docklet-backups/hub/saved.tar"))
				require.NoError(t, err)
				require.Equal(t, before, copy)
				require.Equal(t, []string{"saved.tar.partial"}, f.members)
			} else {
				require.Error(t, err)
				require.Equal(t, dto.SystemBackupCopyFailed, f.run.CopyState)
				expected := map[string]string{"mount": "backup.copy.mount-failed", "write": "backup.copy.write-failed", "space": "backup.copy.insufficient-space", "pending-save": "backup.copy.write-failed", "terminal-save": "backup.copy.write-failed"}
				require.Equal(t, expected[fail], f.run.CopyErrorCode)
			}
			after, err := os.ReadFile(filepath.Join(f.service.backupDir(), f.run.ArchiveName))
			require.NoError(t, err)
			require.Equal(t, hash, sha256.Sum256(after))
			require.Equal(t, dto.SystemBackupStatusSucceeded, f.run.Status)
			_, err = os.Stat(filepath.Join(f.root, "docklet-backups/hub/saved.tar.partial"))
			require.ErrorIs(t, err, os.ErrNotExist)
		})
	}
}
func TestHubArchiveCopyMaintenanceDrain(t *testing.T) {
	f := newCopyFixture(t)
	f.copyEntered = make(chan context.Context, 1)
	f.blockCopy = true
	terminal, settle := make(chan struct{}), make(chan struct{})
	save := f.service.store.SaveRun
	f.service.store.SaveRun = func(ctx context.Context, r *dto.SystemBackupRun) error {
		if r.CopyState == dto.SystemBackupCopyFailed {
			close(terminal)
			<-settle
		}
		return save(ctx, r)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- f.service.copyArchive(ctx, f.run) }()
	request := <-f.copyEntered
	require.NoError(t, f.service.maintenance.Begin("next", "backup"))
	require.ErrorIs(t, request.Err(), context.Canceled, "maintenance synchronously cancels the registered copy")
	<-terminal
	drained := make(chan error, 1)
	go func() { drained <- f.service.maintenance.Wait(t.Context()) }()
	select {
	case <-drained:
		t.Fatal("drain returned before terminal copy persistence")
	default:
	}
	close(settle)
	require.EqualError(t, <-done, "runtime.interrupted")
	require.NoError(t, <-drained)
	require.Equal(t, "runtime.interrupted", f.run.CopyErrorCode)
}
func TestHubPendingCopyStartupSettlement(t *testing.T) {
	f := newCopyFixture(t)
	require.NoError(t, f.service.db.Create(&copyRunRow{ID: "old", CopyState: "pending"}).Error)
	require.ErrorIs(t, f.service.ReconcileStorage(t.Context()), ErrSystemBackupUnavailable, "in-memory fixture has no persistent archive path; pending settlement still precedes storage readiness")
	var row copyRunRow
	require.NoError(t, f.service.db.First(&row, "id = ?", "old").Error)
	require.Equal(t, "failed", row.CopyState)
	require.Equal(t, "runtime.interrupted", row.CopyErrorCode)
}
func TestHubCopyTargetAdmissionAndReferenceFence(t *testing.T) {
	f := newCopyFixture(t)
	require.NoError(t, f.service.validateCopyTarget(t.Context(), f.run.CopyTargetID))
	require.NoError(t, f.service.db.Model(&backuptarget.Target{}).Where("id = ?", f.run.CopyTargetID).Update("host_id", "host-a").Error)
	require.ErrorIs(t, f.service.validateCopyTarget(t.Context(), f.run.CopyTargetID), ErrSystemBackupTargetUnavailable)
}

func TestHubCopyCredentialRedaction(t *testing.T) {
	f := newCopyFixture(t)
	secret := "test-only"
	encrypted, err := crypto.Encrypt(secret)
	require.NoError(t, err)
	require.NoError(t, f.service.db.Model(&backuptarget.Target{}).Where("id = ?", f.run.CopyTargetID).Updates(map[string]any{"kind": "smb", "host_id": nil, "path": "", "server": "nas.example.com", "share": "archive", "secret_enc": encrypted}).Error)
	err = f.service.copyArchive(t.Context(), f.run)
	require.Error(t, err)
	require.NotContains(t, err.Error(), secret)
	require.EqualError(t, err, "backup.copy.mount-failed")
	encoded, err := json.Marshal(f.run)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), secret)
	require.NotContains(t, f.run.CopyErrorCode, secret)
}

func TestHubCopyRetentionRemovesOnlyRecordedFinal(t *testing.T) {
	f := newCopyFixture(t)
	require.NoError(t, f.service.copyArchive(t.Context(), f.run))
	dir := filepath.Join(f.root, "docklet-backups/hub")
	for _, name := range []string{"foreign.tar", "saved.tar.partial"} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte("kept"), 0600))
	}
	require.NoError(t, f.service.removeArchiveCopy(t.Context(), f.run))
	_, err := os.Stat(filepath.Join(dir, "saved.tar"))
	require.ErrorIs(t, err, os.ErrNotExist)
	for _, name := range []string{"foreign.tar", "saved.tar.partial"} {
		_, err := os.Stat(filepath.Join(dir, name))
		require.NoError(t, err)
	}
}
