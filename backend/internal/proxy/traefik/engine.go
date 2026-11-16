// Source: docklet-next/issues/473
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-16
// License: Apache-2.0
// Deviations: Original Traefik v3 engine.

package traefik

import (
	"context"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/proxy"
	"github.com/florianfinn/docklet-next/backend/internal/proxy/hostio"
	dto "github.com/florianfinn/docklet-next/types/proxy"
	"github.com/moby/moby/api/types/container"
)

type runtime interface {
	InspectContainer(context.Context, dto.Instance) (container.InspectResponse, error)
	Copy(context.Context, dto.Instance, string, bool) ([]dto.File, []dto.OpaqueFile, error)
	CopyHeaders(context.Context, dto.Instance, string, bool, func(hostio.FileHeader) error) ([]dto.File, []dto.OpaqueFile, error)
	Target(context.Context, dto.Instance, string) (proxy.Bytes, error)
	Exec(context.Context, dto.Instance, string, string, string, string) (hostio.ExecResult, error)
	Logs(context.Context, dto.Instance, time.Time, func(string) error) error
	Writer(context.Context, dto.Instance) (hostio.FileWriter, error)
	LastVerified(context.Context, dto.Instance, string) (proxy.Bytes, bool, error)
}
type engine struct {
	run        runtime
	containers func(context.Context) ([]container.Summary, error)
	project    func(context.Context, string) (string, error)
	now        func() time.Time
	wait       func(context.Context, time.Duration) error
	limit      func(context.Context, time.Duration) (context.Context, context.CancelFunc)
}

// New constructs the file-provider engine. Register it before Module.Start.
func New(d *proxy.DockerRuntime) proxy.Engine {
	e := newEngine(d)
	e.containers = d.Docker.ListContainers
	e.project = func(ctx context.Context, name string) (string, error) {
		var rows []struct{ ID string }
		err := d.DB.WithContext(ctx).Table("projects").Select("id").Where("COALESCE(compose_project_name, name) = ? AND is_archived = ?", name, false).Limit(2).Scan(&rows).Error
		if err != nil {
			return "", err
		}
		if len(rows) == 1 {
			return rows[0].ID, nil
		}
		return "", nil
	}
	return e
}
func newEngine(r runtime) *engine {
	return &engine{run: r, now: time.Now, limit: context.WithTimeout, wait: func(ctx context.Context, d time.Duration) error {
		t := time.NewTimer(d)
		defer t.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
			return nil
		}
	}}
}
func (*engine) Reload(context.Context, proxy.Intent) error  { return nil }
func (*engine) Restart(context.Context, proxy.Intent) error { return dto.Fail(dto.InvalidInput, 422) }

// Runtime removes its named marker and temporary paths after this callback.
func (*engine) Cleanup(context.Context, proxy.Intent) error { return nil }
