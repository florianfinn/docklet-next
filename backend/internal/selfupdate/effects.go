// Source: docklet-next/issues/439
// Revision: 910032a54830f5ce1d37070964fa2dc7efb8aa2f
// Retrieved: 2026-11-09
// License: Apache-2.0
// Deviations: Original Compose-faithful self-update implementation.

package selfupdate

import (
	"context"
	"sync/atomic"

	"github.com/moby/moby/client"
)

type preEffectError struct{ error }

// Mutation is recorded before sending: a lost reply after a create/stop/start
// cannot authorize restoring env bytes or replaying a swap.
type effectClient struct {
	client.APIClient
	started atomic.Bool
}

func (c *effectClient) ContainerCreate(ctx context.Context, options client.ContainerCreateOptions) (client.ContainerCreateResult, error) {
	c.started.Store(true)
	return c.APIClient.ContainerCreate(ctx, options)
}
func (c *effectClient) ContainerStart(ctx context.Context, id string, options client.ContainerStartOptions) (client.ContainerStartResult, error) {
	c.started.Store(true)
	return c.APIClient.ContainerStart(ctx, id, options)
}
func (c *effectClient) ContainerStop(ctx context.Context, id string, options client.ContainerStopOptions) (client.ContainerStopResult, error) {
	c.started.Store(true)
	return c.APIClient.ContainerStop(ctx, id, options)
}
func (c *effectClient) ContainerRemove(ctx context.Context, id string, options client.ContainerRemoveOptions) (client.ContainerRemoveResult, error) {
	c.started.Store(true)
	return c.APIClient.ContainerRemove(ctx, id, options)
}
func (c *effectClient) ContainerRename(ctx context.Context, id string, options client.ContainerRenameOptions) (client.ContainerRenameResult, error) {
	c.started.Store(true)
	return c.APIClient.ContainerRename(ctx, id, options)
}
