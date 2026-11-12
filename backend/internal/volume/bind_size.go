// Source: docklet-next/issues/327
// Revision: b9b59ae92f1b09eb828940860971002eab9267b6
// Retrieved: 2026-11-01
// License: Apache-2.0
// Deviations: Original M16 bind identity and admission rules; M16 bind mounts and backup completion.

package volume

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"github.com/florianfinn/docklet-next/backend/internal/volume/bindpath"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/volumehelper"
	dto "github.com/florianfinn/docklet-next/types/backup"
	volumetypes "github.com/florianfinn/docklet-next/types/volume"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/client"
	"go.getarcane.app/docker"
	"io"
	"strconv"
	"strings"
	"time"
)

type bindScan struct {
	ctx     context.Context
	cancel  context.CancelFunc
	done    chan struct{}
	waiters int
	result  volumetypes.BindMeasurement
	err     error
}

func (s *VolumeService) MeasureBind(ctx context.Context, key string) (volumetypes.BindMeasurement, error) {
	if _, err := volumetypes.BindSource(key); err != nil {
		return volumetypes.BindMeasurement{}, err
	}
	return s.bindState().measure(ctx, key, 120*time.Second, s.scanBind)
}
func (state *bindInventory) measure(ctx context.Context, key string, timeout time.Duration, scan func(context.Context, string) (volumetypes.BindMeasurement, error)) (volumetypes.BindMeasurement, error) {
	state.mu.Lock()
	run := state.scans[key]
	if run == nil {
		work, cancel := context.WithTimeout(context.WithoutCancel(ctx), timeout)
		run = &bindScan{ctx: work, cancel: cancel, done: make(chan struct{})}
		state.scans[key] = run
		go func() {
			result, err := scan(work, key)
			cancel()
			state.mu.Lock()
			run.result, run.err = result, err
			if err == nil {
				state.measured[key] = result
			}
			if state.scans[key] == run {
				delete(state.scans, key)
			}
			close(run.done)
			state.mu.Unlock()
		}()
	}
	run.waiters++
	state.mu.Unlock()
	defer func() {
		state.mu.Lock()
		run.waiters--
		if run.waiters == 0 {
			run.cancel()
			if state.scans[key] == run {
				delete(state.scans, key)
			}
		}
		state.mu.Unlock()
	}()
	select {
	case <-ctx.Done():
		return volumetypes.BindMeasurement{}, ctx.Err()
	case <-run.done:
		return run.result, run.err
	}
}

func (s *VolumeService) scanBind(ctx context.Context, key string) (result volumetypes.BindMeasurement, err error) {
	defer func() {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			err = dto.NewProblem(504, "volume.bind.size-timeout", ctx.Err())
		}
	}()
	cli, err := s.dockerService.GetClient(ctx)
	if err != nil {
		return result, err
	}
	source, err := bindpath.Check(ctx, cli, s.db, key)
	if err != nil {
		return result, err
	}
	image, err := s.getVolumeHelperImageInternal(ctx, cli)
	if err != nil {
		return result, err
	}
	kind, err := bindpath.Witness(ctx, cli, image, source, s.execInContainerInternal)
	if err != nil {
		return result, err
	}
	labels := volumehelper.Labels()
	labels["docklet.bind-key"] = key
	created, err := cli.ContainerCreate(ctx, client.ContainerCreateOptions{Config: &container.Config{Image: image, Cmd: []string{"sleep", "infinity"}, Labels: labels}, HostConfig: volumehelper.HostConfig(image, nil, []mount.Mount{{Type: mount.TypeBind, Source: source, Target: "/source", ReadOnly: true}})})
	if err != nil {
		return result, err
	}
	defer func() {
		clean, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		defer cancel()
		_, _ = cli.ContainerRemove(clean, created.ID, volumehelper.RemoveOptions())
	}()
	if _, err = cli.ContainerStart(ctx, created.ID, client.ContainerStartOptions{}); err != nil {
		return result, err
	}
	result = volumetypes.BindMeasurement{Key: key}
	if kind == "file" {
		out, _, e := s.execInContainerInternal(ctx, created.ID, "", []string{"stat", "-c", "%s", "/source"})
		if e != nil {
			return result, e
		}
		result.MeasuredSize, err = strconv.ParseInt(strings.TrimSpace(out), 10, 64)
		result.MeasuredAt = time.Now().UTC()
		return result, err
	}
	device, _, err := s.execInContainerInternal(ctx, created.ID, "", []string{"stat", "-c", "%d", "/source"})
	if err != nil {
		return result, err
	}
	aggregate := bindSizeSum{device: strings.TrimSpace(device), seen: map[string]bool{}}
	reader, writer := io.Pipe()
	done := make(chan error, 1)
	go func() {
		code, e := docker.ExecInContainer(ctx, cli, created.ID, client.ExecCreateOptions{AttachStdout: true, AttachStderr: true, Cmd: []string{"find", "/source"}}, writer, io.Discard)
		if e == nil && code != 0 {
			e = fmt.Errorf("find failed")
		}
		_ = writer.CloseWithError(e)
		done <- e
	}()
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 4096), 1024*1024)
	batch := []string{}
	flush := func() {
		if len(batch) == 0 {
			return
		}
		args := append([]string{"stat", "-c", "%d %i %h %s %F", "--"}, batch...)
		out, _, e := s.execInContainerInternal(ctx, created.ID, "", args)
		if e != nil {
			aggregate.partial = true
		}
		lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
		if len(lines) != len(batch) {
			aggregate.partial = true
		}
		for _, line := range lines {
			aggregate.add(line)
		}
		batch = batch[:0]
	}
	for scanner.Scan() {
		name := scanner.Text()
		if name != "/source" && !strings.HasPrefix(name, "/source/") {
			aggregate.partial = true
			continue
		}
		batch = append(batch, name)
		if len(batch) == 500 {
			flush()
		}
		if ctx.Err() != nil {
			break
		}
	}
	flush()
	readErr := scanner.Err()
	_ = reader.Close()
	execErr := <-done
	if ctx.Err() != nil {
		return result, ctx.Err()
	}
	if readErr != nil || execErr != nil {
		aggregate.partial = true
	}
	result.MeasuredSize, result.MeasuredPartial, result.MeasuredAt = aggregate.size, aggregate.partial, time.Now().UTC()
	return result, nil
}

type bindSizeSum struct {
	device  string
	seen    map[string]bool
	size    int64
	partial bool
}

func (s *bindSizeSum) add(line string) {
	fields := strings.Fields(line)
	if len(fields) < 5 {
		s.partial = true
		return
	}
	for _, field := range fields[:3] {
		if _, err := strconv.ParseUint(field, 10, 64); err != nil {
			s.partial = true
			return
		}
	}
	size, err := strconv.ParseInt(fields[3], 10, 64)
	if err != nil || size < 0 {
		s.partial = true
		return
	}
	if fields[0] != s.device || !strings.HasPrefix(strings.Join(fields[4:], " "), "regular") {
		return
	}
	inode := fields[0] + ":" + fields[1]
	if s.seen[inode] {
		return
	}
	s.seen[inode] = true
	if size > int64(^uint64(0)>>1)-s.size {
		s.partial = true
		return
	}
	s.size += size
}
