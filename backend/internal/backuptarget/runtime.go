// Source: docklet-next/issues/286
// Revision: b9b59ae92f1b09eb828940860971002eab9267b6
// Retrieved: 2026-11-01
// License: Apache-2.0
// Deviations: Original M16 bind identity and admission rules; M16 bind mounts and backup completion.

package backuptarget

import (
	"context"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/volumehelper"
	dto "github.com/florianfinn/docklet-next/types/backup"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
	"path"
	"strings"
	"time"
	"uuid"
)

type TargetExec func(context.Context, string, string, []string) (string, string, error)
type Session struct {
	ID, Root        string
	Plan            MountPlan
	cli             *client.Client
	exec            TargetExec
	transient       string
	cleanupDeadline time.Time
}

func Root(target dto.SyncedTarget) string {
	if target.Kind == "local" || target.Kind == "mount" {
		return "/target"
	}
	return path.Join("/target", target.Path)
}

// OpenSession owns only its helper and, for drafts, its randomly named mount volume.
func OpenSession(ctx context.Context, cli *client.Client, image string, target dto.SyncedTarget, host string, transient bool, exec TargetExec) (session *Session, err error) {
	return openSession(ctx, cli, image, target, host, transient, exec, time.Time{})
}

func openSession(ctx context.Context, cli *client.Client, image string, target dto.SyncedTarget, host string, transient bool, exec TargetExec, cleanupDeadline time.Time) (session *Session, err error) {
	plan := Plan(target, host, false)
	session = &Session{Plan: plan, Root: Root(target), cli: cli, exec: exec, cleanupDeadline: cleanupDeadline}
	if plan.Volume != nil {
		if transient {
			name := "docklet-target-probe-" + uuid.New().String()
			plan.Volume.Name = name
			plan.Mounts[0].Source = name
			session.transient = name
			session.Plan = plan
		}
		if _, err = cli.VolumeCreate(ctx, *plan.Volume); err != nil {
			_ = session.Close(ctx)
			return nil, ProbeProblem(err)
		}
	}
	defer func() {
		if err != nil {
			_ = session.Close(ctx)
		}
	}()
	labels := volumehelper.Labels()
	if transient {
		labels["docklet.target-probe"] = "true"
	} else {
		labels["docklet.hub-copy"] = "true"
	}
	created, err := cli.ContainerCreate(ctx, client.ContainerCreateOptions{Config: &container.Config{Image: image, Cmd: []string{"sleep", "infinity"}, NetworkDisabled: true, Labels: labels}, HostConfig: volumehelper.HostConfig(image, nil, plan.Mounts)})
	if err != nil {
		return session, ProbeProblem(err)
	}
	session.ID = created.ID
	if _, err = cli.ContainerStart(ctx, session.ID, client.ContainerStartOptions{}); err != nil {
		return session, ProbeProblem(err)
	}
	if plan.Witness {
		var out string
		out, _, err = exec(ctx, session.ID, "", []string{"stat", "-c", "%d", "/target", "/parent"})
		if err != nil {
			return session, ProbeProblem(err)
		}
		if err = CheckWitness(out); err != nil {
			return session, err
		}
	}
	return session, nil
}
func (s *Session) Close(ctx context.Context) error {
	ctx, cancel := s.cleanupContext(ctx)
	defer cancel()
	if s.ID != "" {
		if _, err := s.cli.ContainerRemove(ctx, s.ID, volumehelper.RemoveOptions()); err != nil {
			return err
		}
		s.ID = ""
	}
	if s.transient != "" {
		_, err := s.cli.VolumeRemove(ctx, s.transient, client.VolumeRemoveOptions{})
		return err
	}
	return nil
}

// Cleanup survives caller cancellation while respecting a probe's total budget.
func (s *Session) cleanupContext(ctx context.Context) (context.Context, context.CancelFunc) {
	deadline := time.Now().Add(30 * time.Second)
	if !s.cleanupDeadline.IsZero() && s.cleanupDeadline.Before(deadline) {
		deadline = s.cleanupDeadline
	}
	return context.WithDeadline(context.WithoutCancel(ctx), deadline)
}

func ProbeProblem(err error) error {
	text := strings.ToLower(err.Error())
	code := "backup.target.mount-failed"
	switch {
	case strings.Contains(text, "no such file") || strings.Contains(text, "does not exist"):
		code = "backup.target.path-missing"
	case strings.Contains(text, "permission denied"):
		code = "backup.target.permission-denied"
	case strings.Contains(text, "unreachable") || strings.Contains(text, "timed out"):
		code = "backup.target.unreachable"
	}
	return dto.NewProblem(422, code, err)
}
func Probe(ctx context.Context, cli *client.Client, image string, target dto.SyncedTarget, host string, exec TargetExec) (dto.TargetProbeResult, error) {
	started := time.Now()
	deadline := started.Add(60 * time.Second)
	if parent, ok := ctx.Deadline(); ok && parent.Before(deadline) {
		deadline = parent
	}
	// Reserve cleanup time inside the synchronous response budget.
	reserve := min(15*time.Second, max(0, deadline.Sub(started)/4))
	ctx, cancel := context.WithDeadline(ctx, deadline.Add(-reserve))
	defer cancel()
	session, err := openSession(ctx, cli, image, target, host, true, exec, deadline)
	if err != nil {
		return probeResult(err), nil
	}
	file := path.Join(session.Root, ".docklet-probe-"+uuid.New().String())
	_, _, err = exec(ctx, session.ID, "", []string{"sh", "-c", `set -e; printf probe > "$1"; rm -f -- "$1"`, "sh", file})
	if err != nil {
		cleanup, cleanupCancel := session.cleanupContext(ctx)
		_, _, _ = exec(cleanup, session.ID, "", []string{"rm", "-f", "--", file})
		cleanupCancel()
		err = ProbeProblem(err)
	}
	closeErr := session.Close(ctx)
	if err == nil && closeErr != nil {
		err = ProbeProblem(closeErr)
	}
	return probeResult(err), nil
}
func probeResult(err error) dto.TargetProbeResult {
	if err == nil {
		return dto.TargetProbeResult{OK: true}
	}
	return dto.TargetProbeResult{Code: err.Error()}
}
