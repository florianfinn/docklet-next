// Source: docklet-next/issues/439
// Revision: 910032a54830f5ce1d37070964fa2dc7efb8aa2f
// Retrieved: 2026-11-09
// License: Apache-2.0
// Deviations: Original Compose-faithful self-update implementation.

package selfupdate

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/compose-spec/compose-go/v2/dotenv"
	"github.com/compose-spec/compose-go/v2/loader"
	composetypes "github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/compose/v5/pkg/api"
	"go.yaml.in/yaml/v4"
)

// Replay contains only daemon-host paths taken from the current service labels.
type Replay struct {
	Project          string   `json:"project"`
	Service          string   `json:"service"`
	WorkingDir       string   `json:"workingDir"`
	ConfigFiles      []string `json:"configFiles"`
	EnvFile          string   `json:"envFile"`
	Variable         string   `json:"variable"`
	CurrentReference string   `json:"currentReference"`
}

func ReplayFromLabels(labels map[string]string, role, current string) (Replay, error) {
	r := Replay{Project: labels[api.ProjectLabel], Service: labels[api.ServiceLabel], WorkingDir: labels[api.WorkingDirLabel], CurrentReference: current, Variable: "HUB_IMAGE"}
	if role == "agent" {
		r.Variable = "DOCKLET_AGENT_IMAGE"
	}
	if r.Project == "" || r.Service == "" || r.WorkingDir == "" || labels[api.ConfigFilesLabel] == "" {
		return r, Refuse(CodeNotCompose)
	}
	r.ConfigFiles = strings.Split(labels[api.ConfigFilesLabel], ",")
	r.EnvFile = labels[api.EnvironmentFileLabel]
	if r.EnvFile == "" {
		r.EnvFile = filepath.Join(r.WorkingDir, ".env")
	}
	if strings.Contains(r.EnvFile, ",") || !filepath.IsAbs(r.WorkingDir) {
		return r, Refuse(CodeLayout)
	}
	return r, nil
}

// regularInside refuses symlinks in every component, not only in the final file.
func regularInside(root, path string) error {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return Refuse(CodeLayout)
	}
	relative, err := filepath.Rel(root, path)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(os.PathSeparator)) {
		return Refuse(CodeLayout)
	}
	for p := path; ; p = filepath.Dir(p) {
		info, err := os.Lstat(p)
		if err != nil || info.Mode()&os.ModeSymlink != 0 {
			return Refuse(CodeLayout)
		}
		if p == path && !info.Mode().IsRegular() {
			return Refuse(CodeLayout)
		}
		if p == filepath.Dir(p) {
			break
		}
	}
	return nil
}

func (r Replay) load(ctx context.Context, override string) (*composetypes.Project, error) {
	if r.Project == "" || r.Service == "" || len(r.ConfigFiles) == 0 || (r.Variable != "HUB_IMAGE" && r.Variable != "DOCKLET_AGENT_IMAGE") {
		return nil, Refuse(CodeLayout)
	}
	if err := regularInside(r.WorkingDir, r.EnvFile); err != nil {
		return nil, err
	}
	env, err := dotenv.GetEnvFromFile(map[string]string{}, []string{r.EnvFile})
	if err != nil {
		return nil, Refuse(CodeLayout)
	}
	if override != "" {
		env[r.Variable] = override
	}
	files := make([]composetypes.ConfigFile, 0, len(r.ConfigFiles))
	for _, path := range r.ConfigFiles {
		if err := regularInside(r.WorkingDir, path); err != nil {
			return nil, err
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		// Nested models may introduce independent interpolation environments. This
		// package replay profile deliberately supports only the recorded file list.
		var model map[string]any
		if yaml.Unmarshal(raw, &model) != nil || model["include"] != nil {
			return nil, Refuse(CodeLayout)
		}
		if services, ok := model["services"].(map[string]any); ok {
			for _, value := range services {
				if service, ok := value.(map[string]any); ok && service["extends"] != nil {
					return nil, Refuse(CodeLayout)
				}
			}
		}
		files = append(files, composetypes.ConfigFile{Filename: path, Content: raw})
	}
	p, err := loader.LoadWithContext(ctx, composetypes.ConfigDetails{WorkingDir: r.WorkingDir, ConfigFiles: files, Environment: env}, func(o *loader.Options) {
		o.SetProjectName(r.Project, true)
		o.SkipResolveEnvironment = true
		o.SkipResolveLabels = true
	})
	if err != nil {
		return nil, fmt.Errorf("%w: %v", Refuse(CodeLayout), err)
	}
	selected, err := p.WithSelectedServices([]string{r.Service}, composetypes.IgnoreDependencies)
	if err != nil {
		return nil, Refuse(CodeLayout)
	}
	for _, service := range selected.Services {
		for _, file := range service.EnvFiles {
			if err := regularInside(r.WorkingDir, file.Path); err != nil {
				return nil, err
			}
		}
		for _, file := range service.LabelFiles {
			if err := regularInside(r.WorkingDir, file); err != nil {
				return nil, err
			}
		}
	}
	selected, err = selected.WithServicesEnvironmentResolved(true)
	if err != nil {
		return nil, Refuse(CodeLayout)
	}
	selected, err = selected.WithServicesLabelsResolved(true)
	if err != nil {
		return nil, Refuse(CodeLayout)
	}
	for name, service := range selected.Services {
		service.Build = nil
		service.PullPolicy = composetypes.PullPolicyNever
		service.CustomLabels = map[string]string{api.ProjectLabel: r.Project, api.ServiceLabel: name, api.WorkingDirLabel: r.WorkingDir, api.ConfigFilesLabel: strings.Join(r.ConfigFiles, ","), api.EnvironmentFileLabel: r.EnvFile, api.OneoffLabel: "False", api.VersionLabel: api.ComposeVersion}
		selected.Services[name] = service
	}
	return selected, nil
}

// DryRun proves the effective merged image is controlled by the package variable.
// Process environment and RepoDigests cannot influence the proof.
func (r Replay) DryRun(ctx context.Context) (*composetypes.Project, error) {
	p, err := r.load(ctx, "")
	if err != nil {
		return nil, err
	}
	if p.Services[r.Service].Image != r.CurrentReference {
		return nil, Refuse(CodeLayout)
	}
	sentinel := "example.com/docklet-probe:0.0.0"
	probe, err := r.load(ctx, sentinel)
	if err != nil || probe.Services[r.Service].Image != sentinel {
		return nil, Refuse(CodeLayout)
	}
	return p, nil
}
func (r Replay) Target(ctx context.Context, target string) (*composetypes.Project, error) {
	p, err := r.load(ctx, target)
	if err != nil {
		return nil, err
	}
	if p.Services[r.Service].Image != target {
		return nil, Refuse(CodeLayout)
	}
	return p, nil
}
