// Source: docklet-next/issues/445
// Revision: db966bd5e74458ea032d70eba1b7e4593c7b104e
// Retrieved: 2026-11-14
// License: Apache-2.0
// Deviations: Original local and delivered registry revision invalidation regressions.

package registry

import (
	"net/http"
	"testing"

	"github.com/florianfinn/docklet-next/types/containerregistry"
	"github.com/stretchr/testify/require"
	"uuid"
)

func TestRegistryCredentialRevisionCRUDAndSync(t *testing.T) {
	db := setupContainerRegistryTestDBInternal(t)
	s := NewContainerRegistryService(db, nil, nil, nil, http.DefaultClient)
	require.Zero(t, s.CredentialRevision())
	reg, err := s.CreateRegistry(t.Context(), containerregistry.CreateContainerRegistryRequest{URL: "https://registry.example.com", RegistryType: "generic", Username: "example", Token: uuid.New().String(), Enabled: new(true)})
	require.NoError(t, err)
	require.Equal(t, uint64(1), s.CredentialRevision(), "create advances revision")
	_, err = s.UpdateRegistry(t.Context(), reg.ID, containerregistry.UpdateContainerRegistryRequest{Username: new("replacement")})
	require.NoError(t, err)
	require.Equal(t, uint64(2), s.CredentialRevision(), "update advances revision")
	_, err = s.UpdateRegistry(t.Context(), reg.ID, containerregistry.UpdateContainerRegistryRequest{ClearCredential: true})
	require.NoError(t, err)
	require.Equal(t, uint64(3), s.CredentialRevision(), "clearing credentials advances revision")
	require.NoError(t, s.SyncRegistries(t.Context(), []containerregistry.Sync{{ID: reg.ID, URL: reg.URL, RegistryType: "generic", Enabled: true}}))
	require.Equal(t, uint64(4), s.CredentialRevision(), "agent delivery advances revision")
	require.NoError(t, s.DeleteRegistry(t.Context(), reg.ID))
	require.Equal(t, uint64(5), s.CredentialRevision(), "delete advances revision")
	_, err = s.UpdateRegistry(t.Context(), "missing", containerregistry.UpdateContainerRegistryRequest{})
	require.Error(t, err)
	require.Equal(t, uint64(5), s.CredentialRevision(), "failed update changes no config")
}
