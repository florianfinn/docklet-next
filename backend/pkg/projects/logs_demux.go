// Source: getarcaneapp/arcane/backend/pkg/projects/logs_demux.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Bound source frames and lines before Compose buffering; preserve
// producer metadata through the optional in-process log sink; fail on missing inspect metadata.

package projects

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"strings"
	"sync"

	"github.com/docker/cli/cli/command"
	"github.com/moby/moby/api/pkg/stdcopy"
	"github.com/moby/moby/client"
	"go.getarcane.app/docker"
	"go.getarcane.app/docker/compat"
)

// logsDemuxCompatibleDockerCliInternal exposes an API client that keeps
// stderr metadata alive in non-TTY container log streams. Used only by
// ComposeLogs; other compose operations keep the unmodified client.
type logsDemuxCompatibleDockerCliInternal struct {
	command.Cli

	apiClient client.APIClient
}

func (c *logsDemuxCompatibleDockerCliInternal) Client() client.APIClient {
	return c.apiClient
}

func wrapDockerCLIWithLogsDemuxInternal(cli command.Cli) command.Cli {
	if cli == nil {
		return nil
	}
	return &logsDemuxCompatibleDockerCliInternal{
		Cli:       cli,
		apiClient: &logsDemuxDockerClientInternal{APIClient: compat.WrapDockerAPIClientForInspectCompatibility(cli.Client())},
	}
}

// logsDemuxDockerClientInternal records each container's TTY state from the
// inspect calls Compose already performs, and reworks non-TTY multiplexed log
// responses so stderr entries stay identifiable: stderr lines are prefixed
// with Arcane's stderr marker and every entry is re-framed as a stdout
// stdcopy frame, which Compose merges through its stdout consumer path. TTY
// lines use the same bound and retain their combined-stream meaning.
type logsDemuxDockerClientInternal struct {
	client.APIClient

	ttyMu            sync.RWMutex
	containerTty     map[string]bool
	containerSources map[string]string
}

func (c *logsDemuxDockerClientInternal) ContainerInspect(ctx context.Context, containerID string, options client.ContainerInspectOptions) (client.ContainerInspectResult, error) {
	result, err := c.APIClient.ContainerInspect(ctx, containerID, options)
	if err != nil || result.Container.Config == nil {
		key := strings.ToLower(strings.TrimSpace(containerID))
		c.ttyMu.Lock()
		delete(c.containerTty, key)
		delete(c.containerSources, key)
		c.ttyMu.Unlock()
		if err == nil {
			err = errors.New("container log metadata unavailable: missing inspect config")
		}
		return result, err
	}
	c.recordContainerTTYInternal(containerID, result.Container.Config.Tty)
	source := result.Container.Config.Labels["com.docker.compose.service"]
	if source == "" {
		source = strings.TrimPrefix(result.Container.Name, "/")
	}
	if source == "" {
		source = containerID
	}
	c.ttyMu.Lock()
	if c.containerSources == nil {
		c.containerSources = make(map[string]string)
	}
	c.containerSources[strings.ToLower(strings.TrimSpace(containerID))] = source
	c.ttyMu.Unlock()
	return result, err
}

func (c *logsDemuxDockerClientInternal) ContainerLogs(ctx context.Context, containerID string, options client.ContainerLogsOptions) (client.ContainerLogsResult, error) {
	tty, known := c.containerTTYInternal(containerID)
	if !known {
		// Compose inspects the same full ID before opening logs. Never guess TTY or
		// pass an unbounded body to its text consumer when that metadata is absent.
		return nil, errors.New("container log metadata unavailable: inspect required")
	}
	resp, err := c.APIClient.ContainerLogs(ctx, containerID, options)
	if err != nil {
		return resp, err
	}
	c.ttyMu.RLock()
	service := c.containerSources[strings.ToLower(strings.TrimSpace(containerID))]
	c.ttyMu.RUnlock()
	return &stderrMarkedDemuxedStreamInternal{upstream: resp, records: docker.NewLogReader(resp, tty), ctx: ctx, tty: tty, sink: docker.LogSinkFromContext(ctx), service: service}, nil
}

func (c *logsDemuxDockerClientInternal) recordContainerTTYInternal(containerID string, tty bool) {
	key := strings.ToLower(strings.TrimSpace(containerID))
	c.ttyMu.Lock()
	defer c.ttyMu.Unlock()
	if c.containerTty == nil {
		c.containerTty = make(map[string]bool, 8)
	}
	c.containerTty[key] = tty
}

func (c *logsDemuxDockerClientInternal) containerTTYInternal(containerID string) (tty, known bool) {
	key := strings.ToLower(strings.TrimSpace(containerID))
	c.ttyMu.RLock()
	defer c.ttyMu.RUnlock()
	tty, known = c.containerTty[key]
	return tty, known
}

// One record is emitted per read refill. The decoder never allocates by the
// advertised frame size, and both pending source prefixes have a fixed bound.
const maxLogEntryBytesInternal = docker.MaxLogLineBytes

type stderrMarkedDemuxedStreamInternal struct {
	upstream io.ReadCloser
	records  *docker.LogReader
	ctx      context.Context
	tty      bool
	sink     docker.LogSink
	service  string
	out      bytes.Buffer
}

func (s *stderrMarkedDemuxedStreamInternal) Read(p []byte) (int, error) {
	if s.out.Len() == 0 {
		record, err := s.records.Next(s.ctx)
		if err != nil {
			return 0, err
		}
		record.Service = s.service
		if s.sink != nil {
			if err = s.sink(record); err != nil {
				return 0, err
			}
		}
		text := docker.LogText(record)
		if !s.tty {
			var header [8]byte
			header[0] = byte(stdcopy.Stdout)
			binary.BigEndian.PutUint32(header[4:], uint32(len(text)+1)) //nolint:gosec // bounded log text plus newline
			s.out.Write(header[:])
		}
		s.out.WriteString(text)
		s.out.WriteByte('\n')
	}
	return s.out.Read(p)
}
func (s *stderrMarkedDemuxedStreamInternal) Close() error { return s.upstream.Close() }
