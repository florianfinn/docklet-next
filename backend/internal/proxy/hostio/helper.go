// Source: docklet-next/issues/473
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-15
// License: Apache-2.0
// Deviations: Original engine-independent proxy host boundary.

package hostio

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json/v2"
	"fmt"
	"github.com/florianfinn/docklet-next/backend/internal/volume"
	dto "github.com/florianfinn/docklet-next/types/proxy"
	"github.com/google/uuid"
	"github.com/moby/moby/api/pkg/stdcopy"
	"github.com/moby/moby/client"
	"go.getarcane.app/acfs/types"
	"go.getarcane.app/docker"
	"io"
	"path"
	"strconv"
	"strings"
)

type FileWriter interface {
	Write(context.Context, string, []byte, uint32) error
	Replace(context.Context, string, string) error
	Remove(context.Context, string) error
	Mkdir(context.Context, string) error
	RemoveTree(context.Context, string) error
}
type HelperWriter struct {
	Volumes *volume.VolumeService
	Key     string
	Guard   func(context.Context) error
}
type boundedBuffer struct {
	bytes.Buffer
	limit int
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	if len(p) > b.limit-b.Len() {
		return 0, dto.Fail(dto.InputLimit, 422)
	}
	return b.Buffer.Write(p)
}
func HelperExec(ctx context.Context, cli *client.Client, id string, args []string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, ExecTimeout)
	defer cancel()
	out := &boundedBuffer{limit: MaxExecBytes}
	stderr := &boundedBuffer{limit: MaxExecBytes}
	exit, err := docker.ExecInContainer(ctx, cli, id, client.ExecCreateOptions{AttachStdout: true, AttachStderr: true, Cmd: args}, out, stderr)
	if err != nil {
		return nil, err
	}
	if out.Len()+stderr.Len() > MaxExecBytes {
		return nil, dto.Fail(dto.InputLimit, 422)
	}
	if exit != 0 {
		return nil, dto.Fail(dto.ExecFailed, 422)
	}
	return out.Bytes(), nil
}

// Metadata must be provable with the installed helper. Older helpers without an
// xattr reader fail closed rather than silently losing ACLs or attributes.
const helperMetadataScript = `set -eu
p="$1"
command -v getfattr >/dev/null 2>&1
if ! test -e "$p"; then exit 0; fi
test ! -L "$p"
test -f "$p"
test "$(stat -c %h "$p")" = 1
m="$(stat -c %a "$p")"
case "$m" in [0-7][0-7][0-7]) ;; *) exit 1;; esac
case "$m" in ?[4567]?|??[4567]) ;; *) exit 1;; esac
command -v getfattr >/dev/null 2>&1
attrs="$(getfattr -m - --absolute-names "$p" 2>/dev/null)"
test -z "$attrs"
`

func helperCheck(ctx context.Context, cli *client.Client, id, p string) error {
	if !Relative(p) || Secret(p) {
		return dto.Fail(dto.InvalidInput, 422)
	}
	// ACFS stat's no-follow projection guards every existing parent segment.
	parts := strings.Split(p, "/")
	for n := 1; n < len(parts); n++ {
		b, err := HelperExec(ctx, cli, id, []string{"acfs", "stat", "--root", "/volume", "--path", "/" + strings.Join(parts[:n], "/")})
		if err != nil {
			return err
		}
		var s types.StatResponse
		if json.Unmarshal(b, &s) != nil || s.Version != types.ProtocolVersion || s.Entry.IsSymlink || !s.Entry.IsDirectory {
			return dto.Fail(dto.SourceReadOnly, 409)
		}
	}
	_, err := HelperExec(ctx, cli, id, []string{"sh", "-c", helperMetadataScript, "sh", path.Join("/volume", p)})
	if err != nil {
		return dto.Fail(dto.UnsupportedMetadata, 409)
	}
	return nil
}
func (w HelperWriter) call(ctx context.Context, p string, fn func(context.Context, *client.Client, string) error) error {
	if w.Volumes == nil {
		return dto.Fail(dto.EngineUnavailable, 503)
	}
	return w.Volumes.WithProxyHelper(ctx, w.Key, func(ctx context.Context, cli *client.Client, id string) error {
		if w.Guard != nil {
			if err := w.Guard(ctx); err != nil {
				return err
			}
		}
		if err := helperCheck(ctx, cli, id, p); err != nil {
			return err
		}
		if w.Guard != nil {
			if err := w.Guard(ctx); err != nil {
				return err
			}
		}
		return fn(ctx, cli, id)
	})
}
func (w HelperWriter) Write(ctx context.Context, p string, b []byte, mode uint32) error {
	if len(b) > MaxFileBytes {
		return dto.Fail(dto.InputLimit, 422)
	}
	return w.call(ctx, p, func(ctx context.Context, cli *client.Client, id string) error {
		return helperWriteInput(ctx, cli, id, []string{"acfs", "write", "--root", "/volume", "--path", "/" + p, "--size", strconv.Itoa(len(b)), "--mode", fmt.Sprintf("%04o", mode&0777)}, b)
	})
}
func helperWriteInput(ctx context.Context, cli *client.Client, id string, args []string, b []byte) error {
	ctx, cancel := context.WithTimeout(ctx, ExecTimeout)
	defer cancel()
	created, err := cli.ExecCreate(ctx, id, client.ExecCreateOptions{AttachStdin: true, AttachStdout: true, AttachStderr: true, Cmd: args})
	if err != nil {
		return err
	}
	attached, err := cli.ExecAttach(ctx, created.ID, client.ExecAttachOptions{})
	if err != nil {
		return err
	}
	defer attached.Close()
	stop := context.AfterFunc(ctx, attached.Close)
	defer stop()
	out := &boundedBuffer{limit: MaxExecBytes}
	stderr := &boundedBuffer{limit: MaxExecBytes}
	done := make(chan error, 1)
	go func() { _, err := stdcopy.StdCopy(out, stderr, attached.Reader); done <- err }()
	_, writeErr := io.Copy(attached.Conn, bytes.NewReader(b))
	closeErr := attached.CloseWrite()
	readErr := <-done
	if ctx.Err() != nil {
		return dto.Fail(dto.ExecRunning, 409)
	}
	if writeErr != nil {
		return writeErr
	}
	if closeErr != nil {
		return closeErr
	}
	if readErr != nil {
		return readErr
	}
	inspected, err := cli.ExecInspect(ctx, created.ID, client.ExecInspectOptions{})
	if err != nil {
		return err
	}
	if inspected.Running {
		return dto.Fail(dto.ExecRunning, 409)
	}
	if inspected.ExitCode != 0 {
		return dto.Fail(dto.ExecFailed, 422)
	}
	return nil
}
func (w HelperWriter) Replace(ctx context.Context, from, to string) error {
	if !Relative(from) {
		return dto.Fail(dto.InvalidInput, 422)
	}
	return w.call(ctx, to, func(ctx context.Context, cli *client.Client, id string) error {
		if err := helperCheck(ctx, cli, id, from); err != nil {
			return err
		}
		manifest, err := json.Marshal(types.ApplyManifest{Version: types.ProtocolVersion, Changes: []types.ApplyChange{{Operation: types.ApplyRename, Path: "/" + from, TargetPath: "/" + to}}})
		if err != nil {
			return err
		}
		return helperApply(ctx, cli, id, manifest, w.Guard)
	})
}
func helperApply(ctx context.Context, cli *client.Client, id string, manifest []byte, guard func(context.Context) error) error {
	stage := "docklet-proxy-" + uuid.NewString()
	_, err := HelperExec(ctx, cli, id, []string{"acfs", "mkdir", "--root", "/tmp", "--path", "/" + stage})
	if err != nil {
		return err
	}
	defer func() {
		clean, cancel := context.WithTimeout(context.WithoutCancel(ctx), ExecTimeout)
		defer cancel()
		_, _ = HelperExec(clean, cli, id, []string{"acfs", "remove", "--root", "/tmp", "--path", "/" + stage})
	}()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	if err = tw.WriteHeader(&tar.Header{Name: "manifest.json", Mode: 0600, Size: int64(len(manifest))}); err != nil {
		return err
	}
	if _, err = tw.Write(manifest); err != nil {
		return err
	}
	if err = tw.Close(); err != nil {
		return err
	}
	if _, err = cli.CopyToContainer(ctx, id, client.CopyToContainerOptions{DestinationPath: "/tmp/" + stage, Content: bytes.NewReader(buf.Bytes())}); err != nil {
		return err
	}
	if guard != nil {
		if err := guard(ctx); err != nil {
			return err
		}
	}
	out, err := HelperExec(ctx, cli, id, []string{"acfs", "apply", "--root", "/volume", "--staging", "/tmp/" + stage, "--manifest", "manifest.json"})
	if err != nil {
		return err
	}
	var response types.ApplyResponse
	if json.Unmarshal(out, &response) != nil || response.Applied != 1 || response.Version != types.ProtocolVersion {
		return fmt.Errorf("invalid helper apply receipt")
	}
	return nil
}
func (w HelperWriter) Remove(ctx context.Context, p string) error {
	if !Relative(p) {
		return dto.Fail(dto.InvalidInput, 422)
	}
	run := func(ctx context.Context, cli *client.Client, id string) error {
		_, err := HelperExec(ctx, cli, id, []string{"acfs", "remove", "--root", "/volume", "--path", "/" + p})
		return err
	}
	base := path.Base(p)
	if strings.HasPrefix(base, ".docklet-") && strings.HasSuffix(base, ".tmp") || strings.HasPrefix(base, "docklet-check-") && strings.HasSuffix(base, ".yml") {
		return w.Volumes.WithProxyHelper(ctx, w.Key, run)
	}
	return w.call(ctx, p, run)
}

func (w HelperWriter) Mkdir(ctx context.Context, p string) error {
	return w.call(ctx, p, func(ctx context.Context, cli *client.Client, id string) error {
		_, err := HelperExec(ctx, cli, id, []string{"acfs", "mkdir", "--root", "/volume", "--path", "/" + p, "--mode", "0755"})
		return err
	})
}
func (w HelperWriter) RemoveTree(ctx context.Context, p string) error {
	if !Relative(p) || !strings.HasPrefix(path.Base(p), ".docklet-staging-") {
		return dto.Fail(dto.InvalidInput, 422)
	}
	return w.Volumes.WithProxyHelper(ctx, w.Key, func(ctx context.Context, cli *client.Client, id string) error {
		_, err := HelperExec(ctx, cli, id, []string{"acfs", "remove", "--root", "/volume", "--path", "/" + p})
		return err
	})
}
