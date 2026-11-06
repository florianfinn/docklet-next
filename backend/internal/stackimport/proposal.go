// Source: docklet-next/issues/348
// Revision: b5068aff3da7eac11f208d1f4d8c9068d4a8c073
// Retrieved: 2026-11-06
// License: Apache-2.0
// Deviations: Original bounded one-time stack import implementation.

package stackimport

import (
	"bytes"
	"context"
	"errors"
	"io"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/compose-spec/compose-go/v2/loader"
	compose "github.com/compose-spec/compose-go/v2/types"
	dto "github.com/florianfinn/docklet-next/types/stackimport"
	"gopkg.in/yaml.v3"
)

var yamlLine = regexp.MustCompile(`line ([0-9]+)`)
var envName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
var interpolation = regexp.MustCompile(`\$\$|\$\{([A-Za-z_][A-Za-z0-9_]*)(:?[-+?])?|\$([A-Za-z_][A-Za-z0-9_]*)`)

// parseProposal validates only the supplied bytes. External requirements stay in
// the original compose content and are reported without reading or resolving them.
func parseProposal(ctx context.Context, data []byte) (dto.Result, error) {
	result := dto.Result{ComposeContent: string(data), EnvNames: []string{}, Found: []string{"import.found.compose"}, Missing: []string{}}
	invalid := func(err error) (dto.Result, error) {
		problem := dto.Failure("import.invalid-compose", 422)
		if err != nil {
			if m := yamlLine.FindStringSubmatch(err.Error()); m != nil {
				problem.Line, _ = strconv.Atoi(m[1])
			}
		}
		return dto.Result{}, problem
	}
	if len(data) > fileLimit {
		return dto.Result{}, dto.Failure("import.too-large", 413)
	}
	if !utf8.Valid(data) {
		return invalid(nil)
	}
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	var model map[string]any
	if err := decoder.Decode(&model); err != nil {
		return invalid(err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return invalid(err)
	}
	services, ok := model["services"].(map[string]any)
	if !ok || len(services) == 0 {
		return invalid(nil)
	}
	addMissing := func(code string) { result.Missing = append(result.Missing, "import.missing."+code) }
	if _, ok := model["include"]; ok {
		addMissing("include")
	}
	for _, value := range services {
		service, ok := value.(map[string]any)
		if !ok {
			return invalid(nil)
		}
		for key, code := range map[string]string{"env_file": "env-file", "label_file": "label-file", "extends": "extends"} {
			if _, ok := service[key]; ok {
				addMissing(code)
			}
		}
		build := service["build"]
		if b, ok := build.(map[string]any); ok {
			build = b["context"]
			if build == nil {
				build = "."
			}
		}
		if b, ok := build.(string); ok && !path.IsAbs(b) && !strings.Contains(b, "://") && !strings.HasPrefix(b, "git@") {
			addMissing("build-context")
		}
	}
	collectVariables(model, &result.EnvNames)
	if len(result.EnvNames) > 0 {
		addMissing("variables")
	}
	// Config is supplied in memory as well as Content; no file is opened for the source.
	_, err := loader.LoadWithContext(ctx, compose.ConfigDetails{Environment: compose.Mapping{}, ConfigFiles: []compose.ConfigFile{{Filename: "proposal.yaml", Content: data, Config: model}}}, func(o *loader.Options) {
		o.SetProjectName("proposal", true)
		o.SkipInterpolation = true
		o.SkipInclude = true
		o.SkipExtends = true
		o.SkipResolveEnvironment = true
		o.SkipResolveLabels = true
		o.SkipConsistencyCheck = true
		o.ResolvePaths = false
	})
	if err != nil {
		return invalid(err)
	}
	result.EnvNames = uniqueSorted(result.EnvNames)
	result.Missing = uniqueSorted(result.Missing)
	return result, nil
}

func collectVariables(value any, names *[]string) {
	switch v := value.(type) {
	case string:
		for _, m := range interpolation.FindAllStringSubmatch(v, -1) {
			if m[0] == "$$" {
				continue
			}
			name := m[1]
			if name == "" {
				name = m[3]
			}
			if m[2] != "-" && m[2] != ":-" && m[2] != "+" && m[2] != ":+" {
				*names = append(*names, name)
			}
		}
	case map[string]any:
		for _, entry := range v {
			collectVariables(entry, names)
		}
	case []any:
		for _, entry := range v {
			collectVariables(entry, names)
		}
	}
}
func envExampleNames(data []byte) []string {
	names := []string{}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "export ") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		}
		key, _, ok := strings.Cut(line, "=")
		key = strings.TrimSpace(key)
		if ok && envName.MatchString(key) {
			names = append(names, key)
		}
	}
	return uniqueSorted(names)
}
func uniqueSorted(values []string) []string { slices.Sort(values); return slices.Compact(values) }
