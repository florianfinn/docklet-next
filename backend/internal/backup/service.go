// Source: getarcaneapp/arcane/backend/internal/backup/service.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; removed S3 destinations; M12 scoped output, observed availability and strict volume parsing.

// Package backup owns the shared Rustic backup engine used by volume and
// system backups: typed repository operations, per-repository serialization
// and durable run admission.
package backup

import (
	"cmp"
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/image"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/volumehelper"
	"github.com/florianfinn/docklet-next/backend/pkg/scheduler/runs"
	"github.com/florianfinn/docklet-next/types/backup"
	"github.com/florianfinn/docklet-next/types/scheduler"
	"github.com/italypaleale/francis/actor"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/client"
	kit "go.getarcane.app/kit/pkg"
	"path"
	"slices"
	"strings"
	"sync"
	"time"
	"uuid"
)

const (
	VolumeAdmissionScope = "volume-backup"
	SystemAdmissionScope = "system-backup"
	RecoveryKeyConfigID  = "system-recovery"
)

type Repository struct {
	ObserveSnapshot  func(context.Context, string, bool) error
	ObserveInventory func(context.Context, []DiscoveredSnapshot) error
	Password         string
	BeforeRun        func(context.Context) error
	ID               string
	Environment      []string
	Mounts           []mount.Mount
}

type Snapshot struct {
	ID   string `json:"id"`
	Size int64  `json:"-"`
}

type rusticSnapshotOutputInternal struct {
	ID      string `json:"id"`
	Summary struct {
		TotalBytesProcessed int64 `json:"total_bytes_processed"`
	} `json:"summary"`
}

// RestoreOptions selects what a snapshot restore writes and where.
type RestoreOptions struct {
	// DeleteExtra removes files in the target that are absent from the snapshot.
	DeleteExtra bool
	// SourcePath restores only this path inside the snapshot when non-empty.
	SourcePath string
	// DestinationPath overrides the restore target path inside the helper
	// container; the target mount's path is used when empty.
	DestinationPath string
	// ExtraMounts are attached alongside the target, e.g. mounts nested inside it.
	ExtraMounts []mount.Mount
}

// CreateSnapshotInput describes what one snapshot captures: the mounts to
// attach to the helper and the helper paths to back up. AsPath rewrites the
// snapshot path of a single source and cannot combine with several.
type CreateSnapshotInput struct {
	Mounts  []mount.Mount
	Sources []string
	AsPath  string
	Tags    []string
	Globs   []string
}

// RootSnapshotInput captures one mount with its contents at the snapshot root.
func RootSnapshotInput(source mount.Mount, tags ...string) CreateSnapshotInput {
	return CreateSnapshotInput{Mounts: []mount.Mount{source}, Sources: []string{source.Target}, AsPath: "/", Tags: tags}
}

func snapshotCommandInternal(label string, input CreateSnapshotInput) ([]string, error) {
	if len(input.Sources) == 0 {
		return nil, errors.New("at least one snapshot source is required")
	}
	if input.AsPath != "" && len(input.Sources) > 1 {
		return nil, errors.New("a snapshot path rewrite requires a single source")
	}
	command := []string{"backup", "--init", "--json", "--host", "arcane", "--label", label}
	for _, tag := range input.Tags {
		command = append(command, "--tag", tag)
	}
	for _, glob := range input.Globs {
		command = append(command, "--glob", glob)
	}
	if input.AsPath != "" {
		command = append(command, "--as-path", input.AsPath)
	}
	command = append(command, "--")
	return append(command, input.Sources...), nil
}

// Engine owns repository serialization and application-owned backup workers.
type Engine struct {
	imageReference func() string
	imageService   *image.ImageService
	admission      *runs.Admission
	lifecycleCtx   context.Context
	cancel         context.CancelFunc
	workers        sync.WaitGroup
	stopping       bool
	mu             sync.Mutex
	repositories   map[string]*sync.Mutex
	service        *actor.Service
	handlers       map[string]func(context.Context, string, []byte, bool) error
	failures       map[string]func(context.Context, string, []byte, error) error
	leases         map[string]*runs.Lease
	unresolved     map[string]backup.DurableRunCommand
	executionReady func() bool
	authorize      func(context.Context, backup.DurableRunCommand) error
}

func NewEngine(ctx context.Context, admission *runs.Admission, imageService *image.ImageService) *Engine {
	runCtx, cancel := context.WithCancel(ctx)
	return &Engine{
		cancel:       cancel,
		imageService: imageService,
		admission:    admission,
		lifecycleCtx: runCtx,
		repositories: make(
			map[string]*sync.Mutex,
		),
		handlers: make(
			map[string]func(
				context.Context,
				string,
				[]byte,
				bool,
			) error,
		),
		failures: make(
			map[string]func(
				context.Context,
				string,
				[]byte,
				error,
			) error,
		),
		leases: make(
			map[string]*runs.Lease,
		),
		unresolved: make(map[string]backup.DurableRunCommand),
	}
}

func (e *Engine) TryAcquireRun(ctx context.Context, scope, id string) (*runs.Lease, bool, error) {
	if e == nil || e.admission == nil {
		return nil, false, errors.New("backup engine is unavailable")
	}
	return e.admission.TryAcquire(ctx, scheduler.AdmissionKey{Scope: scope, ID: id})
}

func (e *Engine) Stop(ctx context.Context) error {
	if e == nil {
		return nil
	}
	e.mu.Lock()
	e.stopping = true
	e.cancel()
	e.mu.Unlock()
	done := make(chan struct{})
	go func() {
		e.workers.Wait()
		e.mu.Lock()
		leases := e.leases
		e.leases = make(map[string]*runs.Lease)
		e.mu.Unlock()
		for _, lease := range leases {
			lease.Release(ctx)
		}
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// CreateSnapshot backs the input's sources up into the repository as one
// snapshot. The repository is initialized on first use.
func (e *Engine) CreateSnapshot(ctx context.Context, dockerClient *client.Client, repository Repository, password, label string, input CreateSnapshotInput) (Snapshot, error) {
	command, err := snapshotCommandInternal(label, input)
	if err != nil {
		return Snapshot{}, err
	}
	output, err := e.runInternal(ctx, dockerClient, repository, password, command, input.Mounts...)
	if err != nil {
		return Snapshot{}, err
	}
	if isVolumeRepository(repository) {
		return decodeVolumeSnapshot(output)
	}
	var decoded rusticSnapshotOutputInternal
	if unmarshalErr := json.Unmarshal([]byte(output), &decoded); unmarshalErr != nil {
		return Snapshot{}, fmt.Errorf("failed to decode Rustic snapshot: %w", unmarshalErr)
	}
	if decoded.ID == "" {
		return Snapshot{}, errors.New("rustic did not return a snapshot ID")
	}
	return Snapshot{ID: decoded.ID, Size: decoded.Summary.TotalBytesProcessed}, nil
}

// RestoreSnapshot restores a snapshot (or one path inside it) onto the target mount.
func (e *Engine) RestoreSnapshot(ctx context.Context, dockerClient *client.Client, repository Repository, password, snapshotID string, target mount.Mount, options RestoreOptions) error {
	command := []string{"restore", "--verify-existing"}
	if options.DeleteExtra {
		command = append(command, "--delete")
	}
	source := snapshotID + ":/"
	if options.SourcePath != "" {
		source = snapshotID + ":/" + strings.TrimPrefix(options.SourcePath, "/")
	}
	destination := cmp.Or(options.DestinationPath, target.Target)
	command = append(command, "--", source, destination)
	mounts := append([]mount.Mount{target}, options.ExtraMounts...)
	_, err := e.runInternal(ctx, dockerClient, repository, password, command, mounts...)
	return observeSnapshotResult(ctx, repository, snapshotID, err)
}

// ListSnapshotFiles lists a snapshot path and returns paths relative to the
// snapshot root. A supplied path is nonrecursive unless recursive is true.
func (e *Engine) ListSnapshotFiles(ctx context.Context, dockerClient *client.Client, repository Repository, password, snapshotID, filePath string, recursive bool) ([]string, error) {
	if isVolumeRepository(repository) {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 60*time.Second)
		defer cancel()
	}
	command := []string{"ls", "--json"}
	if recursive {
		command = append(command, "--recursive")
	}
	cleanedPath := strings.Trim(strings.TrimSpace(filePath), "/")
	source := snapshotID + ":/" + cleanedPath
	if cleanedPath != "" && strings.HasSuffix(strings.TrimSpace(filePath), "/") {
		source += "/"
	}
	command = append(command, "--", source)
	output, err := e.runInternal(ctx, dockerClient, repository, password, command)
	err = observeSnapshotResult(ctx, repository, snapshotID, err)
	if err != nil {
		return nil, fmt.Errorf("failed to list Rustic snapshot: %w", err)
	}
	var files []string
	if isVolumeRepository(repository) {
		files, err = decodeVolumeFiles(output, recursive)
		if err != nil {
			return nil, err
		}
	} else if unmarshalErr := json.Unmarshal([]byte(output), &files); unmarshalErr != nil {
		return nil, fmt.Errorf("failed to decode Rustic file list: %w", unmarshalErr)
	}
	if !recursive && len(files) > 0 {
		// Rustic's JSON listing omits node types; the stable long listing restores them.
		longCommand := slices.Clone(command)
		longCommand[1] = "--long"
		longOutput, runErr := e.runInternal(ctx, dockerClient, repository, password, longCommand)
		runErr = observeSnapshotResult(ctx, repository, snapshotID, runErr)
		if runErr != nil {
			return nil, fmt.Errorf("failed to list Rustic snapshot metadata: %w", runErr)
		}
		files, runErr = markSnapshotDirectoriesInternal(files, longOutput)
		if runErr != nil {
			if isVolumeRepository(repository) {
				return nil, backup.NewProblem(502, "volume.backup.engine-output-invalid", runErr)
			}
			return nil, runErr
		}
	}
	return qualifySnapshotListingInternal(files, cleanedPath), nil
}

func qualifySnapshotListingInternal(files []string, snapshotPath string) []string {
	qualified := slices.Clone(files)
	prefix := strings.Trim(strings.TrimSpace(snapshotPath), "/")
	if prefix == "" {
		return qualified
	}
	for index, file := range qualified {
		directory := strings.HasSuffix(file, "/")
		relative := strings.TrimPrefix(strings.TrimPrefix(file, "./"), "/")
		qualified[index] = path.Join(prefix, relative)
		if directory {
			qualified[index] += "/"
		}
	}
	return qualified
}

func markSnapshotDirectoriesInternal(files []string, longOutput string) ([]string, error) {
	if len(files) == 0 {
		if strings.TrimSpace(longOutput) != "" {
			return nil, errors.New("rustic file and metadata listings have different lengths")
		}
		return []string{}, nil
	}
	lines := strings.Split(strings.ReplaceAll(longOutput, "\r\n", "\n"), "\n")
	if len(lines) != len(files) {
		return nil, errors.New("rustic file and metadata listings have different lengths")
	}
	marked := slices.Clone(files)
	for index, line := range lines {
		if line == "" {
			return nil, errors.New("rustic returned empty file metadata")
		}
		if line[0] == 'd' && !strings.HasSuffix(marked[index], "/") {
			marked[index] += "/"
		}
	}
	return marked, nil
}

// ReadSnapshotTextFile returns one text file from a snapshot.
func (e *Engine) ReadSnapshotTextFile(ctx context.Context, dockerClient *client.Client, repository Repository, password, snapshotID, filePath string) (string, error) {
	output, err := e.runInternal(ctx, dockerClient, repository, password, []string{
		"dump", "--archive", "content", "--", snapshotID + ":/" + strings.TrimPrefix(filePath, "/"),
	})
	if err != nil {
		return "", fmt.Errorf("failed to read Rustic snapshot file: %w", err)
	}
	return output, nil
}

// DiscoveredSnapshot describes one snapshot found in a repository.
type DiscoveredSnapshot struct {
	ID      string    `json:"id"`
	Time    time.Time `json:"time"`
	Label   string    `json:"label"`
	Tags    []string  `json:"tags"`
	Summary struct {
		TotalBytesProcessed int64 `json:"total_bytes_processed"`
	} `json:"summary"`
}

// RunSnapshotTag links committed snapshots to their durable backup record.
func RunSnapshotTag(runID string) string { return "arcane-run:" + runID }

// ConfirmRunSnapshot makes a validated snapshot discoverable by run ID.
func (e *Engine) ConfirmRunSnapshot(ctx context.Context, dockerClient *client.Client, repository Repository, password, runID, snapshotID string) (Snapshot, error) {
	if !fullSnapshotIDInternal(snapshotID) {
		return Snapshot{}, errors.New("a full snapshot ID is required")
	}
	command := []string{"tag", "--add", RunSnapshotTag(runID), "--", snapshotID}
	if _, err := e.runInternal(ctx, dockerClient, repository, password, command); err != nil {
		return Snapshot{}, err
	}
	// Tagging rewrites the snapshot and changes its ID.
	snapshot, found, err := e.FindRunSnapshot(ctx, dockerClient, repository, password, runID, "")
	if err != nil {
		return Snapshot{}, err
	}
	if !found {
		return Snapshot{}, errors.New("confirmed backup snapshot is unavailable")
	}
	return snapshot, nil
}

// FindRunSnapshot verifies a known snapshot or recovers its committed response by run tag.
func (e *Engine) FindRunSnapshot(ctx context.Context, dockerClient *client.Client, repository Repository, password, runID, snapshotID string) (Snapshot, bool, error) {
	snapshots, err := e.ListSnapshots(ctx, dockerClient, repository, password)
	if err != nil {
		return Snapshot{}, false, err
	}
	var found Snapshot
	for _, snapshot := range snapshots {
		match := snapshotID != "" && snapshot.ID == snapshotID
		if snapshotID == "" {
			match = slices.Contains(snapshot.Tags, RunSnapshotTag(runID))
		}
		if !match {
			continue
		}
		if found.ID != "" {
			return Snapshot{}, false, errors.New("multiple snapshots match the backup run")
		}
		found = Snapshot{ID: snapshot.ID, Size: snapshot.Summary.TotalBytesProcessed}
	}
	if snapshotID != "" && found.ID == "" {
		return Snapshot{}, false, errors.New("recorded backup snapshot is unavailable")
	}
	return found, found.ID != "", nil
}

// ListSnapshots enumerates every snapshot in the repository.
func (e *Engine) ListSnapshots(ctx context.Context, dockerClient *client.Client, repository Repository, password string) ([]DiscoveredSnapshot, error) {
	if isVolumeRepository(repository) {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 60*time.Second)
		defer cancel()
	}
	output, err := e.runInternal(ctx, dockerClient, repository, password, []string{"snapshots", "--json"})
	if err != nil {
		return nil, err
	}
	snapshots, err := decodeSnapshotsInternal(output)
	if isVolumeRepository(repository) && err != nil {
		return nil, backup.NewProblem(502, "volume.backup.engine-output-invalid", err)
	}
	if err == nil && repository.ObserveInventory != nil {
		err = repository.ObserveInventory(ctx, snapshots)
	}
	return snapshots, err
}

// ForgetSnapshots removes the snapshots from the repository and prunes their
// data in a single pass.
func (e *Engine) ForgetSnapshots(ctx context.Context, dockerClient *client.Client, repository Repository, password string, snapshotIDs []string) error {
	if len(snapshotIDs) == 0 {
		return errors.New("at least one snapshot ID is required")
	}
	requested := make([]string, 0, len(snapshotIDs))
	for _, id := range snapshotIDs {
		if !fullSnapshotIDInternal(id) {
			return errors.New("a full snapshot ID is required")
		}
		requested = append(requested, strings.ToLower(id))
	}
	requested = kit.Unique(requested)
	if e == nil {
		return errors.New("backup engine is unavailable")
	}
	if strings.TrimSpace(repository.ID) == "" {
		return errors.New("backup repository ID is required")
	}
	defer e.lockRepositoryInternal(repository.ID)()
	if err := ctx.Err(); err != nil {
		return err
	}
	output, err := e.runContainerInternal(ctx, dockerClient, repository, password, []string{"snapshots", "--json"})
	if err != nil {
		return err
	}
	snapshots, err := decodeSnapshotsInternal(output)
	if err != nil {
		return err
	}
	if repository.ObserveInventory != nil {
		if err := repository.ObserveInventory(ctx, snapshots); err != nil {
			return err
		}
	}
	existing := make(map[string]string, len(snapshots))
	for _, snapshot := range snapshots {
		existing[strings.ToLower(snapshot.ID)] = snapshot.ID
	}
	command := []string{"forget", "--prune"}
	for _, id := range requested {
		if listedID, found := existing[id]; found {
			if len(command) == 2 {
				command = append(command, "--")
			}
			command = append(command, listedID)
		}
	}
	if len(command) == 2 {
		command = []string{"prune"}
	}
	_, err = e.runContainerInternal(ctx, dockerClient, repository, password, command)
	return err
}

// ChangeRepositoryPassword re-keys the repository; the scratch Rustic image has no shell, so the new password travels as an argument.
func (e *Engine) ChangeRepositoryPassword(ctx context.Context, dockerClient *client.Client, repository Repository, currentPassword, newPassword string) error {
	if strings.TrimSpace(newPassword) == "" {
		return errors.New("new repository password is required")
	}
	_, err := e.runInternal(ctx, dockerClient, repository, currentPassword, []string{"key", "password", "--new-password", newPassword})
	return err
}

// Replicate copies one snapshot between repositories by restoring it into a
// temporary volume and backing that volume up into the target repository. The
// source is read once from the repository, never from the live data. Rustic's
// native `copy` would move only missing packs, but it addresses the target via
// a TOML config profile, which the env-only Repository cannot express yet —
// the materialize-and-rebackup here trades disk and I/O for that simplicity.
func (e *Engine) Replicate(ctx context.Context, dockerClient *client.Client, from Repository, fromSnapshotID string, to Repository, password, label string, tags ...string) (Snapshot, error) {
	temporaryVolume := "arcane-rustic-copy-" + uuid.New().String()
	if _, err := dockerClient.VolumeCreate(ctx, client.VolumeCreateOptions{Name: temporaryVolume, Labels: volumehelper.Labels()}); err != nil {
		return Snapshot{}, fmt.Errorf("failed to create temporary Rustic copy volume: %w", err)
	}
	defer func() {
		_, _ = dockerClient.VolumeRemove(context.WithoutCancel(ctx), temporaryVolume, client.VolumeRemoveOptions{Force: true})
	}()
	copyMount := mount.Mount{Type: mount.TypeVolume, Source: temporaryVolume, Target: "/volume"}
	if err := e.RestoreSnapshot(ctx, dockerClient, from, password, fromSnapshotID, copyMount, RestoreOptions{DeleteExtra: true}); err != nil {
		return Snapshot{}, fmt.Errorf("failed to load Rustic snapshot for replication: %w", err)
	}
	copyMount.ReadOnly = true
	snapshot, err := e.CreateSnapshot(ctx, dockerClient, to, password, label, RootSnapshotInput(copyMount, tags...))
	if err != nil {
		return Snapshot{}, fmt.Errorf("failed to replicate Rustic snapshot: %w", err)
	}
	return snapshot, nil
}

func (e *Engine) runInternal(ctx context.Context, dockerClient *client.Client, repository Repository, password string, command []string, extraMounts ...mount.Mount) (string, error) {
	if e == nil {
		return "", errors.New("backup engine is unavailable")
	}
	if strings.TrimSpace(repository.ID) == "" {
		return "", errors.New("backup repository ID is required")
	}
	defer e.lockRepositoryInternal(repository.ID)()
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return e.runContainerInternal(ctx, dockerClient, repository, password, command, extraMounts...)
}

func (e *Engine) lockRepositoryInternal(id string) func() {
	e.mu.Lock()
	lock := e.repositories[id]
	if lock == nil {
		lock = &sync.Mutex{}
		e.repositories[id] = lock
	}
	e.mu.Unlock()
	lock.Lock()
	return lock.Unlock
}

// ExpiredRunIDs returns the IDs of succeeded runs with snapshots that fall
// outside the newest keep entries for one policy, oldest last. Callers delete
// each run through their own delete path so snapshots are forgotten too.
func ExpiredRunIDs(ctx context.Context, db *database.DB, table, policyID string, keep int) ([]string, error) {
	var ids []string
	err := db.WithContext(ctx).
		Table(table).
		Where(
			"policy_id = ? AND status = ? AND COALESCE(local_snapshot_id, '') <> ''",
			policyID,
			"succeeded",
		).
		Order("created_at DESC").
		Offset(keep).
		Pluck("id", &ids).Error
	return ids, err
}
