// Source: docklet-next/issues/286
// Revision: b9b59ae92f1b09eb828940860971002eab9267b6
// Retrieved: 2026-11-01
// License: Apache-2.0
// Deviations: Original M16 bind identity and admission rules; M16 bind mounts and backup completion.

package backup

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"github.com/florianfinn/docklet-next/backend/internal/backuptarget"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/volumehelper"
	dto "github.com/florianfinn/docklet-next/types/backup"
	"github.com/moby/moby/client"
	"go.getarcane.app/docker"
	"io"
	"path"
	"strconv"
	"strings"
	"time"
)

func (s *Service) validateCopyTarget(ctx context.Context, id string) error {
	if id == "" {
		return nil
	}
	if s.db == nil {
		return ErrSystemBackupTargetUnavailable
	}
	var target backuptarget.Target
	if err := s.db.WithContext(ctx).First(&target, "id = ? AND retired_at IS NULL", id).Error; err != nil {
		return ErrSystemBackupTargetUnavailable
	}
	if target.HostID != nil && *target.HostID != "0" {
		return ErrSystemBackupTargetUnavailable
	}
	return nil
}
func (s *Service) copySession(ctx context.Context, id string) (*backuptarget.Session, error) {
	if s.docker == nil {
		return nil, errors.New("backup.copy.mount-failed")
	}
	target, err := (&backuptarget.Store{DB: s.db}).Resolve(ctx, id, "0", false)
	if err != nil {
		return nil, err
	}
	cli, err := s.docker.GetClient(ctx)
	if err != nil {
		return nil, err
	}
	image := ""
	if s.settings != nil {
		image = s.settings.GetSettingsConfig().ToolsImage.Value
	}
	image, err = volumehelper.ResolveHelperImage(ctx, cli, volumehelper.ToolsImage(image))
	if err != nil {
		return nil, err
	}
	return backuptarget.OpenSession(ctx, cli, image, target, "0", false, copyExec(cli))
}
func copyExec(cli *client.Client) backuptarget.TargetExec {
	return func(ctx context.Context, id, user string, args []string) (string, string, error) {
		var out, stderr bytes.Buffer
		code, err := docker.ExecInContainer(ctx, cli, id, client.ExecCreateOptions{User: user, AttachStdout: true, AttachStderr: true, Cmd: args}, &out, &stderr)
		if err == nil && code != 0 {
			err = errors.New(stderr.String())
		}
		return out.String(), stderr.String(), err
	}
}
func copyCode(ctx context.Context, err error, phase string) string {
	if ctx.Err() != nil {
		return "runtime.interrupted"
	}
	if strings.Contains(strings.ToLower(err.Error()), "no space left") {
		return "backup.copy.insufficient-space"
	}
	return "backup.copy." + phase + "-failed"
}

// copyArchive registers in the drain before pending persistence or any Docker effects.
func (s *Service) copyArchive(ctx context.Context, run *dto.SystemBackupRun) error {
	if run.CopyTargetID == "" {
		return nil
	}
	work, cancel := context.WithCancel(ctx)
	defer cancel()
	release, err := s.maintenance.AdmitCancel(cancel)
	if err != nil {
		run.CopyState = dto.SystemBackupCopyFailed
		run.CopyErrorCode = "runtime.interrupted"
		_ = s.store.SaveRun(context.WithoutCancel(ctx), run)
		return errors.New(run.CopyErrorCode)
	}
	defer release()
	run.CopyState = dto.SystemBackupCopyPending
	run.CopyErrorCode = ""
	if err = s.store.SaveRun(context.WithoutCancel(work), run); err != nil {
		run.CopyState = dto.SystemBackupCopyFailed
		run.CopyErrorCode = "backup.copy.write-failed"
		_ = s.store.SaveRun(context.WithoutCancel(work), run)
		return errors.New("backup.copy.write-failed")
	}
	copyErr := s.writeArchiveCopy(work, run)
	run.CopyState = dto.SystemBackupCopySucceeded
	if copyErr != nil {
		run.CopyState = dto.SystemBackupCopyFailed
		run.CopyErrorCode = copyErr.Error()
	}
	if err = s.store.SaveRun(context.WithoutCancel(work), run); err != nil {
		run.CopyState = dto.SystemBackupCopyFailed
		run.CopyErrorCode = "backup.copy.write-failed"
		_ = s.store.SaveRun(context.WithoutCancel(work), run)
		return errors.New("backup.copy.write-failed")
	}
	return copyErr
}
func (s *Service) writeArchiveCopy(ctx context.Context, run *dto.SystemBackupRun) error {
	session, err := s.copySession(ctx, run.CopyTargetID)
	if err != nil {
		return errors.New(copyCode(ctx, err, "mount"))
	}
	defer session.Close(ctx)
	cli, err := s.docker.GetClient(ctx)
	if err != nil {
		return errors.New(copyCode(ctx, err, "write"))
	}
	exec := copyExec(cli)
	dir := path.Join(session.Root, "docklet-backups", "hub")
	name, err := dto.ArchiveNameForID(run.ID)
	if err != nil || name != run.ArchiveName {
		return errors.New("backup.copy.write-failed")
	}
	partial := path.Join(dir, name+".partial")
	final := path.Join(dir, name)
	defer func() {
		clean, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		defer cancel()
		_, _, _ = exec(clean, session.ID, "", []string{"rm", "-f", "--", partial})
	}()
	if _, _, err = exec(ctx, session.ID, "", []string{"mkdir", "-p", "--", dir}); err != nil {
		return errors.New(copyCode(ctx, err, "write"))
	}
	file, size, _, err := s.OpenArchive(ctx, run.ID)
	if err != nil {
		return errors.New(copyCode(ctx, err, "write"))
	}
	defer file.Close()
	reader, writer := io.Pipe()
	done := make(chan error, 1)
	go func() {
		archive := tar.NewWriter(writer)
		e := archive.WriteHeader(&tar.Header{Name: name + ".partial", Size: size, Mode: 0600})
		if e == nil {
			_, e = io.CopyN(archive, file, size)
		}
		if closeErr := archive.Close(); e == nil {
			e = closeErr
		}
		_ = writer.CloseWithError(e)
		done <- e
	}()
	_, err = cli.CopyToContainer(ctx, session.ID, client.CopyToContainerOptions{DestinationPath: dir, Content: reader})
	_ = reader.CloseWithError(err)
	encodeErr := <-done
	if err == nil {
		err = encodeErr
	}
	if err != nil {
		return errors.New(copyCode(ctx, err, "write"))
	}
	_, _, err = exec(ctx, session.ID, "", []string{"sh", "-c", `set -e; test "$(stat -c %s "$1")" = "$3"; mv -- "$1" "$2"`, "sh", partial, final, strconv.FormatInt(size, 10)})
	if err != nil {
		return errors.New(copyCode(ctx, err, "write"))
	}
	return nil
}
func (s *Service) removeArchiveCopy(ctx context.Context, run *dto.SystemBackupRun) error {
	session, err := s.copySession(ctx, run.CopyTargetID)
	if err != nil {
		return err
	}
	defer session.Close(ctx)
	name, err := dto.ArchiveNameForID(run.ID)
	if err != nil || name != run.ArchiveName {
		return errors.New("backup.copy.write-failed")
	}
	cli, err := s.docker.GetClient(ctx)
	if err != nil {
		return err
	}
	_, _, err = copyExec(cli)(ctx, session.ID, "", []string{"rm", "-f", "--", path.Join(session.Root, "docklet-backups", "hub", name)})
	return err
}
