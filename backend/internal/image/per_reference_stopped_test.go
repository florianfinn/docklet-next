// Source: docklet-next/issues/412
// Revision: 6e3afa6a78f4f60ffd74858d9a1eaefbb1b73bda
// Retrieved: 2026-11-12
// License: Apache-2.0
// Deviations: Original stopped digest consumer coverage for per-reference observations.

package image_test

import (
	"strings"
	"testing"

	imageService "github.com/florianfinn/docklet-next/backend/internal/image"
	"github.com/florianfinn/docklet-next/backend/internal/registry"
	"github.com/moby/moby/api/types/container"
	"github.com/stretchr/testify/require"
)

// A stopped digest container sharing its reference with a running sibling is
// shown with an update by the reader, so its update check must not refuse.
func TestPerReferenceStoppedDigestConsumerCanUpdate(t *testing.T) {
	f := newReferenceFixture(t)
	f.containers[2].State = "exited"
	result, err := f.checker().ReadUpdateInfo(t.Context(), f.refs[0])
	require.NoError(t, err)
	require.NotNil(t, result.ContainerUpdates["replica"])
	updates, err := f.reader().GetUpdateInfoByContainers(t.Context(), f.containers)
	require.NoError(t, err)
	require.True(t, updates["replica"].HasUpdate)
	svc := imageService.NewImageService(f.db, f.docker, registry.NewContainerRegistryService(f.db, nil, nil, nil), f.checker(), nil, nil, nil, nil)
	stopped := f.containers[2]
	checked, err := svc.CheckContainerImage(t.Context(), container.InspectResponse{ID: stopped.ID, Image: stopped.ImageID, Config: &container.Config{Image: stopped.Image, Labels: stopped.Labels}})
	require.NoError(t, err, "the action agrees with the reader")
	require.True(t, checked.HasUpdate)
}

// A scoped row of a stopped container is refreshed by later checks; it never
// freezes at an older failure or success.
func TestPerReferenceStoppedDigestRowFollowsRechecks(t *testing.T) {
	f := newReferenceFixture(t)
	_, err := f.checker().ReadUpdateInfo(t.Context(), f.refs[0])
	require.NoError(t, err)
	f.containers[2].State = "exited"
	f.denied.Store(true)
	_, err = f.checker().ReadUpdateInfo(t.Context(), f.refs[0])
	require.NoError(t, err)
	f.denied.Store(false)
	_, err = f.checker().ReadUpdateInfo(t.Context(), f.refs[0])
	require.NoError(t, err)
	updates, err := f.reader().GetUpdateInfoByContainers(t.Context(), f.containers)
	require.NoError(t, err)
	require.Equal(t, "version", updates["first"].State)
	require.Equal(t, updates["first"].State, updates["replica"].State)
	require.Equal(t, updates["first"].HasUpdate, updates["replica"].HasUpdate)
}

// An identity-fenced consumer is never actionable, whether it was never checked
// or had an earlier success. States follow the consumer contract shared with
// tag-policy consumers: the response is unknown, the stored row a failed check.
func TestPerReferenceStoppedIdentityFence(t *testing.T) {
	for _, after := range []bool{false, true} {
		t.Run(map[bool]string{false: "initial", true: "changed-after-success"}[after], func(t *testing.T) {
			f := newReferenceFixture(t)
			f.containers[2].State = "exited"
			if after {
				_, err := f.checker().ReadUpdateInfo(t.Context(), f.refs[0])
				require.NoError(t, err)
			}
			f.containers[2].ImageID = "sha256:" + strings.Repeat("c", 64)
			result, err := f.checker().ReadUpdateInfo(t.Context(), f.refs[0])
			require.NoError(t, err)
			require.Equal(t, "unknown", result.ContainerUpdates["replica"].State)
			require.False(t, result.ContainerUpdates["replica"].HasUpdate)
			updates, err := f.reader().GetUpdateInfoByContainers(t.Context(), f.containers)
			require.NoError(t, err)
			require.Equal(t, "error", updates["replica"].State)
			require.False(t, updates["replica"].HasUpdate)
			svc := imageService.NewImageService(f.db, f.docker, registry.NewContainerRegistryService(f.db, nil, nil, nil), f.checker(), nil, nil, nil, nil)
			stopped := f.containers[2]
			checked, err := svc.CheckContainerImage(t.Context(), container.InspectResponse{ID: stopped.ID, Image: stopped.ImageID, Config: &container.Config{Image: stopped.Image, Labels: stopped.Labels}})
			require.NoError(t, err)
			require.Equal(t, "unknown", checked.State)
			require.False(t, checked.HasUpdate)
		})
	}
}

// A stopped consumer's initial failure survives a later check of another
// reference that rewrites the shared image-level row.
func TestPerReferenceStoppedInitialFailureAcrossReferences(t *testing.T) {
	f := newReferenceFixture(t)
	f.containers[2].State = "exited"
	f.denied.Store(true)
	_, err := f.checker().ReadMultiple(t.Context(), f.refs)
	require.NoError(t, err)
	updates, err := f.reader().GetUpdateInfoByContainers(t.Context(), f.containers)
	require.NoError(t, err)
	require.NotNil(t, updates["replica"])
	require.Equal(t, "error", updates["replica"].State)
	require.False(t, updates["replica"].HasUpdate)
}
