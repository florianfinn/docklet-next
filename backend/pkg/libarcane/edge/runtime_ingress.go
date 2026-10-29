// Source: docklet-next/docs/design/runtime-control.md
// Revision: 230a4a3cb92c59cc15e1f6767abd18f017e05ca4b83835524aa8c20e50a89e46
// Retrieved: 2026-10-19
// License: Apache-2.0
// Deviations: M12 synchronous volume-backup cancellation reuses the existing request registry;  Bounded remote runtime implementation, active in production since #125.

package edge

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"
)

const runtimeRequestBodyLimit = 1 << 20
const runtimePendingLimit = 64

type runtimeMemoryBodyKeyInternal struct{}

func RuntimeIngressBodyBounded(ctx context.Context) bool {
	v, _ := ctx.Value(runtimeMemoryBodyKeyInternal{}).(bool)
	return v
}

// NewRuntimeTunnelClient enables the bounded runtime command vocabulary used
// by the production agent alongside its existing ordinary commands.
func NewRuntimeTunnelClient(cfg *Config, handler http.Handler) *TunnelClient {
	c := NewTunnelClient(cfg, handler)
	c.runtimeCommands = true
	return c
}
func runtimeRequestPathInternal(path string) bool {
	return path == "/api/environments/0/runtime-registration" || path == "/api/environments/0/runtime-target" || path == "/api/environments/0/runtime-runs" || strings.HasPrefix(path, "/api/environments/0/runtime-runs/")
}

// Only synchronous backup commands need request cancellation during preparation.
// Accepted backup/restore/test workers keep their independent activity lifetime.
func volumeBackupCancelableRequest(method, path string) bool {
	command, ok := ResolveEdgeCommandName(method, path, false).Get()
	if !ok {
		return false
	}
	switch command {
	case "volume.backup.download", "volume.backup.files_browse", "volume.backup.files", "volume.backup.has_path", "volume.backup.delete":
		return true
	}
	return false
}

func RuntimeCommandName(method, path string) (string, bool) {
	if method == http.MethodPost && path == "/api/environments/0/runtime-registration" {
		return "runtime.register", true
	}
	if method == http.MethodPost && path == "/api/environments/0/runtime-target" {
		return "runtime.resolve", true
	}
	base := "/api/environments/0/runtime-runs"
	if path == base && method == http.MethodPost {
		return "runtime.execute", true
	}
	if strings.HasPrefix(path, base+"/") {
		suffix := strings.TrimPrefix(path, base+"/")
		if !strings.Contains(suffix, "/") && suffix != "" && method == http.MethodGet {
			return "runtime.query", true
		}
		if strings.HasSuffix(suffix, "/cancel") && strings.Count(suffix, "/") == 1 && method == http.MethodPost {
			return "runtime.cancel", true
		}
	}
	return "", false
}
func runtimeCommandValidInternal(command, method, path string) bool {
	expected, ok := RuntimeCommandName(method, path)
	return ok && command == expected
}

type runtimeRequestIngressInternal struct {
	conn       TunnelConnection
	ctx        context.Context
	cancel     context.CancelFunc
	stop       func() bool
	transferID string
	running    bool
	expired    chan struct{}
	done       chan struct{}
	finished   sync.Once
}

func (c *TunnelClient) scheduleRuntimeRequestInternal(ctx context.Context, conn TunnelConnection, msg *TunnelMessage, workers *sync.WaitGroup, command bool) {
	volumeBackup := volumeBackupCancelableRequest(msg.Method, msg.Path)
	if !runtimeRequestPathInternal(msg.Path) && !volumeBackup {
		workers.Go(func() { c.handleRequest(ctx, conn, msg) })
		return
	}
	if (!c.runtimeCommands && !volumeBackup) || len(msg.Body) > runtimeRequestBodyLimit || msg.ID == "" {
		c.sendCommandCompleteInternal(conn, msg.ID, 400, "runtime ingress unavailable")
		return
	}
	timeout := min(c.requestTimeoutInternal(), 5*time.Minute)
	if msg.TimeoutMillis > 0 {
		timeout = min(timeout, time.Duration(min(msg.TimeoutMillis, int64(300000)))*time.Millisecond)
	}
	requestCtx, cancel := context.WithTimeout(ctx, timeout)
	entry := &runtimeRequestIngressInternal{expired: make(chan struct{}), done: make(chan struct{}), conn: conn, ctx: requestCtx, cancel: cancel, transferID: strings.TrimSpace(msg.Metadata[bodyTransferMetadataKey])}
	c.runtimeRequestMu.Lock()
	if c.runtimeRequests == nil {
		c.runtimeRequests = make(map[string]*runtimeRequestIngressInternal)
	}
	if len(c.runtimeRequests) >= runtimePendingLimit || c.runtimeRequests[msg.ID] != nil {
		c.runtimeRequestMu.Unlock()
		cancel()
		c.sendCommandCompleteInternal(conn, msg.ID, 503, "runtime ingress busy")
		return
	}
	c.runtimeRequests[msg.ID] = entry
	entry.stop = context.AfterFunc(requestCtx, func() { defer close(entry.expired); c.expireRuntimeRequestInternal(msg.ID, entry) })
	c.runtimeRequestMu.Unlock()
	if entry.transferID != "" && command {
		c.beginCommandRequestTransferInternal(requestCtx, conn, entry.transferID, msg)
		if requestCtx.Err() != nil {
			c.expireRuntimeRequestInternal(msg.ID, entry)
		}
		return
	}
	c.startRuntimeHandlerInternal(requestCtx, conn, msg, workers, command)
}
func (c *TunnelClient) startRuntimeHandlerInternal(ctx context.Context, conn TunnelConnection, msg *TunnelMessage, workers *sync.WaitGroup, command bool) {
	c.runtimeRequestMu.Lock()
	entry := c.runtimeRequests[msg.ID]
	if entry == nil || entry.conn != conn || entry.running || ctx.Err() != nil {
		c.runtimeRequestMu.Unlock()
		return
	}
	entry.running = true
	c.runtimeRequestMu.Unlock()
	workers.Go(func() {
		defer func() {
			if !entry.stop() {
				<-entry.expired
			}
			entry.cancel()
			c.runtimeRequestMu.Lock()
			if c.runtimeRequests[msg.ID] == entry {
				delete(c.runtimeRequests, msg.ID)
			}
			c.runtimeRequestMu.Unlock()
			entry.finished.Do(func() { close(entry.done) })
		}()
		if ctx.Err() != nil {
			return
		}
		if command {
			c.handleCommandRequest(ctx, conn, msg)
		} else {
			c.handleRequest(ctx, conn, msg)
		}
	})
}
func (c *TunnelClient) expireRuntimeRequestInternal(id string, entry *runtimeRequestIngressInternal) {
	c.runtimeRequestMu.Lock()
	defer c.runtimeRequestMu.Unlock()
	if entry.transferID != "" {
		if value, ok := c.requestTransfers.Load(entry.transferID); ok {
			if transfer, ok := value.(*commandRequestTransfer); ok && transfer.request.ID == id && transfer.conn == entry.conn && transfer.ctx == entry.ctx {
				if c.requestTransfers.CompareAndDelete(entry.transferID, transfer) {
					transfer.stopInternal()
				}
			}
		}
	}
	if !entry.running && c.runtimeRequests[id] == entry {
		delete(c.runtimeRequests, id)
		entry.finished.Do(func() { close(entry.done) })
	}
}
func (c *TunnelClient) cancelRuntimeRequestInternal(conn TunnelConnection, id string) {
	c.runtimeRequestMu.Lock()
	entry := c.runtimeRequests[id]
	c.runtimeRequestMu.Unlock()
	if entry != nil && entry.conn == conn {
		entry.cancel()
		c.expireRuntimeRequestInternal(id, entry)
	}
}
func (c *TunnelClient) cancelRuntimeSessionInternal(conn TunnelConnection) {
	c.runtimeRequestMu.Lock()
	entries := make(map[string]*runtimeRequestIngressInternal)
	for id, entry := range c.runtimeRequests {
		if entry.conn == conn {
			entries[id] = entry
		}
	}
	c.runtimeRequestMu.Unlock()
	for id, entry := range entries {
		entry.cancel()
		c.expireRuntimeRequestInternal(id, entry)
	}
}
