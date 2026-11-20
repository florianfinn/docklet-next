// Source: docklet-next/issues/492
// Revision: 54fd54b6bf259e09c1d4fa66ab1cd7ebdebf09e6
// Retrieved: 2026-11-20
// License: Apache-2.0
// Deviations: Original resolver execution, recovery and verification.

package resolver_test

import (
	"context"
	"errors"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/proxy/traefik"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	"github.com/moby/moby/api/types/mount"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	compose "github.com/compose-spec/compose-go/v2/types"
	"github.com/florianfinn/docklet-next/backend/internal/proxy/resolver"
	dto "github.com/florianfinn/docklet-next/types/proxy"
	"github.com/moby/moby/api/types/container"
	"github.com/stretchr/testify/require"
)

func TestR17ReplacementBinding(t *testing.T) {
	for _, branch := range []string{"match", "other-source", "other-kind", "other-target", "zero", "multiple"} {
		t.Run(branch, func(t *testing.T) {
			old := dto.Instance{Identity: dto.Identity{ContainerID: "old"}, Source: dto.Source{Canonical: "/example/project/config", MountTarget: "/etc/traefik", Kind: dto.ProjectSource}}
			rows := []container.Summary{{ID: "new", State: "running", Labels: map[string]string{"com.docker.compose.project": "example", "com.docker.compose.service": "gateway"}}}
			if branch == "zero" {
				rows = nil
			}
			if branch == "multiple" {
				rows = append(rows, rows[0])
			}
			runtime := &resolver.Runtime{List: func(context.Context) ([]container.Summary, error) { return rows, nil }, Inspect: func(context.Context, string) (container.InspectResponse, error) {
				target := "/etc/traefik"
				if branch == "other-target" {
					target = "/other"
				}
				return container.InspectResponse{ID: "new", Name: "/example-gateway", State: &container.State{Running: true, StartedAt: "2026-11-20T02:00:00Z"}, Config: &container.Config{}, Mounts: []container.MountPoint{{Destination: target}}}, nil
			}, Source: func(context.Context, container.InspectResponse, container.MountPoint) (dto.Source, error) {
				source := old.Source
				if branch == "other-source" {
					source.Canonical = "/other/project"
				}
				if branch == "other-kind" {
					source.Kind = dto.BindSource
				}
				return source, nil
			}}
			current, err := runtime.Replacement(t.Context(), resolver.Target{Project: "example", Service: "gateway"}, old)
			if branch == "match" {
				require.NoError(t, err)
				require.Equal(t, "new", current.Identity.ContainerID)
				require.Equal(t, old.Source, current.Source)
			} else {
				require.Error(t, err)
				require.Equal(t, dto.IdentityUnclear, err.(*dto.Problem).Code)
			}
		})
	}
}
func TestR2StartupObserverPatternsAndOldInventory(t *testing.T) {
	for _, branch := range []string{"clean", "startup-error", "acme-error", "stopped", "logs-unavailable", "restored-present", "restored-absent", "timer-boundary", "inventory-epoch"} {
		t.Run(branch, func(t *testing.T) {
			now := time.Date(2026, 11, 20, 2, 0, 0, 0, time.UTC)
			instance := dto.Instance{Identity: dto.Identity{ContainerID: "new", StartedAt: now.Format(time.RFC3339Nano)}}
			runtime := &resolver.Runtime{Now: func() time.Time { now = now.Add(8 * time.Second); return now }, Wait: func(context.Context, time.Duration) error {
				if branch == "timer-boundary" {
					now = now.Add(20 * time.Second)
					return context.DeadlineExceeded
				}
				return nil
			}, Inspect: func(context.Context, string) (container.InspectResponse, error) {
				return container.InspectResponse{ID: instance.Identity.ContainerID, State: &container.State{Running: branch != "stopped", StartedAt: instance.Identity.StartedAt}}, nil
			}, Logs: func(ctx context.Context, i dto.Instance, since time.Time, consume func(string) error) error {
				require.Equal(t, instance.Identity, i.Identity)
				if branch == "logs-unavailable" {
					return errors.New("unreadable")
				}
				if branch == "startup-error" {
					return consume("command traefik error: field not found")
				}
				return consume("providerName=Example.acme error=unreachable authority")
			}, Inventory: func(context.Context, dto.Instance) (dto.Inventory, error) {
				names := []string{"Example"}
				if branch == "restored-absent" {
					names = nil
				}
				identity := instance.Identity
				if branch == "inventory-epoch" {
					identity.StartedAt = "2026-11-20T03:00:00Z"
				}
				return dto.Inventory{Identity: identity, Eligibility: dto.Eligibility{Writable: true}, Resolvers: names}, nil
			}}
			present := branch != "restored-present" && branch != "restored-absent"
			err := runtime.Observe(t.Context(), instance, "Example", present, time.Date(2026, 11, 20, 1, 59, 59, 0, time.UTC))
			if branch == "clean" || branch == "acme-error" || branch == "restored-absent" || branch == "timer-boundary" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}
func TestR4SettleRejectsTransitionalEffect(t *testing.T) {
	rows := []container.Summary{{ID: "new", State: "restarting", Labels: map[string]string{"com.docker.compose.project": "example", "com.docker.compose.service": "gateway"}}}
	runtime := &resolver.Runtime{List: func(context.Context) ([]container.Summary, error) { return rows, nil }, Inspect: func(context.Context, string) (container.InspectResponse, error) {
		return container.InspectResponse{}, nil
	}, Wait: func(context.Context, time.Duration) error { return context.DeadlineExceeded }}
	err := runtime.Settle(t.Context(), resolver.Target{Project: "example", Service: "gateway"})
	require.Error(t, err)
	require.Equal(t, dto.EffectUnsettled, err.(*dto.Problem).Code)
}
func TestScopedRecreateHasNoBuildPullOrDependencies(t *testing.T) {
	model := &compose.Project{Name: "example", WorkingDir: filepath.Join(t.TempDir(), "project"), Services: compose.Services{"gateway": {Name: "gateway", Image: "traefik:v3.7.16", PullPolicy: "always", Build: &compose.BuildConfig{Context: "."}, DependsOn: compose.DependsOnConfig{"app": {Condition: "service_started"}}}, "app": {Name: "app", Image: "example/app:1.0.0"}}}
	scoped, err := resolver.Scoped(model, "gateway")
	require.NoError(t, err)
	require.Len(t, scoped.Services, 1)
	require.Nil(t, scoped.Services["gateway"].Build)
	require.Equal(t, "never", scoped.Services["gateway"].PullPolicy)
	require.Empty(t, scoped.Services["gateway"].DependsOn)
	require.NotNil(t, model.Services["gateway"].Build)
}

func TestR8RealAdmissionLayouts(t *testing.T) {
	for _, branch := range []string{"flags", "file", "unmanaged", "replicas", "overrides", "include", "extends", "anchor", "env-selection", "symlink-env", "namespace", "toml", "entrypoint", "scalar", "environment", "mixed", "outside", "storage", "nested-storage", "shadow-static", "implicit-file", "command-mismatch", "network-owner", "volumes-from"} {
		t.Run(branch, func(t *testing.T) {
			v := target(t, branch == "file" || branch == "toml" || branch == "outside" || branch == "mixed" || branch == "implicit-file")
			q := request()
			dir := v.Directory
			db, err := database.Initialize(t.Context(), sqlitestore.URL(filepath.Join(t.TempDir(), "runtime.db")), database.MigrationOptions{})
			require.NoError(t, err)
			defer db.Close()
			require.NoError(t, db.Exec("INSERT INTO projects(id,name,path,status,service_count,running_count,is_archived) VALUES(?,?,?,?,?,?,?)", "project-one", "example", dir, "running", 1, 1, false).Error)
			source := dto.Source{Kind: dto.ProjectSource, Canonical: dir, MountTarget: "/etc/traefik"}
			labels := map[string]string{"com.docker.compose.project": "example", "com.docker.compose.service": "gateway"}
			rows := []container.Summary{{ID: "original", State: "running", Labels: labels}}
			inspected := container.InspectResponse{ID: "original", Name: "/example-gateway", State: &container.State{Running: true, StartedAt: "2026-11-20T01:00:00Z"}, Config: &container.Config{Labels: labels, Cmd: []string{"--entrypoints.web.address=:80", "--providers.file.directory=/etc/traefik/routes"}}, Mounts: []container.MountPoint{{Type: mount.TypeBind, Source: dir, Destination: "/etc/traefik", RW: true}, {Type: mount.TypeBind, Source: dir, Destination: "/certs", RW: true}}}
			inv := inventory()
			content := composeText
			if v.Static != "" {
				inv.RootFile = "/etc/traefik/" + v.Static
				inspected.Config.Cmd = []string{"--configfile=" + inv.RootFile}
				content = strings.Replace(composeText, "      - --entrypoints.web.address=:80 # listener\n      - --providers.file.directory=/etc/traefik/routes", "      - --configfile="+inv.RootFile, 1)
			}
			switch branch {
			case "implicit-file":
				// No compose command: the image default runs and Traefik finds the file itself.
				b, readErr := os.ReadFile(filepath.Join(dir, v.Static))
				require.NoError(t, readErr)
				require.NoError(t, os.WriteFile(filepath.Join(dir, "traefik.yml"), b, 0644))
				inv.RootFile = "/etc/traefik/traefik.yml"
				inspected.Config.Cmd = []string{"traefik"}
				content = strings.Replace(content, "    command:\n      - --configfile=/etc/traefik/"+v.Static+"\n", "", 1)
			case "command-mismatch":
				inspected.Config.Cmd = []string{"--entrypoints.web.address=:8080"}
			case "network-owner":
				content = strings.Replace(content, "    command:", "    network_mode: service:app\n    command:", 1)
			case "volumes-from":
				content = strings.Replace(content, "    command:", "    volumes_from: [app]\n    command:", 1)
			case "unmanaged":
				require.NoError(t, db.Exec("DELETE FROM projects").Error)
			case "replicas":
				rows = append(rows, rows[0])
			case "overrides":
				require.NoError(t, os.WriteFile(filepath.Join(dir, "compose.override.yaml"), []byte("services: {}\n"), 0644))
			case "include":
				content = "include: [other.yaml]\n" + content
			case "extends":
				content = strings.Replace(content, "    command:", "    extends: other\n    command:", 1)
			case "anchor":
				content = strings.Replace(content, "image: traefik:v3.7.16", "image: &base traefik:v3.7.16", 1)
			case "env-selection":
				require.NoError(t, os.WriteFile(filepath.Join(dir, ".env"), []byte("COMPOSE_FILE=other.yaml\n"), 0600))
			case "symlink-env":
				require.NoError(t, os.Remove(filepath.Join(dir, ".env")))
				require.NoError(t, os.Symlink("other", filepath.Join(dir, ".env")))
			case "namespace":
				content = strings.Replace(content, "  app:\n", "  app:\n    network_mode: service:gateway\n", 1)
			case "toml":
				inv.Eligibility = dto.Eligibility{Reason: dto.UnsupportedFormat}
				inv.RootFile = "/etc/traefik/static.toml"
				require.NoError(t, os.WriteFile(filepath.Join(dir, "static.toml"), []byte("[entryPoints.web]\naddress=':80'\n"), 0644))
				inspected.Config.Cmd = []string{"--configfile=" + inv.RootFile}
				content = strings.Replace(content, "--configfile=/etc/traefik/config/static.yaml", "--configfile="+inv.RootFile, 1)
			case "entrypoint":
				inspected.Config.Entrypoint = []string{"traefik", "--api=true"}
			case "scalar":
				content = strings.Replace(content, "command:\n      - --entrypoints.web.address=:80 # listener\n      - --providers.file.directory=/etc/traefik/routes", "command: --entrypoints.web.address=:80", 1)
				inspected.Config.Cmd = []string{"--entrypoints.web.address=:80"}
			case "environment":
				inspected.Config.Env = []string{"TRAEFIK_API=true"}
			case "mixed":
				inspected.Config.Cmd = append(inspected.Config.Cmd, "--api=true")
				content = strings.Replace(content, "--configfile="+inv.RootFile, "--configfile="+inv.RootFile+"\n      - --api=true", 1)
			case "outside":
				inv.RootFile = "/other/static.yaml"
				inspected.Config.Cmd = []string{"--configfile=" + inv.RootFile}
				content = strings.Replace(content, "--configfile=/etc/traefik/config/static.yaml", "--configfile="+inv.RootFile, 1)
			case "shadow-static":
				inv.RootFile = "/etc/traefik/config/static.yaml"
				inspected.Config.Cmd = []string{"--configfile=" + inv.RootFile}
				content = strings.Replace(content, "      - --entrypoints.web.address=:80 # listener\n      - --providers.file.directory=/etc/traefik/routes", "      - --configfile="+inv.RootFile, 1)
				inspected.Mounts = append(inspected.Mounts, container.MountPoint{Type: mount.TypeBind, Source: t.TempDir(), Destination: "/etc/traefik/config", RW: true})
			case "nested-storage":
				q.Storage = "/certs/missing/acme.json"
			case "storage":
				inspected.Mounts[1].RW = false
			}
			require.NoError(t, os.WriteFile(filepath.Join(dir, "compose.yaml"), []byte(content), 0644))
			runtime := &resolver.Runtime{DB: db.DB, ProjectsDirectory: filepath.Dir(dir), Splice: traefik.Splice, Inspect: func(context.Context, string) (container.InspectResponse, error) { return inspected, nil }, List: func(context.Context) ([]container.Summary, error) { return rows, nil }, Source: func(context.Context, container.InspectResponse, container.MountPoint) (dto.Source, error) {
				return source, nil
			}, Inventory: func(context.Context, dto.Instance) (dto.Inventory, error) { return inv, nil }}
			_, err = runtime.Prepare(t.Context(), dto.ResolverApplyRequest{Instance: dto.Instance{Kind: dto.Traefik, Identity: dto.Identity{ContainerID: "original"}, Source: source}, Request: q})
			if branch == "file" || branch == "flags" || branch == "implicit-file" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
			if branch == "network-owner" || branch == "volumes-from" {
				var refused *resolver.LayoutError
				require.ErrorAs(t, err, &refused)
				require.Equal(t, dto.NamespaceDependent, refused.Reason)
			}
			if branch == "command-mismatch" {
				requireCode(t, err, dto.TargetUnclear)
			}
			if branch == "toml" {
				var refused *resolver.LayoutError
				require.ErrorAs(t, err, &refused)
				require.Equal(t, dto.TOMLStatic, refused.Reason)
			}
			actual, e := os.ReadFile(filepath.Join(dir, "compose.yaml"))
			require.NoError(t, e)
			require.Equal(t, content, string(actual))
			if branch == "file" || branch == "flags" {
				source.ProjectPath = dir
				runtime.Inspect = func(context.Context, string) (container.InspectResponse, error) {
					t.Fatal("confirmation must not call Docker")
					return container.InspectResponse{}, nil
				}
				files := []dto.ResolverFile{}
				for _, file := range v.Files {
					files = append(files, file.ResolverFile)
				}
				current, e := runtime.Confirmation(t.Context(), dto.Instance{Source: source}, files)
				require.NoError(t, e)
				state, e := resolver.Current(current)
				require.NoError(t, e)
				require.True(t, state.Present)
				source.Canonical = "/example/other-project"
				_, e = runtime.Confirmation(t.Context(), dto.Instance{Source: source}, files)
				require.Error(t, e)
			}
		})
	}
}

func TestScopedRecreateUsesTheExportedComposePort(t *testing.T) {
	v := target(t, false)
	calls := 0
	runtime := &resolver.Runtime{ProjectsDirectory: filepath.Dir(v.Directory), Up: func(ctx context.Context, model *compose.Project, services []string, orphans, force, volumes bool, bound time.Duration) error {
		calls++
		require.Equal(t, []string{v.Service}, services)
		require.Len(t, model.Services, 1)
		require.Equal(t, "never", model.Services[v.Service].PullPolicy)
		require.Nil(t, model.Services[v.Service].Build)
		require.Empty(t, model.Services[v.Service].DependsOn)
		require.False(t, orphans)
		require.True(t, force)
		require.False(t, volumes)
		require.Equal(t, 60*time.Second, bound)
		deadline, ok := ctx.Deadline()
		require.True(t, ok)
		require.LessOrEqual(t, time.Until(deadline), 60*time.Second)
		return nil
	}}
	require.NoError(t, runtime.Recreate(t.Context(), v))
	require.Equal(t, 1, calls)
}
