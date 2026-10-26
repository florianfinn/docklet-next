// Source: docklet-next/docs/design/images-updates-identity.md
// Revision: fcf8c8d3a079154c299912a7e22e0e903e5e4e38
// Retrieved: 2026-10-26
// License: Apache-2.0
// Deviations: Real producer role relations and source-derived classic evidence.

package identity

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	imageDTO "github.com/florianfinn/docklet-next/types/image"
	"github.com/moby/moby/api/types/image"
	"github.com/moby/moby/client"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/require"
)

func fixture(t *testing.T, name string, v any) {
	t.Helper()
	raw, e := os.ReadFile("testdata/" + name + ".json")
	require.NoError(t, e)
	require.NoError(t, json.Unmarshal(raw, v))
}
func daemon(t *testing.T, api, qualified string, manifests string) *client.Client {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/info") {
			_, _ = w.Write([]byte(`{"OSType":"linux","Architecture":"x86_64"}`))
			return
		}
		name := qualified
		if r.URL.Query().Get("manifests") == "1" {
			name = manifests
		}
		b, e := os.ReadFile("testdata/" + name + ".json")
		if e != nil {
			w.WriteHeader(404)
			return
		}
		_, _ = w.Write(b)
	}))
	t.Cleanup(server.Close)
	c, e := client.New(client.WithHost(server.URL), client.WithAPIVersion(api), client.WithHTTPClient(server.Client()))
	require.NoError(t, e)
	t.Cleanup(func() { _ = c.Close() })
	return c
}
func TestIdentityEvidenceStores(t *testing.T) {
	for _, scenario := range []struct{ name, qualified, p, api string }{
		{"classic", "classic", "linux/amd64", "1.48"},
		{"containerd-index", "containerd-qualified", "linux/amd64", "1.49"},
		{"containerd-pulled", "containerd-pulled-qualified", "linux/arm/v7", "1.49"},
		{"containerd-index", "containerd-qualified", "linux/amd64", "1.48"},
	} {
		t.Run(scenario.name+scenario.api, func(t *testing.T) {
			var img image.InspectResponse
			fixture(t, scenario.name, &img)
			c := daemon(t, scenario.api, scenario.qualified, "manifests")
			local, e := Resolve(t.Context(), c, img, scenario.p, nil)
			require.NoError(t, e)
			require.Equal(t, img.ID, local.ID.String())
			require.Equal(t, scenario.p, local.Platform.String())
			if scenario.name == "classic" {
				require.Empty(t, local.Digest())
				require.Equal(t, img.ID, local.Key.String())
			} else {
				var qualified image.InspectResponse
				fixture(t, scenario.qualified, &qualified)
				require.Equal(t, qualified.ID, local.Key.String())
				require.Equal(t, qualified.ID, local.Digest(), "a direct-manifest Descriptor still denotes containerd")
			}
		})
	}
}
func TestIdentityQualifiedIDRoles(t *testing.T) {
	var img image.InspectResponse
	fixture(t, "containerd-index", &img)
	c := daemon(t, "1.49", "containerd-qualified", "manifests")
	local, e := Resolve(t.Context(), c, img, "", nil)
	require.NoError(t, e)
	require.Equal(t, img.ID, local.ID.String())
	require.NotEqual(t, local.ID.String(), local.Key.String())
	require.Equal(t, "linux/amd64", local.Platform.String())
	var container struct {
		Image                   string
		ImageManifestDescriptor *ocispec.Descriptor
	}
	fixture(t, "pulled-container", &container)
	fixture(t, "containerd-pulled", &img)
	local, e = Resolve(t.Context(), c, img, "", container.ImageManifestDescriptor)
	require.NoError(t, e)
	require.Equal(t, container.Image, local.ID.String())
	require.Equal(t, "linux/arm/v7", local.Platform.String())
	require.Equal(t, container.ImageManifestDescriptor.Digest.String(), local.Key.String())
}
func TestIdentityEvidenceUnavailable(t *testing.T) {
	var img image.InspectResponse
	fixture(t, "containerd-index", &img)
	_, e := Resolve(t.Context(), daemon(t, "1.47", "containerd-qualified", "manifests"), img, "linux/amd64", nil)
	require.Error(t, e)
	_, e = Resolve(t.Context(), daemon(t, "1.48", "containerd-qualified", "manifests"), img, "linux/s390x", nil)
	require.Error(t, e)
	img.Descriptor.Digest = "sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
	_, e = Resolve(t.Context(), daemon(t, "1.49", "containerd-qualified", "manifests"), img, "linux/amd64", nil)
	require.Error(t, e)
}
func TestIdentityRebuildStores(t *testing.T) {
	for _, name := range []string{"classic", "containerd-index", "containerd-pulled"} {
		t.Run(name, func(t *testing.T) {
			var img image.InspectResponse
			fixture(t, name, &img)
			p, qualified := "linux/amd64", "containerd-qualified"
			if name == "containerd-pulled" {
				p = "linux/arm/v7"
				qualified = "containerd-pulled-qualified"
			}
			l, e := Resolve(t.Context(), daemon(t, "1.49", qualified, "manifests"), img, p, nil)
			require.NoError(t, e)
			// The same config republished under a different manifest is only a rebuild
			// under containerd; expected local ID is predicted before any pull.
			manifest := "sha256:" + strings.Repeat("b", 64)
			config := l.Key.String()
			r, e := NewCandidate(manifest, config, l.Platform)
			require.NoError(t, e)
			key, e := l.CandidateKey(r)
			require.NoError(t, e)
			expected, e := l.Expected(r)
			require.NoError(t, e)
			if name == "classic" {
				require.Equal(t, l.Key, key)
				require.True(t, MatchesExpected(l.ID, expected))
			} else {
				require.NotEqual(t, l.Key, key)
				require.Equal(t, manifest, expected.String())
			}
		})
	}
}
func TestIdentityNamedRoleTypes(t *testing.T) {
	// Inspect the actual boundary declarations, then compile a comparison in a
	// scratch package. The suite itself compiles even if roles become aliases.
	f, e := parser.ParseFile(token.NewFileSet(), "types.go", nil, 0)
	require.NoError(t, e)
	roles := map[string]bool{}
	for _, decl := range f.Decls {
		if g, ok := decl.(*ast.GenDecl); ok {
			for _, spec := range g.Specs {
				if s, ok := spec.(*ast.TypeSpec); ok {
					roles[s.Name.Name] = !s.Assign.IsValid()
				}
			}
		}
	}
	for _, name := range []string{"LocalID", "NativeKey", "Platform", "RegistryConfig", "RegistryManifest", "RegistryCandidate", "ExpectedLocalID", "TopLevelDigest"} {
		require.True(t, roles[name], name)
	}
	fs := token.NewFileSet()
	snippet, e := parser.ParseFile(fs, "roles.go", `package roles;type LocalID struct{v string};type ExpectedLocalID struct{v string};var l LocalID;var e ExpectedLocalID;var _ = l == e`, 0)
	require.NoError(t, e)
	_, e = (&types.Config{}).Check("roles", fs, []*ast.File{snippet}, nil)
	require.ErrorContains(t, e, "mismatched types")
}

func TestIdentityImageOnlyDefault(t *testing.T) {
	var img, manifest image.InspectResponse
	fixture(t, "containerd-index", &img)
	fixture(t, "manifests", &manifest)
	var pulled struct{ ImageManifestDescriptor *ocispec.Descriptor }
	fixture(t, "pulled-container", &pulled)
	// Start with the real available AMD64 child, then put the separately captured
	// available ARM child first. Array order must not select the image platform.
	var amd image.ManifestSummary
	for _, child := range manifest.Manifests {
		if child.Available && child.Kind == "image" {
			amd = child
			break
		}
	}
	arm := amd
	arm.Descriptor = *pulled.ImageManifestDescriptor
	arm.ImageData = &image.ImageProperties{Platform: *pulled.ImageManifestDescriptor.Platform}
	manifest.Manifests = []image.ManifestSummary{arm, amd}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/info") {
			_, _ = w.Write([]byte(`{"OSType":"linux","Architecture":"x86_64"}`))
			return
		}
		_ = json.NewEncoder(w).Encode(manifest)
	}))
	defer server.Close()
	c, e := client.New(client.WithHost(server.URL), client.WithHTTPClient(server.Client()), client.WithAPIVersion("1.48"))
	require.NoError(t, e)
	defer c.Close()
	local, e := Resolve(t.Context(), c, img, "", nil)
	require.NoError(t, e)
	require.Equal(t, "linux/amd64", local.Platform.String())
	require.Equal(t, amd.Descriptor.Digest.String(), local.Key.String())
	manifest.Manifests = []image.ManifestSummary{arm, amd, amd}
	_, e = Resolve(t.Context(), c, img, "", nil)
	require.Error(t, e, "duplicate available children at P are ambiguous")
	manifest.Manifests = []image.ManifestSummary{arm}
	_, e = Resolve(t.Context(), c, img, "", nil)
	require.Error(t, e, "another available platform cannot stand in for the daemon default")
}

func TestIdentityClassicImageDefault(t *testing.T) {
	var img image.InspectResponse
	fixture(t, "classic", &img)
	c := daemon(t, "1.48", "classic", "manifests")
	_, e := Resolve(t.Context(), c, img, "", nil)
	require.NoError(t, e)
	img.Architecture = "arm"
	img.Variant = "v7"
	_, e = Resolve(t.Context(), c, img, "", nil)
	require.Error(t, e, "image-level reads require the daemon default")
	current, e := ResolveContainer(t.Context(), c, img, "", nil)
	require.NoError(t, e)
	require.Equal(t, "linux/arm/v7", current.Platform.String())
}

func TestIdentityNativeDescriptorValidation(t *testing.T) {
	var img image.InspectResponse
	fixture(t, "containerd-index", &img)
	c := daemon(t, "1.49", "containerd-index", "manifests")
	_, e := Resolve(t.Context(), c, img, "linux/amd64", nil)
	require.Error(t, e, "a qualified response which still describes an index is not a native platform manifest")
	var descriptor struct{ ImageManifestDescriptor *ocispec.Descriptor }
	fixture(t, "pulled-container", &descriptor)
	descriptor.ImageManifestDescriptor.MediaType = ocispec.MediaTypeImageIndex
	_, e = Resolve(t.Context(), c, img, "", descriptor.ImageManifestDescriptor)
	require.Error(t, e, "container provenance also needs a manifest descriptor")
}

func TestIdentityDescriptorProjection(t *testing.T) {
	for _, name := range []string{"classic", "containerd-index", "containerd-pulled"} {
		var img image.InspectResponse
		fixture(t, name, &img)
		projected := imageDTO.NewDetailSummary(&img)
		if img.Descriptor == nil {
			require.Empty(t, projected.Descriptor.Digest)
			require.Empty(t, projected.Descriptor.MediaType)
			require.Zero(t, projected.Descriptor.Size)
		} else {
			require.Equal(t, img.Descriptor.Digest.String(), projected.Descriptor.Digest)
			require.Equal(t, img.Descriptor.MediaType, projected.Descriptor.MediaType)
			require.Equal(t, img.Descriptor.Size, projected.Descriptor.Size)
		}
	}
}

func TestIdentityRunningContainerProducer(t *testing.T) {
	var img image.InspectResponse
	fixture(t, "containerd-running", &img)
	var container struct {
		Image                   string
		ImageManifestDescriptor *ocispec.Descriptor
	}
	fixture(t, "container", &container)
	current, e := ResolveContainer(t.Context(), daemon(t, "1.49", "containerd-qualified", "manifests"), img, "", container.ImageManifestDescriptor)
	require.NoError(t, e)
	require.Equal(t, container.Image, current.ID.String())
	require.Equal(t, container.ImageManifestDescriptor.Digest.String(), current.Key.String())
	require.NotEqual(t, current.ID.String(), current.Key.String())
	require.Equal(t, "linux/amd64", current.Platform.String())
}
