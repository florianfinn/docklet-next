// Source: docklet-next/issues/409
// Revision: fe99a2a2962fa1b5df9ebaaff115f89690f685fb
// Retrieved: 2026-11-09
// License: Apache-2.0
// Deviations: Original connected tag-policy update from the real checker through agent preflight to Engine create.

package activity_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/imageupdate"
	"github.com/florianfinn/docklet-next/backend/internal/registry"
	"github.com/florianfinn/docklet-next/backend/internal/updater"
	"github.com/florianfinn/docklet-next/backend/pkg/scheduler/runs"
	francistest "github.com/florianfinn/docklet-next/backend/pkg/utils/francis/testing"
	activityDTO "github.com/florianfinn/docklet-next/types/activity"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	projectDTO "github.com/florianfinn/docklet-next/types/project"
	dto "github.com/florianfinn/docklet-next/types/updater"
	"github.com/google/go-containerregistry/pkg/name"
	registryServer "github.com/google/go-containerregistry/pkg/registry"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/require"
	"go.getarcane.app/sys/crypto"
	"go.getarcane.app/updater/labels"
)

// The cases come from the fixture the updater package's real checker test
// asserts (TestPatternTagCheckerMatchesTheSharedFixture).
type connectedPatternCase struct {
	Name, Pattern, CurrentTag, NextTag string
	Update                             imageDTO.UpdateInfo
}

// A tag-policy update observed by the real checker is prepared by the real
// agent preflight, sent through Direct or Edge, pulled by digest and created
// from the selected literal tag.
func TestTagPolicyUpdateFromRealCheckToEngineCreate(t *testing.T) {
	raw, err := os.ReadFile("../../../web/src/data/http/testdata/pattern-tag-updates.json")
	require.NoError(t, err)
	var cases []connectedPatternCase
	require.NoError(t, json.Unmarshal(raw, &cases))
	require.NotEmpty(t, cases)
	for _, tc := range cases {
		for _, isEdge := range []bool{false, true} {
			t.Run(map[bool]string{false: "direct", true: "edge"}[isEdge]+"/"+tc.Name, func(t *testing.T) { testConnectedPatternTag(t, tc, isEdge) })
		}
	}
}

func testConnectedPatternTag(t *testing.T, tc connectedPatternCase, isEdge bool) {
	f := newRemoteIntegration(t, isEdge)
	cli, err := client.New(client.WithHost(f.agent.http.server.URL), client.WithHTTPClient(f.agent.http.server.Client()), client.WithAPIVersion("1.49"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = cli.Close() })
	f.agent.docker.Client = cli
	crypto.InitEncryption(&crypto.Config{Environment: "test", EncryptionKey: "synthetic-lifecycle-encryption-key-32bytes"})
	fw := francistest.New(t)
	gate := runs.NewAdmission(fw.Service(), t.Name())
	require.NoError(t, gate.Register(fw))
	francistest.Start(t, fw)
	f.agent.images.SetImageAdmission(gate, nil)

	server := httptest.NewServer(registryServer.New())
	t.Cleanup(server.Close)
	repo := strings.TrimPrefix(server.URL, "http://") + "/app"
	oldRef, newRef := repo+":"+tc.CurrentTag, repo+":"+tc.NextTag
	manifests, configs := map[string]string{}, map[string]string{}
	for _, tag := range []string{tc.CurrentTag, tc.NextTag} {
		img, e := mutate.ConfigFile(empty.Image, &v1.ConfigFile{OS: "linux", Architecture: "amd64", Config: v1.Config{Labels: map[string]string{"example.marker": tag}}})
		require.NoError(t, e)
		ref, e := name.NewTag(repo+":"+tag, name.Insecure)
		require.NoError(t, e)
		require.NoError(t, remote.Write(ref, img, remote.WithContext(t.Context())))
		manifest, e := img.Digest()
		require.NoError(t, e)
		config, e := img.ConfigName()
		require.NoError(t, e)
		manifests[repo+":"+tag], configs[repo+":"+tag] = manifest.String(), config.String()
	}
	oldID, newID, newManifest := configs[oldRef], configs[newRef], manifests[newRef]
	require.NoError(t, f.agent.db.Create(&registry.ContainerRegistry{URL: server.URL, Enabled: true, Insecure: true, RegistryType: "generic"}).Error)

	// Classic Engine store: image IDs are config digests, RepoDigests name the manifests.
	var mu sync.Mutex
	tags := map[string]string{oldRef: oldID, oldID: oldID}
	repoDigests := map[string]string{oldID: repo + "@" + manifests[oldRef], newID: repo + "@" + newManifest}
	reference, pulls := oldRef, []string{}
	f.agent.http.setLabels(map[string]string{"com.docker.compose.project": "example_stack", "com.docker.compose.service": "app", "com.docker.compose.container-number": "1", "com.docker.compose.config-hash": "example-hash"})
	f.agent.http.onCall = func(w http.ResponseWriter, r *http.Request) bool {
		path := r.URL.Path
		if i := strings.Index(path[1:], "/"); i >= 0 {
			path = path[i+1:]
		}
		w.Header().Set("Content-Type", "application/json")
		mu.Lock()
		defer mu.Unlock()
		switch {
		case path == "/info":
			_, _ = w.Write([]byte(`{"OSType":"linux","Architecture":"x86_64"}`))
		case strings.HasPrefix(path, "/distribution/"):
			ref, _ := url.PathUnescape(strings.TrimSuffix(strings.TrimPrefix(path, "/distribution/"), "/json"))
			_ = json.NewEncoder(w).Encode(map[string]any{"Descriptor": map[string]any{"digest": manifests[ref], "mediaType": "application/vnd.docker.distribution.manifest.v2+json"}})
		case strings.HasPrefix(path, "/images/") && strings.HasSuffix(path, "/json"):
			ref, _ := url.PathUnescape(strings.TrimSuffix(strings.TrimPrefix(path, "/images/"), "/json"))
			id, ok := tags[ref]
			if !ok {
				w.WriteHeader(404)
				_, _ = w.Write([]byte(`{"message":"not found"}`))
				return true
			}
			all := []string{}
			for tag, value := range tags {
				if value == id && !strings.HasPrefix(tag, "sha256:") && !strings.Contains(tag, "@") {
					all = append(all, tag)
				}
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"Id": id, "Os": "linux", "Architecture": "amd64", "RepoTags": all, "RepoDigests": []string{repoDigests[id]}, "Config": map[string]any{}})
		case strings.HasPrefix(path, "/images/") && strings.HasSuffix(path, "/tag"):
			ref, _ := url.PathUnescape(strings.TrimSuffix(strings.TrimPrefix(path, "/images/"), "/tag"))
			tags[r.URL.Query().Get("repo")+":"+r.URL.Query().Get("tag")] = tags[ref]
			w.WriteHeader(201)
		case path == "/images/create":
			pulls = append(pulls, r.URL.Query().Get("fromImage")+"|"+r.URL.Query().Get("tag"))
			tags[repo+"@"+newManifest], tags[newID] = newID, newID
			_, _ = w.Write([]byte(`{"status":"downloaded"}`))
		case strings.HasPrefix(path, "/images/") && r.Method == http.MethodDelete:
			ref, _ := url.PathUnescape(strings.TrimPrefix(path, "/images/"))
			delete(tags, ref)
			_, _ = w.Write([]byte(`[]`))
		case path == "/containers/create":
			f.agent.http.mu.Lock()
			body := f.agent.http.events[len(f.agent.http.events)-1].Body
			f.agent.http.mu.Unlock()
			var conf struct{ Image string }
			require.NoError(t, json.Unmarshal([]byte(body), &conf))
			reference = conf.Image
			f.agent.http.mu.Lock()
			if f.agent.http.activeID == runtimeReplacementContainerID {
				f.agent.http.composeContainer = false
				f.agent.http.activeID = ""
			}
			f.agent.http.mu.Unlock()
			return false
		case path == "/containers/json" && r.URL.Query().Get("filters") == "":
			f.agent.http.mu.Lock()
			present, running, id, containerLabels := f.agent.http.composeContainer, f.agent.http.running, f.agent.http.activeID, f.agent.http.labels
			f.agent.http.mu.Unlock()
			rows := []any{}
			if present {
				state := map[bool]string{false: "exited", true: "running"}[running]
				rows = append(rows, map[string]any{"Id": id, "Image": reference, "ImageID": tags[reference], "State": state, "Labels": containerLabels})
			}
			_ = json.NewEncoder(w).Encode(rows)
		case path != "/containers/json" && strings.HasPrefix(path, "/containers/") && strings.HasSuffix(path, "/json"):
			id := strings.TrimSuffix(strings.TrimPrefix(path, "/containers/"), "/json")
			f.agent.http.mu.Lock()
			running, containerLabels := f.agent.http.running, f.agent.http.labels
			f.agent.http.mu.Unlock()
			state := map[bool]string{false: "exited", true: "running"}[running]
			_ = json.NewEncoder(w).Encode(map[string]any{"Id": id, "Name": "/example_stack-app-1", "Image": tags[reference], "Config": map[string]any{"Image": reference, "Labels": containerLabels}, "State": map[string]any{"Status": state, "Running": running}, "NetworkSettings": map[string]any{"Networks": map[string]any{}}})
		default:
			return false
		}
		return true
	}

	target := activity.RuntimeTarget{Kind: "stack", ID: "example-project"}
	source := "name: example_stack\nservices:\n  app:\n    image: " + oldRef + "\n    labels:\n      " + labels.LabelUpdateStrategy + ": tag\n"
	if tc.Pattern != "" {
		source += "      " + labels.LabelUpdateTagPattern + ": '" + tc.Pattern + "'\n"
	}
	path := f.agent.project(target.ID, target.ID, source)
	deploy := f.agent.command(activity.RuntimeDeployProject, target)
	deploy.Options.Deploy = &projectDTO.DeployOptions{PullPolicy: "never"}
	f.agent.run(deploy)
	baseline, err := f.agent.projects.ImageBaselineStatus(t.Context(), target.ID, f.store.RegistrationID)
	require.NoError(t, err, "baseline code=%s events=%s", imageDTO.Neutral(err, "unknown", 500).Problem.Code, f.agent.debugEvents())
	require.True(t, baseline.Available)

	// The real checker is the agent's lifecycle checker (NewUpdaterService wires it).
	registries := registry.NewContainerRegistryService(f.agent.db, nil, nil, f.agent.settings, server.Client())
	checks := imageupdate.NewImageUpdateService(f.agent.db, f.agent.settings, registries, f.agent.docker, nil, nil, nil)
	checker, err := updater.NewUpdaterService(f.agent.db, f.agent.settings, f.agent.docker, f.agent.projects, checks, registries, nil, f.agent.images, nil, nil, f.hubActivity, &config.Config{}, nil, nil, nil)
	require.NoError(t, err)
	checked, err := checker.CheckProjectUpdates(t.Context(), target.ID)
	require.NoError(t, err)
	var record imageupdate.ImageUpdateRecord
	require.NoError(t, f.agent.db.First(&record, "project_id = ? AND service_name = ?", target.ID, "app").Error)
	for _, observed := range []*imageDTO.UpdateInfo{checked.ServiceUpdates["app"].UpdateInfo, record.UpdateInfo()} {
		require.NotNil(t, observed)
		require.Empty(t, observed.Error)
		require.Equal(t, []any{tc.Update.State, tc.Update.HasUpdate, tc.Update.CurrentVersion, tc.Update.LatestVersion}, []any{observed.State, observed.HasUpdate, observed.CurrentVersion, observed.LatestVersion})
	}

	meta, err := f.sender.Resolve(t.Context(), activityDTO.RuntimeTargetRequest{Registration: activityDTO.RuntimeExecutorIdentity{ManagerID: f.store.ManagerID, RegistrationID: f.store.RegistrationID, ExecutorGeneration: f.store.ExecutorGeneration}, Target: target, Lifecycle: &dto.LifecycleRequest{ExpectedRevision: baseline.SavedRevision, Services: []string{"app"}}})
	require.NoError(t, err)
	require.NotNil(t, meta.Lifecycle)
	require.Len(t, meta.Lifecycle.Selections, 1)
	selection := meta.Lifecycle.Selections[0]
	require.Equal(t, []string{"version", newRef, newManifest, newID}, []string{selection.Kind, selection.Reference, selection.Digest, selection.ImageID})

	command := f.command(activity.RuntimeUpdateProject, target)
	command.Options.Lifecycle = meta.Lifecycle
	admitted, err := f.hub.Admit(t.Context(), command)
	require.NoError(t, err)
	ref := activity.RuntimeRunReference{Store: f.store, RunID: admitted.ActivityID}
	_, err = f.control.Start(t.Context(), ref, command.Options)
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	require.NoError(t, f.control.WaitDispatch(ctx, ref.RunID))
	agent, err := f.agent.waitLocal(ref.RunID)
	require.NoError(t, err)
	require.Equal(t, activityDTO.StatusSuccess, agent.Status, f.agent.debugEvents())
	out, err := f.control.Wait(ctx, ref, nil)
	require.NoError(t, err)
	require.Equal(t, activityDTO.StatusSuccess, out.Status)

	body, err := os.ReadFile(filepath.Join(path, "compose.yaml"))
	require.NoError(t, err)
	require.Equal(t, strings.Replace(source, oldRef, newRef, 1), string(body))
	mu.Lock()
	defer mu.Unlock()
	require.Len(t, pulls, 1)
	require.Contains(t, pulls[0], newManifest, "the candidate is pulled by its immutable digest")
	require.Equal(t, newRef, reference, "the Engine creates the container from the selected literal tag")
}
