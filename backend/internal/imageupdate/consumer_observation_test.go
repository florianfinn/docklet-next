// Source: docklet-next/issues/409
// Revision: e683d402f358e87b8eef5b81a71b530fd70cc0fe
// Retrieved: 2026-11-10
// License: Apache-2.0
// Deviations: Original late consumer identity and failed-refresh invalidation assertions.

package imageupdate

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/docker"
	"github.com/florianfinn/docklet-next/backend/internal/imageupdate/children/identity"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/imageref"
	checkDTO "github.com/florianfinn/docklet-next/types/imageupdate"
	"github.com/moby/moby/api/types/container"
	"github.com/stretchr/testify/require"
)

func TestConsumerObservationLateIdentityAndFailureInvalidation(t *testing.T) {
	for _, change := range []string{"policy", "image", "identity", "missing", "failed-refresh"} {
		t.Run(change, func(t *testing.T) {
			db := setupImageUpdateRegistryTestDBInternal(t)
			ref := "registry.example.com/app:1.0.0"
			id := "sha256:" + strings.Repeat("a", 64)
			next := "sha256:" + strings.Repeat("b", 64)
			labels := map[string]string{"com.getarcaneapp.arcane.updater.strategy": "tag"}
			consumer := container.Summary{ID: "consumer", Image: ref, ImageID: id, Labels: labels}
			native, err := identity.DecodeObservation("native-v1:classic:linux/amd64", id, next)
			require.NoError(t, err)
			saved := ImageUpdateRecord{ID: "container::consumer", ContainerID: consumer.ID, ImageID: id, Repository: "registry.example.com/app", Tag: "1.0.0", PolicyKey: imageref.UpdatePolicyKey(ref, labels), HasUpdate: true, CheckTime: time.Now().Add(-time.Hour)}
			saved.SetNativeObservation(native)
			require.NoError(t, db.Create(&saved).Error)
			inspected := container.InspectResponse{ID: consumer.ID, Image: id, Config: &container.Config{Image: ref, Labels: labels}}
			switch change {
			case "policy":
				inspected.Config.Labels = map[string]string{"com.getarcaneapp.arcane.updater.strategy": "digest"}
			case "image":
				inspected.Image = next
			case "identity":
				inspected.ID = "replacement"
			}
			daemon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if change == "missing" || change == "failed-refresh" {
					http.NotFound(w, r)
					return
				}
				require.True(t, strings.HasSuffix(r.URL.Path, "/containers/consumer/json"))
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(inspected)
			}))
			t.Cleanup(daemon.Close)
			svc := NewImageUpdateService(db, nil, nil, &docker.DockerClientService{Client: newImageUpdateTestDockerClientInternal(t, daemon)}, nil, nil, nil)
			if change == "failed-refresh" {
				result, err := svc.ReadUpdateInfo(t.Context(), ref)
				require.NoError(t, err)
				require.Equal(t, "unknown", result.State)
			} else {
				result := &checkDTO.Response{ImageRef: ref, State: "version", HasUpdate: true, CurrentDigest: id, LatestDigest: next, CheckTime: time.Now().UTC()}
				require.NoError(t, svc.saveConsumerObservation(t.Context(), consumer, result, identity.LocalImage{}, &native))
				require.Equal(t, "unknown", result.State)
			}
			var after ImageUpdateRecord
			require.NoError(t, db.First(&after, "id=?", saved.ID).Error)
			require.False(t, after.HasUpdate, "a slow or failed check must replace earlier candidate eligibility")
			require.Nil(t, after.Platform, "unproven consumer bytes cannot acquire native provenance")
			require.NotNil(t, after.LastError)
		})
	}
}

func TestConsumerObservationImplicitLatestReference(t *testing.T) {
	db := setupImageUpdateRegistryTestDBInternal(t)
	consumer := container.Summary{ID: "consumer", Image: "registry.example.com/app", ImageID: "sha256:" + strings.Repeat("a", 64), Labels: map[string]string{"com.getarcaneapp.arcane.updater.strategy": "tag"}}
	daemon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(container.InspectResponse{ID: consumer.ID, Image: consumer.ImageID, Config: &container.Config{Image: consumer.Image, Labels: consumer.Labels}})
	}))
	t.Cleanup(daemon.Close)
	svc := NewImageUpdateService(db, nil, nil, &docker.DockerClientService{Client: newImageUpdateTestDockerClientInternal(t, daemon)}, nil, nil, nil)
	result := &checkDTO.Response{State: "error", Code: "update.check-unavailable", CheckTime: time.Now().UTC()}
	require.NoError(t, svc.saveConsumerObservation(t.Context(), consumer, result, identity.LocalImage{}, nil), "an implicit latest reference must not abort the whole batch")
	var saved ImageUpdateRecord
	require.NoError(t, db.First(&saved, "id=?", "container::consumer").Error)
	require.Equal(t, "latest", saved.Tag)
	require.False(t, saved.HasUpdate)
	require.NotNil(t, saved.LastError)
}
