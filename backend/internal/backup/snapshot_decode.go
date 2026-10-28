// Source: getarcaneapp/arcane/backend/internal/backup/service.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Extracted snapshot decoding; strict M12 volume parser and scoped output limits.

package backup

import (
	"context"
	"encoding/hex"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/rustic"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/client"
	"log/slog"
	"strings"
	"time"

	dto "github.com/florianfinn/docklet-next/types/backup"
)

func isVolumeRepository(repository Repository) bool {
	return strings.HasPrefix(repository.ID, "volumes:") || strings.HasPrefix(repository.ID, "target:")
}
func decodeVolumeSnapshot(output string) (Snapshot, error) {
	var snapshot struct {
		ID      string `json:"id"`
		Summary *struct {
			TotalBytesProcessed *int64 `json:"total_bytes_processed"`
		} `json:"summary"`
	}
	err := json.Unmarshal([]byte(output), &snapshot)
	if len(output) > dto.MaxVolumeResponseBytes || err != nil || !fullSnapshotIDInternal(snapshot.ID) {
		return Snapshot{}, dto.NewProblem(502, "volume.backup.engine-output-invalid", err)
	}
	size := int64(-1)
	if snapshot.Summary != nil && snapshot.Summary.TotalBytesProcessed != nil && *snapshot.Summary.TotalBytesProcessed >= 0 {
		size = *snapshot.Summary.TotalBytesProcessed
	}
	return Snapshot{ID: snapshot.ID, Size: size}, nil
}
func decodeVolumeFiles(output string, recursive bool) ([]string, error) {
	var files []string
	err := json.Unmarshal([]byte(output), &files)
	if len(output) > dto.MaxVolumeResponseBytes || err != nil || files == nil {
		return nil, dto.NewProblem(502, "volume.backup.engine-output-invalid", err)
	}
	if !recursive && len(files) > dto.MaxBrowseEntries {
		return nil, dto.NewProblem(422, "volume.backup.listing-too-large", nil)
	}
	for _, file := range files {
		if file == "" || strings.ContainsRune(file, 0) {
			return nil, dto.NewProblem(502, "volume.backup.engine-output-invalid", nil)
		}
	}
	return files, nil
}

func decodeSnapshotsInternal(output string) ([]DiscoveredSnapshot, error) {
	trimmedOutput := strings.TrimSpace(output)
	if trimmedOutput == "" || trimmedOutput[0] != '[' {
		return nil, errors.New("invalid Rustic snapshot listing: expected an array")
	}
	var entries []jsontext.Value
	if err := json.Unmarshal([]byte(output), &entries); err != nil {
		return nil, fmt.Errorf("failed to decode Rustic snapshots: %w", err)
	}

	snapshots := make([]DiscoveredSnapshot, 0, len(entries))
	grouped := false
	for index, entry := range entries {
		if entry.Kind() != '{' {
			return nil, fmt.Errorf("invalid Rustic snapshot listing entry %d: expected an object", index)
		}
		var fields map[string]jsontext.Value
		if err := json.Unmarshal(entry, &fields); err != nil {
			return nil, fmt.Errorf("failed to decode Rustic snapshot listing entry %d: %w", index, err)
		}
		_, hasID := fields["id"]
		groupEntries, hasSnapshots := fields["snapshots"]
		if hasID == hasSnapshots {
			return nil, fmt.Errorf("invalid Rustic snapshot listing entry %d: expected an ID or snapshot group", index)
		}
		if index > 0 && grouped != hasSnapshots {
			return nil, errors.New("invalid Rustic snapshot listing: mixed flat and grouped entries")
		}
		grouped = hasSnapshots
		if !hasSnapshots {
			snapshot, err := decodeSnapshotInternal(entry)
			if err != nil {
				return nil, fmt.Errorf("invalid Rustic snapshot listing entry %d: %w", index, err)
			}
			snapshots = append(snapshots, snapshot)
			continue
		}
		if groupEntries.Kind() != '[' {
			return nil, fmt.Errorf("invalid Rustic snapshot group %d: expected a snapshots array", index)
		}
		var members []jsontext.Value
		if err := json.Unmarshal(groupEntries, &members); err != nil {
			return nil, fmt.Errorf("failed to decode Rustic snapshot group %d: %w", index, err)
		}
		for memberIndex, member := range members {
			snapshot, err := decodeSnapshotInternal(member)
			if err != nil {
				return nil, fmt.Errorf("invalid Rustic snapshot group %d entry %d: %w", index, memberIndex, err)
			}
			snapshots = append(snapshots, snapshot)
		}
	}
	return snapshots, nil
}

func decodeSnapshotInternal(raw jsontext.Value) (DiscoveredSnapshot, error) {
	if raw.Kind() != '{' {
		return DiscoveredSnapshot{}, errors.New("expected a snapshot object")
	}
	var fields map[string]jsontext.Value
	if err := json.Unmarshal(raw, &fields); err != nil {
		return DiscoveredSnapshot{}, fmt.Errorf("failed to decode Rustic snapshot fields: %w", err)
	}
	if _, grouped := fields["snapshots"]; grouped {
		return DiscoveredSnapshot{}, errors.New("expected a flat snapshot object")
	}
	var snapshot DiscoveredSnapshot
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		return DiscoveredSnapshot{}, fmt.Errorf("failed to decode Rustic snapshot payload: %w", err)
	}
	if !fullSnapshotIDInternal(snapshot.ID) {
		return DiscoveredSnapshot{}, errors.New("invalid full snapshot ID")
	}
	return snapshot, nil
}

func fullSnapshotIDInternal(id string) bool {
	if len(id) != 64 {
		return false
	}
	_, err := hex.DecodeString(id)
	return err == nil
}

// SnapshotHasPath checks one exact path. A successful empty listing denotes an
// existing empty directory; only Rustic's missing-node result denotes absence.
func (e *Engine) SnapshotHasPath(ctx context.Context, cli *client.Client, repository Repository, password, snapshot, filePath string) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	output, err := e.runInternal(ctx, cli, repository, password, []string{"ls", "--json", "--", snapshot + ":/" + filePath})
	if err != nil {
		var commandError *rustic.CommandError
		if errors.As(err, &commandError) && strings.Contains(commandError.Diagnostic, "Node `") && strings.Contains(commandError.Diagnostic, "not found in tree.") {
			return false, observeSnapshotResult(ctx, repository, snapshot, nil)
		}
		return false, observeSnapshotResult(ctx, repository, snapshot, err)
	}
	if _, err := decodeVolumeFiles(output, false); err != nil {
		return false, err
	}
	return true, observeSnapshotResult(ctx, repository, snapshot, nil)
}

func (e *Engine) runContainerInternal(ctx context.Context, dockerClient *client.Client, repository Repository, password string, command []string, extraMounts ...mount.Mount) (string, error) {
	ctx = e.imageContext(ctx)
	volumeRepository := isVolumeRepository(repository)
	if volumeRepository {
		ctx = rustic.WithVolumeOutputLimit(ctx)
	}
	if err := e.ensureImageInternal(ctx, dockerClient); err != nil {
		if volumeRepository {
			return "", dto.NewProblem(503, "volume.backup.engine-unavailable", err)
		}
		return "", err
	}
	ctx = rustic.WithBeforeCreate(ctx, repository.BeforeRun)
	mounts := append([]mount.Mount{}, repository.Mounts...)
	mounts = append(mounts, extraMounts...)
	output, err := rustic.Run(ctx, dockerClient, password, command, repository.Environment, mounts, arcaneNetworkModeInternal(ctx, dockerClient))
	var commandError *rustic.CommandError
	if volumeRepository && errors.As(err, &commandError) && strings.Contains(commandError.Diagnostic, "No suitable key found for the given password") {
		return "", dto.NewProblem(422, "backup.target.key-mismatch", err)
	}
	if volumeRepository && errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return "", dto.NewProblem(504, "volume.backup.engine-timeout", err)
	}
	return output, err
}

func arcaneNetworkModeInternal(ctx context.Context, dockerClient *client.Client) container.NetworkMode {
	arcane, err := libarcane.InspectCurrentArcaneContainer(ctx, dockerClient)
	if err != nil || arcane == nil || arcane.ID == "" {
		slog.DebugContext(ctx, "backup engine: running Rustic on the default network", "error", err)
		return ""
	}
	return container.NetworkMode("container:" + arcane.ID)
}
