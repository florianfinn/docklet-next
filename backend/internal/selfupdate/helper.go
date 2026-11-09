// Source: docklet-next/issues/439
// Revision: 910032a54830f5ce1d37070964fa2dc7efb8aa2f
// Retrieved: 2026-11-09
// License: Apache-2.0
// Deviations: Original Compose-faithful self-update implementation.

package selfupdate

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"time"

	composetypes "github.com/compose-spec/compose-go/v2/types"
)

// HelperData is fixed in release builds. Process-boundary tests stamp a private
// directory into their binary to exercise the real command without host mounts.
var HelperData = "/selfupdate-data"

type Request struct {
	OpID            string    `json:"opId"`
	Role            string    `json:"role"`
	OldContainerID  string    `json:"oldContainerId"`
	Replay          Replay    `json:"replay"`
	TargetReference string    `json:"targetReference"`
	TargetVersion   string    `json:"targetVersion"`
	Deadline        time.Time `json:"deadline"`
	// AppUID/AppGID own the result file so the application can read it after
	// the swap; the helper itself runs as root (HelperCreate).
	AppUID int `json:"appUid"`
	AppGID int `json:"appGid"`
}

type Result struct {
	OpID      string    `json:"opId"`
	TargetRef string    `json:"targetRef"`
	Phase     string    `json:"phase"`
	UpdatedAt time.Time `json:"updatedAt"`
}

func ResultPath(data, id string) string { return filepath.Join(data, "self-update", id+".json") }
func RequestPath(data, id string) string {
	return filepath.Join(data, "self-update", id+".request.json")
}

// Helper's only effect seam is the actual Compose executor. Tests may hold this
// seam at a phase barrier; the CLI always supplies the embedded implementation.
type Helper struct {
	Up      func(context.Context, *composetypes.Project, string) error
	Healthy func(context.Context, Request) error
	// Phase is an observation hook, never an alternate path for production effects.
	Phase func(string)
}

func (h Helper) Run(ctx context.Context, data, id string) (err error) {
	if !ValidOpID(id) {
		return Refuse("update.invalid-target")
	}
	var request Request
	if err = ReadJSON(RequestPath(data, id), &request); err != nil {
		return err
	}
	if request.OpID != id || request.TargetReference == "" || request.TargetVersion == "" || request.Deadline.IsZero() {
		return Refuse("update.invalid-target")
	}
	if !time.Now().Before(request.Deadline) {
		return Refuse(CodeUnknown)
	}
	ctx, cancel := context.WithDeadline(ctx, request.Deadline)
	defer cancel()
	result := Result{OpID: id, TargetRef: request.TargetReference}
	phase := func(value string) error {
		result.Phase = value
		result.UpdatedAt = time.Now().UTC()
		if err := writeOwnedJSON(ResultPath(data, id), result, request.AppUID, request.AppGID); err != nil {
			return err
		}
		if h.Phase != nil {
			h.Phase(value)
		}
		return nil
	}
	// A prior phase means this operation has already run. Even an interrupted
	// helper must not replace its original backup with the new env bytes.
	if _, statErr := os.Lstat(ResultPath(data, id)); statErr == nil {
		return Refuse(CodeUnknown)
	} else if !os.IsNotExist(statErr) {
		return statErr
	}
	if _, err = request.Replay.DryRun(ctx); err != nil {
		_ = phase("failed-pre-effect")
		return err
	}
	target, err := request.Replay.Target(ctx, request.TargetReference)
	if err != nil {
		_ = phase("failed-pre-effect")
		return err
	}
	if err = phase("validated"); err != nil {
		return err
	}
	original, err := os.ReadFile(request.Replay.EnvFile)
	if err != nil {
		_ = phase("failed-pre-effect")
		return err
	}
	info, err := os.Stat(request.Replay.EnvFile)
	if err != nil {
		_ = phase("failed-pre-effect")
		return err
	}
	owner, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return Refuse(CodeLayout)
	}
	backup := request.Replay.EnvFile + ".docklet-previous." + id
	if err = backupEnv(backup, original, info.Mode(), owner); err != nil {
		_ = phase("failed-pre-effect")
		return err
	}
	restore := func(cause error) error {
		// The rename is the last step of a replacement: unchanged bytes mean the
		// env file was never replaced, which is a pre-effect failure.
		if envBytesEqual(request.Replay.EnvFile, original) {
			return errors.Join(cause, phase("failed-pre-effect"))
		}
		restoreErr := replaceEnv(request.Replay.EnvFile, original, info.Mode(), owner)
		// A failed restore is ambiguous, never advertised as pre-effect recovery.
		if restoreErr != nil {
			return errors.Join(cause, restoreErr)
		}
		return errors.Join(cause, phase("failed-pre-effect"))
	}
	if err = replaceEnv(request.Replay.EnvFile, writePin(original, request.Replay.Variable, request.TargetReference), info.Mode(), owner); err != nil {
		return restore(err)
	}
	if err = phase("env-written"); err != nil {
		return restore(err)
	}
	// No automatic restoration or rollback is allowed after this durable point:
	// an Engine reply can be lost after it has already created the replacement.
	if err = phase("up-started"); err != nil {
		return restore(err)
	}
	if err = h.Up(ctx, target, request.Replay.Service); err != nil {
		var before preEffectError
		if errors.As(err, &before) {
			return restore(err)
		}
		_ = phase("failed-health")
		return err
	}
	healthCtx, stopHealth := context.WithTimeout(ctx, 120*time.Second)
	defer stopHealth()
	if err = h.Healthy(healthCtx, request); err != nil {
		_ = phase("failed-health")
		return err
	}
	return phase("healthy")
}
