// Source: docklet-next/issues/341
// Revision: 0c8a014eda72706e48d60c22e86526e7dfd15964
// Retrieved: 2026-11-05
// License: Apache-2.0
// Deviations: A digest-policy container never receives another tag's observation.

package image

import (
	"strings"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/imageupdate"
	"github.com/florianfinn/docklet-next/backend/internal/testsqlite"
	"github.com/moby/moby/api/types/container"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestDigestContainerReadsOnlyItsOwnReference(t *testing.T) {
	db, err := gorm.Open(testsqlite.Dialector(t, ":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&imageupdate.ImageUpdateRecord{}))
	svc := &ImageService{db: &database.DB{DB: db}}
	imageID := "sha256:" + strings.Repeat("0", 64)
	require.NoError(t, db.Create(&imageupdate.ImageUpdateRecord{ID: imageID, Repository: "registry.example.com/window", Tag: "latest", CurrentVersion: "latest", HasUpdate: true,
		Platform: new("native-v1:classic:linux/amd64"), CurrentDigest: new("sha256:" + strings.Repeat("a", 64)), LatestDigest: new("sha256:" + strings.Repeat("b", 64)), CheckTime: time.Now().UTC()}).Error)
	scoped, err := svc.GetUpdateInfoByContainers(t.Context(), []container.Summary{
		{ID: "window", Image: "registry.example.com/window:latest", ImageID: imageID},
		{ID: "manual", Image: "registry.example.com/manual:latest", ImageID: imageID},
		{ID: "pinned", Image: "registry.example.com/window:latest@sha256:" + strings.Repeat("c", 64), ImageID: imageID},
	})
	require.NoError(t, err)
	require.NotNil(t, scoped["window"])
	require.True(t, scoped["window"].HasUpdate)
	require.NotNil(t, scoped["pinned"], "a pinned spelling of the checked reference keeps its observation")
	require.Nil(t, scoped["manual"], "another tag of the same image must not receive this observation")
}

func TestSameUpdateReference(t *testing.T) {
	require.True(t, imageupdate.SameUpdateReference("docker.io/library/nginx:latest", "nginx"))
	require.True(t, imageupdate.SameUpdateReference("alpine:3.22", "alpine:3.22@sha256:"+strings.Repeat("d", 64)))
	require.False(t, imageupdate.SameUpdateReference("alpine:3.22", "alpine:3.23"))
	require.False(t, imageupdate.SameUpdateReference("", ""))
}
