// Source: docklet-next/issues/412
// Revision: 6e3afa6a78f4f60ffd74858d9a1eaefbb1b73bda
// Retrieved: 2026-11-12
// License: Apache-2.0
// Deviations: Original live digest observation identity and fallback assertions.

package image

import (
	"strings"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/imageupdate"
	"github.com/florianfinn/docklet-next/backend/internal/testsqlite"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/imageref"
	"github.com/moby/moby/api/types/container"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestDigestScopedObservationRequiresLiveIdentity(t *testing.T) {
	for _, change := range []string{"none", "image", "missing-image", "policy", "reference", "row-key", "container-id", "tag-policy"} {
		t.Run(change, func(t *testing.T) {
			db, err := gorm.Open(testsqlite.Dialector(t, ":memory:"), &gorm.Config{})
			require.NoError(t, err)
			require.NoError(t, db.AutoMigrate(&imageupdate.ImageUpdateRecord{}))
			current := "sha256:" + strings.Repeat("a", 64)
			next := "sha256:" + strings.Repeat("b", 64)
			live := container.Summary{ID: "consumer", Image: "registry.example.com/app:stable", ImageID: current}
			scoped := imageupdate.ImageUpdateRecord{ID: "container::consumer", ContainerID: live.ID, ImageID: current, Repository: "registry.example.com/app", Tag: "stable", PolicyKey: imageref.UpdatePolicyKey(live.Image, live.Labels), CheckTime: time.Now().UTC(), CurrentDigest: &current, LatestDigest: &next, Platform: new("native-v1:classic:linux/amd64"), HasUpdate: true}
			switch change {
			case "image":
				scoped.ImageID = next
			case "missing-image":
				live.ImageID = ""
			case "policy":
				scoped.PolicyKey = "previous-policy"
			case "reference":
				scoped.Tag = "edge"
			case "row-key":
				scoped.ID = "container::previous"
			case "container-id":
				scoped.ContainerID = "previous"
			case "tag-policy":
				live.Labels = map[string]string{"com.getarcaneapp.arcane.updater.strategy": "tag"}
			}
			imageRow := imageupdate.ImageUpdateRecord{ID: current, ImageID: current, Repository: "registry.example.com/app", Tag: "stable", CheckTime: time.Now().UTC(), CurrentDigest: &current, LatestDigest: &current, Platform: new("native-v1:classic:linux/amd64")}
			require.NoError(t, db.Create(&[]imageupdate.ImageUpdateRecord{scoped, imageRow}).Error)
			svc := &ImageService{db: &database.DB{DB: db}}
			updates, err := svc.GetUpdateInfoByContainers(t.Context(), []container.Summary{live})
			require.NoError(t, err)
			if change == "missing-image" || change == "tag-policy" {
				require.NotContains(t, updates, live.ID)
				return
			}
			require.Contains(t, updates, live.ID)
			want := "current"
			if change == "none" {
				want = "rebuild"
			}
			require.Equal(t, want, updates[live.ID].State, "only a matching scoped row overrides the image-level reference fallback")
			// Without same-reference image evidence, stale scoped rows inherit nothing.
			require.NoError(t, db.Model(&imageupdate.ImageUpdateRecord{}).Where("id = ?", current).Update("tag", "other").Error)
			updates, err = svc.GetUpdateInfoByContainers(t.Context(), []container.Summary{live})
			require.NoError(t, err)
			if change != "none" {
				require.NotContains(t, updates, live.ID)
			} else {
				require.Equal(t, "rebuild", updates[live.ID].State)
			}
		})
	}
}
