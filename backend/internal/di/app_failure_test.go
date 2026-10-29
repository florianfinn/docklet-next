// Source: docklet-next/issues/245
// Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original test for the Francis host failure callback.

package di

import (
	"context"
	"errors"
	"testing"
)

func TestActorHostFailureHandlerEndsApplication(t *testing.T) {
	original := errors.New("host is not registered")
	var got error
	actorHostFailureHandler(context.Background(), func(err error) { got = err })(original)
	if !errors.Is(got, original) {
		t.Fatalf("failApp received %v, want the host error", got)
	}
}
