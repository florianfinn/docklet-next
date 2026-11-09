// Source: docklet-next/issues/438
// Revision: 57e835f7cce5778fd3d164298e5b488e11abb71d
// Retrieved: 2026-11-09
// License: Apache-2.0
// Deviations: Original manifest-bound container execution over actual Direct and Edge transports.

package activity_test

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/container/children/origin"
	activityDTO "github.com/florianfinn/docklet-next/types/activity"
	dto "github.com/florianfinn/docklet-next/types/updater"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/stretchr/testify/require"
)

func TestVerifiedContainerDirectEdgeMovedAndPinned(t *testing.T) {
	for _, isEdge := range []bool{false, true} {
		for _, mode := range []string{"moved", "installed", "stable"} {
			t.Run(fmt.Sprint(isEdge)+"/"+mode, func(t *testing.T) {
				daemon, registry := newContainerImageHTTP(t)
				tag, err := name.NewTag(daemon.ref, name.Insecure)
				require.NoError(t, err)
				manifest, err := remote.Head(tag, remote.WithContext(t.Context()))
				require.NoError(t, err)
				expected := manifest.Digest.String()
				if mode == "moved" {
					moved, err := mutate.ConfigFile(empty.Image, &v1.ConfigFile{OS: "linux", Architecture: "amd64", Config: v1.Config{Labels: map[string]string{"generation": "moved-before-update"}}})
					require.NoError(t, err)
					require.NoError(t, remote.Write(tag, moved, remote.WithContext(t.Context())))
				}
				if mode == "installed" {
					before, err := mutate.ConfigFile(empty.Image, &v1.ConfigFile{OS: "linux", Architecture: "amd64", Config: v1.Config{Labels: map[string]string{"generation": "before"}}})
					require.NoError(t, err)
					require.NoError(t, remote.Write(tag, before, remote.WithContext(t.Context())))
				}
				f := newRemoteIntegration(t, isEdge, func(agent *runtimeServicesFixture) {
					configureContainerImageFixture(t, agent, daemon, registry)
					agent.http.onCall = func(w http.ResponseWriter, r *http.Request) bool {
						if strings.HasSuffix(r.URL.Path, "/images/create") {
							require.Equal(t, daemon.repo, r.URL.Query().Get("fromImage"))
							require.Equal(t, expected, r.URL.Query().Get("tag"), "pull remains pinned after a tag move")
							moved, err := mutate.ConfigFile(empty.Image, &v1.ConfigFile{OS: "linux", Architecture: "amd64", Config: v1.Config{Labels: map[string]string{"generation": "moved-after-compare"}}})
							require.NoError(t, err)
							require.NoError(t, remote.Write(tag, moved, remote.WithContext(t.Context())))
						}
						return daemon.serve(w, r)
					}
				})
				guard := &origin.Store{DB: f.db.DB, LocalRegistration: "hub"}
				intake, err := activity.NewRuntimeHTTPService(f.hub, activity.RuntimeHTTPPorts{ApplicationContext: t.Context(), Store: func(context.Context, string) (activity.RuntimeStoreBinding, error) { return f.store, nil }, PrepareTarget: func(ctx context.Context, _ string, target activity.RuntimeTarget) (activity.RuntimeResolvedTarget, error) {
					return f.agent.adapter.ResolveRuntimeTarget(ctx, target)
				}, GuardContainer: func(ctx context.Context, _ string, id string) error {
					current, err := f.agent.containers.GetContainerByID(ctx, id)
					if err != nil {
						return err
					}
					return guard.Guard(ctx, f.store.RegistrationID, origin.Identity{ID: current.ID, Name: "app", Labels: current.Config.Labels})
				}})
				require.NoError(t, err)
				intake.SetReady(true)
				command := f.command(activity.RuntimeUpdateContainer, activity.RuntimeTarget{Kind: "container", ID: runtimeContainerID})
				command.Options.ContainerImage = &dto.ContainerImageRequest{ExpectedManifest: expected}
				accepted, err := f.hub.Admit(t.Context(), command)
				require.NoError(t, err)
				// A retained request key cannot silently bind to another scanned digest.
				changed := command
				changed.Options.ContainerImage = &dto.ContainerImageRequest{ExpectedManifest: "sha256:" + strings.Repeat("d", 64)}
				_, err = f.hub.Admit(t.Context(), changed)
				require.Error(t, err)
				ref := activity.RuntimeRunReference{Store: f.store, RunID: accepted.ActivityID}
				_, err = f.control.Start(t.Context(), ref, command.Options)
				require.NoError(t, err)
				ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
				defer cancel()
				require.NoError(t, f.control.WaitDispatch(ctx, ref.RunID))
				snapshot, err := f.agent.waitLocal(ref.RunID)
				require.NoError(t, err)
				require.NotNil(t, snapshot.ImageLifecycle)
				if mode != "stable" {
					require.Equal(t, activityDTO.StatusFailed, snapshot.Status)
					require.Equal(t, "update.candidate-changed", snapshot.ImageLifecycle.Code)
					require.Zero(t, daemon.pulls)
					require.Zero(t, daemon.creates)
					require.Zero(t, daemon.tags)
					require.Empty(t, daemon.base.counts())
				} else {
					require.Equal(t, activityDTO.StatusSuccess, snapshot.Status, "%+v", snapshot.ImageLifecycle)
					require.True(t, snapshot.ImageLifecycle.Verified)
					require.Equal(t, 1, daemon.pulls)
					require.Equal(t, 1, daemon.creates)
					// The pull is pinned, but the container keeps its tag for later checks.
					require.Equal(t, []string{daemon.ref, daemon.ref}, []string{daemon.createdImage, daemon.tagTarget})
				}
			})
		}
	}
}
