// Source: docklet-next/issues/439
// Revision: 910032a54830f5ce1d37070964fa2dc7efb8aa2f
// Retrieved: 2026-11-09
// License: Apache-2.0
// Deviations: Original Compose-faithful self-update implementation.

package selfupdate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/docker/compose/v5/pkg/api"
	"github.com/stretchr/testify/require"
)

func replayFixture(t *testing.T) (Replay, []byte) {
	t.Helper()
	dir := t.TempDir()
	old := []byte("INPUT=example-value\n# HUB_IMAGE is supplied by the package default\n")
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".env"), old, 0640))
	file := filepath.Join(dir, "compose.yaml")
	require.NoError(t, os.WriteFile(file, []byte(`name: yaml-name
services:
  hub:
    image: ${HUB_IMAGE:-example.com/docklet-hub:1.0.0}
    environment:
      INPUT: ${INPUT:?required}
    volumes:
      - ./data:/app/data
    depends_on: [neighbour]
  neighbour:
    image: example.com/neighbour:1.0.0
`), 0640))
	return Replay{Project: "label-name", Service: "hub", WorkingDir: dir, ConfigFiles: []string{file}, EnvFile: filepath.Join(dir, ".env"), Variable: "HUB_IMAGE", CurrentReference: "example.com/docklet-hub:1.0.0"}, old
}
func TestReplayProfile(t *testing.T) {
	for _, name := range []string{"implicit env", "explicit env", "override", "image overridden", "required shell value", "symlink env", "outside include", "relative env-file outside", "symlink config", "multiple env"} {
		t.Run(name, func(t *testing.T) {
			r, old := replayFixture(t)
			want := true
			switch name {
			case "explicit env":
				next := filepath.Join(r.WorkingDir, "install.env")
				require.NoError(t, os.Rename(r.EnvFile, next))
				r.EnvFile = next
			case "override", "image overridden":
				extra := "services:\n  hub:\n    hostname: preserved\n"
				if name == "image overridden" {
					extra = "services:\n  hub:\n    image: example.com/other:1.0.0\n"
					want = false
					r.CurrentReference = "example.com/other:1.0.0"
				}
				file := filepath.Join(r.WorkingDir, "override.yaml")
				require.NoError(t, os.WriteFile(file, []byte(extra), 0640))
				r.ConfigFiles = append(r.ConfigFiles, file)
			case "required shell value":
				require.NoError(t, os.WriteFile(r.EnvFile, []byte(""), 0640))
				t.Setenv("INPUT", "must-not-be-inherited")
				want = false
			case "symlink env":
				file := r.EnvFile + ".real"
				require.NoError(t, os.Rename(r.EnvFile, file))
				require.NoError(t, os.Symlink(file, r.EnvFile))
				want = false
			case "outside include":
				f := r.ConfigFiles[0]
				raw, e := os.ReadFile(f)
				require.NoError(t, e)
				require.NoError(t, os.WriteFile(f, append(raw, []byte("include: ../outside.yaml\n")...), 0640))
				want = false
			case "relative env-file outside":
				f := r.ConfigFiles[0]
				raw, e := os.ReadFile(f)
				require.NoError(t, e)
				raw = []byte(strings.Replace(string(raw), "    depends_on: [neighbour]", "    depends_on: [neighbour]\n    env_file: ../outside.env", 1))
				require.NoError(t, os.WriteFile(f, raw, 0640))
				want = false
			case "symlink config":
				f := r.ConfigFiles[0]
				require.NoError(t, os.Rename(f, f+".real"))
				require.NoError(t, os.Symlink(f+".real", f))
				want = false
			case "multiple env":
				r.EnvFile += ",second.env"
				want = false
			}
			before, _ := os.ReadFile(r.EnvFile)
			project, err := r.DryRun(t.Context())
			if !want {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, "label-name", project.Name)
				require.Len(t, project.Services, 1)
				require.Equal(t, filepath.Join(r.WorkingDir, "data"), project.Services["hub"].Volumes[0].Source)
				require.Equal(t, "example-value", *project.Services["hub"].Environment["INPUT"])
				require.Equal(t, r.EnvFile, project.Services["hub"].CustomLabels[api.EnvironmentFileLabel])
				target, err := r.Target(t.Context(), "example.com/docklet-hub:1.0.1")
				require.NoError(t, err)
				require.Equal(t, "example.com/docklet-hub:1.0.1", target.Services["hub"].Image)
			}
			after, _ := os.ReadFile(r.EnvFile)
			require.Equal(t, before, after, "dry run wrote env")
			_ = old
		})
	}
}
func TestReplayLabelsAndAbsentPin(t *testing.T) {
	r, _ := replayFixture(t)
	labels := map[string]string{api.ProjectLabel: r.Project, api.ServiceLabel: r.Service, api.WorkingDirLabel: r.WorkingDir, api.ConfigFilesLabel: r.ConfigFiles[0]}
	got, err := ReplayFromLabels(labels, "hub", r.CurrentReference)
	require.NoError(t, err)
	require.Equal(t, r, got)
	labels[api.EnvironmentFileLabel] = "a,b"
	_, err = ReplayFromLabels(labels, "hub", r.CurrentReference)
	require.Error(t, err)
	// Paths outside the project, relative or unclean are refused from the labels
	// alone, before the action is offered.
	for _, tc := range []struct{ env, config string }{
		{env: "/etc/example/hub.env"},
		{config: "/srv/other/compose.yaml"},
		{config: "compose.yaml"},
		{env: r.WorkingDir + "/../outside.env"},
	} {
		bad := map[string]string{api.ProjectLabel: r.Project, api.ServiceLabel: r.Service, api.WorkingDirLabel: r.WorkingDir, api.ConfigFilesLabel: r.ConfigFiles[0], api.EnvironmentFileLabel: tc.env}
		if tc.config != "" {
			bad[api.ConfigFilesLabel] = r.ConfigFiles[0] + "," + tc.config
		}
		_, err = ReplayFromLabels(bad, "hub", r.CurrentReference)
		require.ErrorContains(t, err, CodeLayout, tc)
	}
	pin := writePin([]byte("# HUB_IMAGE=example\nOTHER=value\n"), "HUB_IMAGE", "example.com/hub:1.0.1")
	require.Contains(t, string(pin), "HUB_IMAGE=example.com/hub:1.0.1\n")
	require.Contains(t, string(pin), "OTHER=value")
	pin = writePin([]byte("HUB_IMAGE=old\nexport HUB_IMAGE=duplicate\nOTHER=value\n"), "HUB_IMAGE", "new")
	require.Equal(t, "OTHER=value\nHUB_IMAGE=new\n", string(pin))
}
