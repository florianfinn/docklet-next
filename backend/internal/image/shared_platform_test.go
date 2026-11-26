// Source: docklet-next/issues/634
// Revision: 35f39e8729df29cabccc10a4adc42b4b68220a7c
// Retrieved: 2026-11-26
// License: Apache-2.0
// Deviations: Original platform scope of image-level observations lent to digest consumers and reference lookups.

package image

import (
	"strings"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/imageupdate"
	"github.com/florianfinn/docklet-next/backend/internal/testsqlite"
	"github.com/moby/moby/api/types/container"
	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// Before its first own check, a digest consumer borrows the image-level row.
// A containerd image ID names an index shared by all platforms, so build facts
// follow only a container proven on the observed platform.
func TestSharedIndexObservationKeepsBuildFactsOnItsPlatform(t *testing.T) {
	index := "sha256:" + strings.Repeat("d", 64)
	current := "sha256:" + strings.Repeat("a", 64)
	next := "sha256:" + strings.Repeat("b", 64)
	built := time.Date(2026, 11, 24, 6, 0, 0, 0, time.UTC)
	for _, store := range []string{"containerd", "classic"} {
		for _, running := range []string{"linux/amd64", "linux/arm64", ""} {
			t.Run(store+"/"+running, func(t *testing.T) {
				db, err := gorm.Open(testsqlite.Dialector(t, ":memory:"), &gorm.Config{})
				require.NoError(t, err)
				require.NoError(t, db.AutoMigrate(&imageupdate.ImageUpdateRecord{}))
				row := imageupdate.ImageUpdateRecord{ID: index, ImageID: index, Repository: "registry.example.com/app", Tag: "latest", CheckTime: time.Now().UTC(), CurrentDigest: &current, LatestDigest: &next, HasUpdate: true, Platform: new("native-v1:" + store + ":linux/amd64"), CurrentCreated: &built, LatestCreated: &built}
				require.NoError(t, db.Create(&row).Error)
				live := container.Summary{ID: "consumer", Image: "registry.example.com/app:latest", ImageID: index}
				if running != "" {
					os, arch, _ := strings.Cut(running, "/")
					live.ImageManifestDescriptor = &ocispec.Descriptor{MediaType: ocispec.MediaTypeImageManifest, Digest: digest.Digest("sha256:" + strings.Repeat("e", 64)), Platform: &ocispec.Platform{OS: os, Architecture: arch}}
				}
				svc := &ImageService{db: &database.DB{DB: db}}
				updates, err := svc.GetUpdateInfoByContainers(t.Context(), []container.Summary{live})
				require.NoError(t, err)
				got := updates[live.ID]
				require.NotNil(t, got)
				require.Equal(t, "rebuild", got.State, "build facts never change the lent state")
				if store == "classic" || running == "linux/amd64" {
					require.Equal(t, "linux/amd64", got.Platform)
					require.NotNil(t, got.LatestCreated)
				} else {
					require.Empty(t, got.Platform, "without a proven platform the web shows neither a date nor a short ID")
					require.Nil(t, got.CurrentCreated)
					require.Nil(t, got.LatestCreated)
				}
				// Reference lookups lend the observation to every consumer of the tag.
				byRef, err := svc.GetUpdateInfoByImageRefs(t.Context(), []string{live.Image})
				require.NoError(t, err)
				require.NotNil(t, byRef[live.Image])
				require.Empty(t, byRef[live.Image].Platform)
				require.Nil(t, byRef[live.Image].LatestCreated)
			})
		}
	}
}
