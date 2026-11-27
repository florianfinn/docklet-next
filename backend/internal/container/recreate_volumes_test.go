// Source: docklet-next/issues/667
// Revision: 99c1b0b8c4dfe56182c72749e092b08202436fa5
// Retrieved: 2026-11-27
// License: Apache-2.0
// Deviations: Original tests for anonymous and image-declared volumes across standalone recreate paths.

package container

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"testing"

	"github.com/florianfinn/docklet-next/backend/internal/event"
	images "github.com/florianfinn/docklet-next/backend/internal/image"
	"github.com/florianfinn/docklet-next/backend/internal/registry"
	"github.com/florianfinn/docklet-next/backend/internal/settings"
	dto "github.com/florianfinn/docklet-next/types/container"
	"github.com/florianfinn/docklet-next/types/user"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/stretchr/testify/require"
)

const (
	imageVolumeName = "1111111111111111111111111111111111111111111111111111111111111111"
	anonBindName    = "2222222222222222222222222222222222222222222222222222222222222222"
)

// volumeMountPoints is what the daemon reports for the old container: an
// image-declared volume, an anonymous short bind and a named volume.
func volumeMountPoints() []container.MountPoint {
	return []container.MountPoint{
		{Type: mount.TypeVolume, Name: imageVolumeName, Destination: "/cache", RW: true},
		{Type: mount.TypeVolume, Name: anonBindName, Destination: "/scratch", RW: true},
		{Type: mount.TypeVolume, Name: "data", Destination: "/data", RW: true},
	}
}

func requireVolumeMount(t *testing.T, host *container.HostConfig, source, target string) {
	t.Helper()
	require.True(t, slices.ContainsFunc(host.Mounts, func(m mount.Mount) bool {
		return m.Type == mount.TypeVolume && m.Source == source && m.Target == target
	}), "mounts %+v lack %s at %s", host.Mounts, source, target)
}

func TestImageUpdateAndRestoreKeepAnonymousVolumes(t *testing.T) {
	s, d, _, _ := updateFixture(t)
	old := d.containers[updateOldID]
	old.HostConfig.Binds = []string{"data:/data", "/scratch"}
	old.Mounts = volumeMountPoints()
	d.containers[updateOldID] = old

	require.NoError(t, s.ExecuteContainerImage(updateContext(t), updateOldID, "app", false, user.Actor{}))
	replacement := d.containers[updateNewID]
	require.Equal(t, []string{"data:/data", anonBindName + ":/scratch"}, replacement.HostConfig.Binds)
	requireVolumeMount(t, replacement.HostConfig, imageVolumeName, "/cache")
	require.Equal(t, []string{"data:/data", "/scratch"}, old.HostConfig.Binds, "inspected host config must stay unchanged")

	// The daemon reports the replacement's volumes by name; restore keeps them.
	replacement.Mounts = volumeMountPoints()
	d.containers[updateNewID] = replacement
	require.NoError(t, s.ExecuteContainerImage(updateContext(t), updateNewID, "app", true, user.Actor{}))
	restored := d.containers[updateRestoreID]
	require.Equal(t, []string{"data:/data", anonBindName + ":/scratch"}, restored.HostConfig.Binds)
	requireVolumeMount(t, restored.HostConfig, imageVolumeName, "/cache")
}

func TestRecreateRecoveryAfterCompletedStartKeepsVolumes(t *testing.T) {
	s, d, _, _ := updateFixture(t)
	old := d.containers[updateOldID]
	old.Mounts = volumeMountPoints()[:1]
	d.containers[updateOldID] = old
	d.failAfter = "start"

	require.NoError(t, s.ExecuteContainerImage(updateContext(t), updateOldID, "app", false, user.Actor{}))
	require.Equal(t, 1, d.counts["create"], "recovery must continue with the completed start, not create again")
	require.NotContains(t, d.containers, updateOldID, "the original is removed after recovery continued")
	requireVolumeMount(t, d.containers[updateNewID].HostConfig, imageVolumeName, "/cache")
}

func TestRecreateRefusesNamelessVolumeBeforeAnyEffect(t *testing.T) {
	s, d, _, db := updateFixture(t)
	old := d.containers[updateOldID]
	old.Mounts = []container.MountPoint{{Type: mount.TypeVolume, Destination: "/cache", RW: true}}
	d.containers[updateOldID] = old

	require.Error(t, s.ExecuteContainerImage(updateContext(t), updateOldID, "app", false, user.Actor{}))
	require.Zero(t, d.counts["create"])
	require.Equal(t, "/app", d.containers[updateOldID].Name, "the original must not be renamed")
	var journals int64
	require.NoError(t, db.DB.Model(&RecreateJournal{}).Count(&journals).Error)
	require.Zero(t, journals)
}

func TestRetainedVolumeMountPoints(t *testing.T) {
	original := container.HostConfig{Binds: []string{"data:/data", "/scratch"}}
	for name, tc := range map[string]struct {
		updated container.HostConfig
		want    []string
	}{
		"unchanged keeps anonymous volumes": {updated: original, want: []string{"/cache", "/scratch"}},
		"retarget to named volume":          {updated: container.HostConfig{Binds: []string{"data:/data", "other:/scratch"}}, want: []string{"/cache"}},
		"retarget to host bind":             {updated: container.HostConfig{Binds: []string{"data:/data", "/srv/x:/scratch"}}, want: []string{"/cache"}},
		"retarget to tmpfs mount":           {updated: container.HostConfig{Binds: []string{"data:/data"}, Mounts: []mount.Mount{{Type: mount.TypeTmpfs, Target: "/scratch"}}}, want: []string{"/cache"}},
		"removed anonymous bind":            {updated: container.HostConfig{Binds: []string{"data:/data"}}, want: []string{"/cache"}},
		"anonymous mount keeps the volume":  {updated: container.HostConfig{Binds: []string{"data:/data"}, Mounts: []mount.Mount{{Type: mount.TypeVolume, Target: "/scratch"}}}, want: []string{"/cache", "/scratch"}},
		"named volume left to the image":    {updated: container.HostConfig{Binds: []string{"/scratch"}}, want: []string{"/cache", "/scratch"}},
	} {
		t.Run(name, func(t *testing.T) {
			var got []string
			for _, mp := range retainedVolumeMountPoints(original, tc.updated, volumeMountPoints()) {
				got = append(got, mp.Destination)
			}
			require.Equal(t, tc.want, got)
		})
	}
}

func TestRecreateVolumeHostConfigCopiesTheRequest(t *testing.T) {
	requested := &container.HostConfig{Binds: []string{"/scratch"}, Mounts: []mount.Mount{{Type: mount.TypeVolume, Target: "/cache"}}}
	old := container.InspectResponse{Name: "/app", HostConfig: requested, Mounts: volumeMountPoints()[:2]}
	host, err := recreateVolumeHostConfig(t.Context(), old, requested)
	require.NoError(t, err)
	require.Equal(t, []string{anonBindName + ":/scratch"}, host.Binds)
	requireVolumeMount(t, host, imageVolumeName, "/cache")
	require.Equal(t, []string{"/scratch"}, requested.Binds)
	require.Empty(t, requested.Mounts[0].Source)
}

// recordingDaemon answers the non-durable recreate sequence and keeps every
// create body. failStart names a container whose start fails.
type recordingDaemon struct {
	mu        sync.Mutex
	creates   []container.HostConfig
	failStart string
}

func (d *recordingDaemon) serve(w http.ResponseWriter, r *http.Request) {
	d.mu.Lock()
	defer d.mu.Unlock()
	switch p := dockerTestPathInternal(r.URL.Path); p {
	case "/containers/create":
		var body struct{ HostConfig container.HostConfig }
		_ = json.NewDecoder(r.Body).Decode(&body)
		d.creates = append(d.creates, body.HostConfig)
		id := "new-id"
		if len(d.creates) > 1 {
			id = "restored-id"
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{"Id": id, "Warnings": []string{}})
	case "/containers/" + d.failStart + "/start":
		http.Error(w, "synthetic start failure", http.StatusInternalServerError)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

func recreateWithoutRun(t *testing.T, d *recordingDaemon, info container.InspectResponse) error {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(d.serve))
	t.Cleanup(server.Close)
	svc := NewContainerService(event.NewEventService(setupProjectTestDBInternal(t), nil, nil), nil, nil, nil, nil)
	_, err := svc.recreateContainerInternal(t.Context(), newTestDockerClientInternal(t, server), info, "redeploy", "web", info.Config, info.HostConfig, nil, "1.41", event.EventTypeContainerDeploy, user.SystemUser)
	return err
}

func TestNonDurableRecreateKeepsVolumes(t *testing.T) {
	d := &recordingDaemon{}
	info := container.InspectResponse{ID: "old-id", Name: "/web", State: &container.State{Running: true}, Config: &container.Config{Image: "app:latest"}, HostConfig: &container.HostConfig{Binds: []string{"/scratch"}}, Mounts: volumeMountPoints()[:2]}
	require.NoError(t, recreateWithoutRun(t, d, info))
	require.Len(t, d.creates, 1)
	require.Equal(t, []string{anonBindName + ":/scratch"}, d.creates[0].Binds)
	requireVolumeMount(t, &d.creates[0], imageVolumeName, "/cache")
	require.Equal(t, []string{"/scratch"}, info.HostConfig.Binds)
}

func TestAutoRemoveStartFailureRestoreKeepsVolumes(t *testing.T) {
	d := &recordingDaemon{failStart: "new-id"}
	info := container.InspectResponse{ID: "old-id", Name: "/web", State: &container.State{Running: true}, Config: &container.Config{Image: "app:latest"}, HostConfig: &container.HostConfig{AutoRemove: true}, Mounts: volumeMountPoints()[:1]}
	require.Error(t, recreateWithoutRun(t, d, info))
	require.Len(t, d.creates, 2, "the failed start must rebuild the auto-removed original")
	requireVolumeMount(t, &d.creates[1], imageVolumeName, "/cache")
}

func volumeBearingOriginal(d *updateDaemon) {
	old := d.containers[updateOldID]
	old.HostConfig.Binds = []string{"data:/data", "/scratch"}
	old.Mounts = volumeMountPoints()
	d.containers[updateOldID] = old
}

func TestEditKeepsAnonymousVolumesUnlessRetargeted(t *testing.T) {
	s, d, _, _ := updateFixture(t)
	volumeBearingOriginal(d)
	_, err := s.EditContainer(updateContext(t), updateOldID, dto.Edit{}, user.Actor{})
	require.NoError(t, err)
	replacement := d.containers[updateNewID]
	require.Equal(t, []string{"data:/data", anonBindName + ":/scratch"}, replacement.HostConfig.Binds)
	requireVolumeMount(t, replacement.HostConfig, imageVolumeName, "/cache")

	s, d, _, _ = updateFixture(t)
	volumeBearingOriginal(d)
	binds := []string{"data:/data", "other:/scratch"}
	_, err = s.EditContainer(updateContext(t), updateOldID, dto.Edit{HostConfig: &dto.HostConfigEdit{Binds: &binds}}, user.Actor{})
	require.NoError(t, err)
	replacement = d.containers[updateNewID]
	require.Equal(t, binds, replacement.HostConfig.Binds, "a retargeted destination keeps the requested volume")
	requireVolumeMount(t, replacement.HostConfig, imageVolumeName, "/cache")
}

func TestRedeployKeepsAnonymousVolumes(t *testing.T) {
	s, d, _, db := updateFixture(t)
	volumeBearingOriginal(d)
	cfg, err := settings.NewSettingsService(t.Context(), db)
	require.NoError(t, err)
	t.Cleanup(func() { cfg.Stop(t.Context()) })
	s.settingsService = cfg
	s.imageService = images.NewImageService(db, s.dockerService, registry.NewContainerRegistryService(db, nil, nil, nil), nil, nil, nil, cfg, nil)
	_, err = s.RedeployContainer(updateContext(t), updateOldID, user.Actor{})
	require.NoError(t, err)
	require.Equal(t, 1, d.counts["pull"])
	replacement := d.containers[updateNewID]
	require.Equal(t, []string{"data:/data", anonBindName + ":/scratch"}, replacement.HostConfig.Binds)
	requireVolumeMount(t, replacement.HostConfig, imageVolumeName, "/cache")
}

// Startup recovery after a crash past start keeps the replacement the
// durable create produced and removes the original without its volumes.
func TestStartupRecoveryKeepsPreservedReplacementAndVolumes(t *testing.T) {
	f := effectFixture(t, "remove-old")
	old := f.d.containers[updateOldID]
	old.HostConfig = &container.HostConfig{Binds: []string{"/scratch"}}
	old.Mounts = volumeMountPoints()[:2]
	f.d.containers[updateOldID] = old
	host, err := recreateVolumeHostConfig(t.Context(), old, old.HostConfig)
	require.NoError(t, err)
	replacement := f.d.containers[updateNewID]
	replacement.HostConfig = host
	f.d.containers[updateNewID] = replacement

	require.NoError(t, f.rt.InitializeLocalStartup(t.Context(), f.store))
	require.Equal(t, "done", f.journalPhase(t, "app"))
	require.Zero(t, f.d.counts["create"], "recovery never creates")
	require.NotContains(t, f.d.containers, updateOldID)
	require.Equal(t, []string{""}, f.d.removeVolumes, "the original is removed without its volumes")
	kept := f.d.containers[updateNewID]
	require.Equal(t, []string{anonBindName + ":/scratch"}, kept.HostConfig.Binds)
	requireVolumeMount(t, kept.HostConfig, imageVolumeName, "/cache")
}
