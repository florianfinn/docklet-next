// Source: docklet-next/docs/design/runtime-control.md
// Revision: d4229b124f079376c959142b63235f6a739be2a9711e0228ec5a63ab552f8cf0
// Retrieved: 2026-11-20
// License: Apache-2.0
// Deviations: Original peer identity confirmation contract for issue 531.

package activity

import (
	"context"
	"strings"

	imageDTO "github.com/florianfinn/docklet-next/types/image"
)

// Acknowledged resolvers consume the explicit admitted digest at every fresh
// identity seam. Legacy ports remain usable only for unconfirmed intents.
type RuntimeAcknowledgedResolver interface {
	ResolveAcknowledgedRuntimeTarget(context.Context, RuntimeTarget, string) (RuntimeResolvedTarget, error)
}
type RuntimeAcknowledgedResolverFunc func(context.Context, RuntimeTarget, string) (RuntimeResolvedTarget, error)

func (fn RuntimeAcknowledgedResolverFunc) ResolveRuntimeTarget(ctx context.Context, target RuntimeTarget) (RuntimeResolvedTarget, error) {
	return fn(ctx, target, "")
}
func (fn RuntimeAcknowledgedResolverFunc) ResolveAcknowledgedRuntimeTarget(ctx context.Context, target RuntimeTarget, digest string) (RuntimeResolvedTarget, error) {
	return fn(ctx, target, digest)
}
func resolveRuntimeAcknowledged(ctx context.Context, resolver RuntimeTargetResolver, target RuntimeTarget, digest string) (RuntimeResolvedTarget, error) {
	if resolver, ok := resolver.(RuntimeAcknowledgedResolver); ok {
		return resolver.ResolveAcknowledgedRuntimeTarget(ctx, target, digest)
	}
	if digest != "" {
		return RuntimeResolvedTarget{}, imageDTO.Failure("runtime.invalid-acknowledgement", 400)
	}
	return resolver.ResolveRuntimeTarget(ctx, target)
}
func runtimeAcknowledgement(value string) (string, error) {
	if value != "" && !imageDTO.ValidRuntimeUncertaintyDigest(value) {
		return "", imageDTO.Failure("runtime.invalid-acknowledgement", 400)
	}
	return strings.ToLower(value), nil
}
