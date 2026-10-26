// Source: docklet-next/docs/design/update-policies.md
// Revision: cdfe93c3dd2557cb21586ae542ebc05cae9bc0af
// Retrieved: 2026-10-26
// License: Apache-2.0
// Deviations: Public and authenticated version reads have separate reference visibility.

package version

import (
	"encoding/json"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/volumehelper"
	dto "github.com/florianfinn/docklet-next/types/version"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestSystemImageReferencesAreNotInPublicVersionReads(t *testing.T) {
	service := &VersionService{version: "2.0.0", disabled: true}
	handler := &VersionHandler{versionService: service}
	public, err := handler.GetAppVersion(t.Context(), &GetAppVersionInput{})
	require.NoError(t, err)
	payload, err := json.Marshal(public.Body)
	require.NoError(t, err)
	require.NotContains(t, string(payload), "toolsImage")
	require.NotContains(t, string(payload), "currentImageRef")
	trusted, err := handler.GetAppVersion(middleware.WithAgentCredential(t.Context()), &GetAppVersionInput{})
	require.NoError(t, err)
	require.Equal(t, volumehelper.DefaultToolsImage, trusted.Body.ToolsImage)
	private := &dto.Info{CurrentImageRef: "registry.example.com/private/agent:2.0.0", ToolsImage: "registry.example.com/private/tools:0.1.0", ToolsImageOverride: true}
	projected := publicVersionInfo(private)
	payload, err = json.Marshal(projected)
	require.NoError(t, err)
	require.NotContains(t, string(payload), "private")
	require.NotEmpty(t, private.CurrentImageRef, "public projection must not mutate the authenticated source")
}
