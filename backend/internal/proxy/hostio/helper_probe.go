// Source: docklet-next/issues/493
// Revision: 54fd54b6bf259e09c1d4fa66ab1cd7ebdebf09e6
// Retrieved: 2026-11-20
// License: Apache-2.0
// Deviations: Original bounded probe client and certificate observation integration.

package hostio

import (
	"context"
	"encoding/binary"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/containerd/errdefs"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/volumehelper"
	dto "github.com/florianfinn/docklet-next/types/proxy"
	"github.com/google/uuid"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
)

const HelperProbeBudget = 12 * time.Second
const ProbeCleanupBudget = 10 * time.Second
const ProbeVersionBudget = 5 * time.Second
const ProbeLabel = "docklet.helper"
const ProbeOperationLabel = "docklet.probe-operation"

type PreparedProbe struct {
	Image       volumehelper.Image
	Reason      dto.Reason
	SupportsTLS bool
}
type probeCapability struct {
	Reason dto.Reason
	TLS    bool
}

// ProbeRunner is agent-local. Capabilities belong to an immutable local image ID,
// while each admission resolves the configured reference again (mutable tags).
type ProbeRunner struct {
	Client       *client.Client
	Docker       func(context.Context) (*client.Client, error)
	Resolve      func(context.Context) (volumehelper.Image, error)
	mu           sync.Mutex
	capabilities map[string]probeCapability
}

func (r *ProbeRunner) Prepare(ctx context.Context) (PreparedProbe, error) {
	image, err := r.Resolve(ctx)
	if err != nil {
		var absent *volumehelper.ImageUnavailable
		if errors.As(err, &absent) {
			return PreparedProbe{Reason: dto.ProbeImageUnavailable}, nil
		}
		return PreparedProbe{}, err
	}
	if image.ID == "" {
		return PreparedProbe{}, &volumehelper.DockerError{Err: errors.New("helper image has no local ID")}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if capability, ok := r.capabilities[image.ID]; ok {
		return PreparedProbe{Image: image, Reason: capability.Reason, SupportsTLS: capability.TLS}, nil
	}
	versionCtx, cancel := context.WithTimeout(ctx, ProbeVersionBudget)
	defer cancel()
	res, err := r.run(versionCtx, image.ID, "", []string{"/usr/bin/docklet-probe", "version"})
	reason := dto.Reason("")
	supportsTLS := false
	if err != nil {
		var cleanup *volumehelper.DockerError
		if errors.As(err, &cleanup) {
			return PreparedProbe{}, err
		}
		message := err.Error()
		if strings.Contains(message, "executable file not found") || strings.Contains(message, "/usr/bin/docklet-probe") && strings.Contains(message, "no such file or directory") {
			reason = dto.ProbeClientMissing
		} else {
			return PreparedProbe{}, err
		}
	} else if res.ExitCode == 127 {
		reason = dto.ProbeClientMissing
	} else {
		var version struct {
			V            int            `json:"v"`
			Version      string         `json:"version"`
			Capabilities jsontext.Value `json:"capabilities"`
		}
		if len(res.Stdout) > MaxProbeJSON || json.Unmarshal(res.Stdout, &version) != nil {
			return PreparedProbe{}, dto.Fail(dto.NotObservable, 409)
		}
		var capabilities []string
		explicit := len(version.Capabilities) > 0
		if explicit && (string(version.Capabilities) == "null" || json.Unmarshal(version.Capabilities, &capabilities) != nil) {
			return PreparedProbe{}, dto.Fail(dto.NotObservable, 409)
		}
		if res.ExitCode != 0 || version.V != 1 || version.Version == "" || explicit && !slices.Contains(capabilities, "http") {
			reason = dto.ProbeClientUnsupported
		} else {
			// Published v1 predates capability negotiation and has a fixed HTTP/TLS contract.
			supportsTLS = !explicit || slices.Contains(capabilities, "tls")
		}
	}
	if r.capabilities == nil {
		r.capabilities = map[string]probeCapability{}
	}
	r.capabilities[image.ID] = probeCapability{Reason: reason, TLS: supportsTLS}
	return PreparedProbe{Image: image, Reason: reason, SupportsTLS: supportsTLS}, nil
}

func (r *ProbeRunner) Probe(ctx context.Context, p PreparedProbe, i dto.Instance, inspect Inspect, host, rawURL, sni string, fallback func(context.Context) (Response, error)) (Response, error) {
	ctx, cancel := context.WithTimeout(ctx, HelperProbeBudget)
	defer cancel()
	args, tls, err := ProbeArgs(host, rawURL, sni)
	if err != nil {
		return Response{}, err
	}
	before, err := inspect(ctx)
	if err != nil {
		return Response{}, err
	}
	fence := LogFence{Expected: before, Inspect: func(ctx context.Context) (Epoch, error) {
		after, err := inspect(ctx)
		if err == nil && !after.Running {
			return after, dto.Fail(dto.ContainerChanged, 409)
		}
		return after, err
	}}
	if !before.Running || before.ContainerID != i.Identity.ContainerID {
		return Response{}, dto.Fail(dto.ContainerChanged, 409)
	}
	var response Response
	if tls && !p.SupportsTLS && p.Reason == "" {
		p.Reason = dto.ProbeClientUnsupported
	}
	if p.Reason != "" {
		response, err = fallback(ctx)
		response.Probe = &dto.ProbeEvidence{Client: dto.WgetClient, Reason: p.Reason}
	} else {
		var res ExecResult
		res, err = r.run(ctx, p.Image.ID, before.ContainerID, args)
		if err == nil {
			response, err = ParseHelperProbe(res, tls)
			if err == nil && tls && response.Certificate != nil && response.Certificate.Route.SNI != args[len(args)-1] {
				err = dto.Fail(dto.NotObservable, 409)
			}
		}
		response.Probe = &dto.ProbeEvidence{Client: dto.HelperClient, ImageID: p.Image.ID}
	}
	// A response arriving after cancellation or from a surviving old netns is not evidence.
	if ctx.Err() != nil {
		return Response{}, ctx.Err()
	}
	if fenceErr := fence.Check(ctx); fenceErr != nil {
		if p.Reason == "" && probeInfrastructure(fenceErr) {
			return Response{}, &ProbeFailure{Err: fenceErr}
		}
		return Response{}, fenceErr
	}
	if err != nil {
		if p.Reason == "" && probeInfrastructure(err) {
			return Response{}, &ProbeFailure{Err: err}
		}
		return Response{}, err
	}
	if response.Certificate != nil {
		response.Certificate.InstanceID = i.ID
		response.Certificate.Epoch = dto.CertificateEpoch{ContainerID: before.ContainerID, StartedAt: before.StartedAt}
		response.Certificate.Route.Host = host
	}
	return response, nil
}

func (r *ProbeRunner) run(ctx context.Context, image, target string, args []string) (out ExecResult, err error) {
	cli := r.Client
	if r.Docker != nil {
		cli, err = r.Docker(ctx)
		if err != nil {
			return out, err
		}
	}
	operation, _ := ctx.Value(probeOperationKey{}).(string)
	if operation == "" {
		operation = uuid.NewString()
	}
	labels := map[string]string{ProbeLabel: "probe", ProbeOperationLabel: operation, libarcane.InternalResourceLabel: "true"}
	config := &container.Config{Image: image, Cmd: args, Labels: labels, NetworkDisabled: target == ""}
	host := &container.HostConfig{ReadonlyRootfs: true, CapDrop: []string{"ALL"}, SecurityOpt: []string{"no-new-privileges:true"}}
	if target != "" {
		host.NetworkMode = container.NetworkMode("container:" + target)
	} else {
		host.NetworkMode = "none"
	}
	created, err := cli.ContainerCreate(ctx, client.ContainerCreateOptions{Config: config, HostConfig: host})
	defer func() {
		if removeErr := cleanupProbe(ctx, cli, created.ID, operation); removeErr != nil {
			err = &volumehelper.DockerError{Err: errors.Join(err, removeErr)}
		}
	}()
	if err != nil {
		return out, err
	}
	if created.ID == "" {
		return out, errors.New("probe helper create returned no ID")
	}
	if _, err = cli.ContainerStart(ctx, created.ID, client.ContainerStartOptions{}); err != nil {
		return out, err
	}
	wait := cli.ContainerWait(ctx, created.ID, client.ContainerWaitOptions{Condition: container.WaitConditionNotRunning})
	select {
	case <-ctx.Done():
		return out, ctx.Err()
	case err = <-wait.Error:
		if err == nil {
			err = errors.New("probe helper wait returned no status")
		}
		return out, err
	case status := <-wait.Result:
		if status.Error != nil && status.Error.Message != "" {
			return out, fmt.Errorf("probe helper wait failed")
		}
		out.ExitCode = int(status.StatusCode)
	}
	logs, err := cli.ContainerLogs(ctx, created.ID, client.ContainerLogsOptions{ShowStdout: true, ShowStderr: true})
	if err != nil {
		return out, err
	}
	defer logs.Close()
	stop := context.AfterFunc(ctx, func() { _ = logs.Close() })
	defer stop()
	stdout, stderr := &boundedBuffer{limit: MaxProbeJSON}, &boundedBuffer{limit: MaxProbeStderr}
	if err = collectProbeLogs(logs, stdout, stderr); err != nil {
		return out, err
	}
	out.Stdout, out.Stderr = stdout.Bytes(), stderr.Bytes()
	return out, nil
}

// Sweep removes only this module's labelled abandoned helpers at agent startup.
func (r *ProbeRunner) Sweep(ctx context.Context) error {
	clean, cancel := context.WithTimeout(context.WithoutCancel(ctx), ProbeCleanupBudget)
	defer cancel()
	cli := r.Client
	if r.Docker != nil {
		var err error
		cli, err = r.Docker(clean)
		if err != nil {
			return err
		}
	}
	rows, err := cli.ContainerList(clean, client.ContainerListOptions{All: true, Filters: client.Filters{}.Add("label", ProbeLabel+"=probe")})
	if err != nil {
		return err
	}
	for _, row := range rows.Items {
		if row.Labels[ProbeLabel] != "probe" {
			continue
		}
		if _, err := cli.ContainerRemove(clean, row.ID, client.ContainerRemoveOptions{Force: true}); err != nil && !errdefs.IsNotFound(err) {
			return err
		}
	}
	return nil
}

// Read the Docker frame header before any allocation. An oversized frame cannot
// make the generic demultiplexer allocate an unbounded buffer first.
func collectProbeLogs(reader io.Reader, stdout, stderr *boundedBuffer) error {
	for {
		var header [8]byte
		_, err := io.ReadFull(reader, header[:])
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		var output *boundedBuffer
		switch header[0] {
		case 1:
			output = stdout
		case 2:
			output = stderr
		default:
			return dto.Fail(dto.NotObservable, 409)
		}
		size := int64(binary.BigEndian.Uint32(header[4:]))
		if size > int64(output.limit-output.Len()) {
			return dto.Fail(dto.NotObservable, 409)
		}
		if _, err = io.CopyN(output, reader, size); err != nil {
			return err
		}
	}
}

type probeOperationKey struct{}

func WithProbeOperation(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, probeOperationKey{}, id)
}
func ProbeOperation(ctx context.Context) string {
	id, _ := ctx.Value(probeOperationKey{}).(string)
	return id
}

type ProbeFailure struct{ Err error }

func (e *ProbeFailure) Error() string { return "probe helper infrastructure failure: " + e.Err.Error() }
func (e *ProbeFailure) Unwrap() error { return e.Err }
func ProbeTransient(err error) bool   { var failure *ProbeFailure; return errors.As(err, &failure) }
func probeInfrastructure(err error) bool {
	var observed *dto.Problem
	return err != nil && !errors.As(err, &observed)
}

func cleanupProbe(ctx context.Context, cli *client.Client, id, operation string) error {
	clean, cancel := context.WithTimeout(context.WithoutCancel(ctx), ProbeCleanupBudget)
	defer cancel()
	ids := []string{id}
	if id == "" {
		// A cancelled create may have taken effect before its response was lost.
		rows, err := cli.ContainerList(clean, client.ContainerListOptions{All: true, Filters: client.Filters{}.Add("label", ProbeLabel+"=probe", ProbeOperationLabel+"="+operation)})
		if err != nil {
			return err
		}
		ids = nil
		for _, row := range rows.Items {
			if row.Labels[ProbeLabel] == "probe" && row.Labels[ProbeOperationLabel] == operation {
				ids = append(ids, row.ID)
			}
		}
	}
	for _, id := range ids {
		if _, err := cli.ContainerRemove(clean, id, client.ContainerRemoveOptions{Force: true}); err != nil && !errdefs.IsNotFound(err) {
			return err
		}
	}
	return nil
}
