// Source: docklet-next/issues/283
// Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original bounded volume-engine Docker output and immediate witness hooks.

package rustic

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"

	dto "github.com/florianfinn/docklet-next/types/backup"
)

type volumeOutputKey struct{}
type beforeCreateKey struct{}

func WithVolumeOutputLimit(ctx context.Context) context.Context {
	return context.WithValue(ctx, volumeOutputKey{}, true)
}
func WithBeforeCreate(ctx context.Context, witness func(context.Context) error) context.Context {
	return context.WithValue(ctx, beforeCreateKey{}, witness)
}

// Reject an oversized frame before reading its payload. stdcopy grows its frame
// buffer from the daemon-provided length, so limiting its destination is too late.
func readVolumeOutput(source io.Reader, listing ...bool) (string, string, error) {
	var stdout, stderr bytes.Buffer
	remaining := uint64(dto.MaxVolumeResponseBytes)
	invalid := func(cause error) (string, string, error) {
		return "", "", dto.NewProblem(502, "volume.backup.engine-output-invalid", cause)
	}
	for {
		var header [8]byte
		n, err := io.ReadFull(source, header[:])
		if err == io.EOF && n == 0 {
			return stdout.String(), stderr.String(), nil
		}
		if err != nil {
			return invalid(err)
		}
		if (header[0] != 1 && header[0] != 2) || header[1] != 0 || header[2] != 0 || header[3] != 0 {
			return invalid(nil)
		}
		size := uint64(binary.BigEndian.Uint32(header[4:]))
		if size > remaining {
			if len(listing) > 0 && listing[0] {
				return "", "", dto.NewProblem(422, "volume.backup.listing-too-large", nil)
			}
			return invalid(nil)
		}
		remaining -= size
		destination := &stdout
		if header[0] == 2 {
			destination = &stderr
		}
		if _, err := io.CopyN(destination, source, int64(size)); err != nil {
			return invalid(err)
		}
	}
}

// CommandError keeps bounded daemon diagnostics available to repository-specific
// classification. HTTP producers expose only their typed neutral problem codes.
type CommandError struct {
	ExitCode   int64
	Diagnostic string
}

func (e *CommandError) Error() string {
	return fmt.Sprintf("rustic exited with code %d: %s", e.ExitCode, e.Diagnostic)
}
