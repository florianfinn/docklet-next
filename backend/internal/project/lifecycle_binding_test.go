// Source: docklet-next/issues/594
// Revision: 48d8f45d867a7a41e571b59382b291c350d3ce42
// Retrieved: 2026-11-24
// License: Apache-2.0
// Deviations: Original reason producers and read-only eligibility regression proofs.

package project

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	life "github.com/florianfinn/docklet-next/backend/internal/project/children/lifecycle"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	dto "github.com/florianfinn/docklet-next/types/updater"
	"github.com/stretchr/testify/require"
)

func TestImageUpdateEligibilityReadOnlyReasons(t *testing.T) {
	for _, tc := range []struct{ name, source, reason, service, variable string }{
		{"literal", lifecycleOriginal, "", "", ""},
		{"variable", strings.Replace(lifecycleOriginal, "registry.example.com/app:1.0.0", "${SERVICE_IMAGE:-registry.example.com/app:1.0.0}", 1), "variable-reference", "app", "SERVICE_IMAGE"},
		{"digest", strings.Replace(lifecycleOriginal, "registry.example.com/app:1.0.0", "registry.example.com/app@sha256:"+strings.Repeat("a", 64), 1), "digest-pinned", "app", ""},
		{"build", strings.Replace(lifecycleOriginal, "    image:", "    build: .\n    image:", 1), "build-service", "app", ""},
		{"include", "include: [included.yaml]\n" + lifecycleOriginal, "include-extends", "", ""},
		{"extends", strings.Replace(lifecycleOriginal, "    image:", "    extends: {file: included.yaml, service: app}\n    image:", 1), "include-extends", "", ""},
		{"unparsable", "services: [", "unparsable", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newLifecycleFixture(t)
			path := filepath.Join(f.project.Path, "compose.yaml")
			require.NoError(t, os.WriteFile(path, []byte(tc.source), 0600))
			before, err := os.ReadFile(path)
			require.NoError(t, err)
			result, err := f.service.ImageUpdateEligibility(t.Context(), f.project.ID)
			require.NoError(t, err)
			var binding *imageDTO.Binding
			if result.Project != nil {
				binding = result.Project
			} else {
				for _, entry := range result.Services {
					if entry.Service == "app" {
						require.Equal(t, tc.reason == "", entry.Eligible)
						binding = entry.Binding
					}
				}
			}
			if tc.reason == "" {
				require.Nil(t, binding)
			} else {
				require.NotNil(t, binding)
				require.Equal(t, tc.reason, binding.Reason)
				require.Equal(t, tc.service, binding.Service)
				require.Equal(t, tc.variable, binding.Variable)
				require.Equal(t, map[bool]string{true: "service", false: "project"}[tc.service != ""], binding.Scope)
			}
			after, err := os.ReadFile(path)
			require.NoError(t, err)
			require.Equal(t, before, after)
			require.Zero(t, f.pulls)
			require.Zero(t, f.deploys)
			require.Zero(t, f.tagWrites)
			for _, table := range []string{"image_lifecycle_runs", "restore_points", "deployment_baselines"} {
				require.True(t, f.db.Migrator().HasTable(table))
				var n int64
				require.NoError(t, f.db.Table(table).Count(&n).Error)
				require.Zero(t, n)
			}
		})
	}
}

func TestBindingInputAndWriterReasons(t *testing.T) {
	t.Run("unwritable-file", func(t *testing.T) {
		f := newLifecycleFixture(t)
		path := filepath.Join(f.project.Path, "compose.yaml")
		require.NoError(t, os.Chmod(path, 0400))
		result, err := f.service.ImageUpdateEligibility(t.Context(), f.project.ID)
		require.NoError(t, err)
		require.NotNil(t, result.Project)
		require.Equal(t, "unwritable-file", result.Project.Reason)
	})
	for _, tc := range []struct {
		reason string
		change func(*life.Snapshot)
	}{
		{"file-missing", func(s *life.Snapshot) { s.Files = nil }},
		{"shared-image-anchor", func(s *life.Snapshot) {
			for i := range s.Files {
				if s.Files[i].Path != "compose.yaml" {
					continue
				}
				s.Files[i].Content = []byte("services:\n  app:\n    image: &shared registry.example.com/app:1.0.0\n  peer:\n    image: *shared\n")
			}
		}},
	} {
		t.Run(tc.reason, func(t *testing.T) {
			f := newLifecycleFixture(t)
			snap, err := f.service.lifecycleInputs(context.Background(), f.project)
			require.NoError(t, err)
			tc.change(snap)
			_, err = lifecycleVersionFiles(snap, []dto.ImageSelection{{Service: "app", SourceReference: "registry.example.com/app:1.0.0", Reference: "registry.example.com/app:2.0.0", Kind: "version"}})
			var problem *imageDTO.OperationError
			require.ErrorAs(t, err, &problem)
			require.NotNil(t, problem.Problem.Binding)
			require.Equal(t, tc.reason, problem.Problem.Binding.Reason)
		})
	}
}

// The refusal code appears only at reviewed sites: the two constructors that
// attach a binding, the relay and sender comparisons, and the AUTO-8 deploy
// point codes. Every bindingFailure call passes a declared literal reason.
func TestBindingRefusalConstructionGuard(t *testing.T) {
	const code = "update.image-binding-unsupported"
	allowed := map[string]int{
		"project/lifecycle_binding.go:bindingFailure":             1,
		"project/lifecycle_deploy.go:PrepareDeployPoint":          3,
		"updater/check_relay.go:relayedCheckRefusal":              1,
		"activity/runtime_layout_refusal.go:const":                2,
		"activity/runtime_layout_refusal.go:runtimeTargetFailure": 1,
		"activity/runtime_layout_refusal.go:runtimeLayoutBinding": 1,
		"activity/runtime_sender.go:metadata":                     1,
	}
	field, ok := reflect.TypeFor[imageDTO.Binding]().FieldByName("Reason")
	require.True(t, ok)
	reasons := strings.Split(field.Tag.Get("enum"), ",")
	found := map[string]int{}
	err := filepath.WalkDir("..", func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			return err
		}
		rel := filepath.ToSlash(strings.TrimPrefix(path, ".."+string(filepath.Separator)))
		for _, decl := range file.Decls {
			site := rel + ":const"
			if fn, ok := decl.(*ast.FuncDecl); ok {
				site = rel + ":" + fn.Name.Name
			}
			ast.Inspect(decl, func(node ast.Node) bool {
				switch n := node.(type) {
				case *ast.BasicLit:
					if value, _ := strconv.Unquote(n.Value); n.Kind == token.STRING && value == code {
						found[site]++
					}
				case *ast.Ident:
					// Counted by name: a use from another file of the package has no resolved Obj.
					if n.Name == "runtimeLayoutUnsupported" {
						found[site]++
					}
				case *ast.CallExpr:
					if name, ok := n.Fun.(*ast.Ident); ok && name.Name == "bindingFailure" && strings.HasPrefix(rel, "project/") {
						require.Len(t, n.Args, 2, site)
						lit, ok := n.Args[1].(*ast.BasicLit)
						require.True(t, ok, site)
						value, _ := strconv.Unquote(lit.Value)
						require.Contains(t, reasons, value, site)
					}
				}
				return true
			})
		}
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, allowed, found)
}

func TestBindingCaptureMultipleAndOutsideFiles(t *testing.T) {
	for _, reason := range []string{"multiple-compose-files", "outside-project"} {
		t.Run(reason, func(t *testing.T) {
			f := newLifecycleFixture(t)
			if reason == "multiple-compose-files" {
				require.NoError(t, os.WriteFile(filepath.Join(f.project.Path, "extra.yaml"), []byte("services:\n  app:\n    environment:\n      EXTRA: example\n"), 0600))
				require.NoError(t, os.WriteFile(filepath.Join(f.project.Path, ".env"), []byte("COMPOSE_FILE=compose.yaml:extra.yaml\n"), 0600))
			} else {
				require.NoError(t, os.WriteFile(filepath.Join(f.root, "shared.env"), []byte("EXAMPLE=public\n"), 0600))
				require.NoError(t, os.WriteFile(filepath.Join(f.project.Path, "compose.yaml"), []byte(strings.Replace(lifecycleOriginal, "    image:", "    env_file: ../shared.env\n    image:", 1)), 0600))
			}
			_, err := f.service.lifecycleInputs(t.Context(), f.project)
			var problem *imageDTO.OperationError
			require.ErrorAs(t, err, &problem)
			require.NotNil(t, problem.Problem.Binding)
			require.Equal(t, reason, problem.Problem.Binding.Reason)
		})
	}
}

// A per-service version reference refusal keeps its service (AUTO-2), so the
// refusal text can name it.
func TestVersionReferenceRefusalNamesService(t *testing.T) {
	for source, want := range map[string]imageDTO.Binding{
		"registry.example.com/app:1.0.0@sha256:" + strings.Repeat("a", 64): {Scope: "service", Service: "app", Reason: "digest-pinned"},
		"registry.example.com/app:${APP_TAG}":                              {Scope: "service", Service: "app", Reason: "variable-reference", Variable: "APP_TAG"},
	} {
		_, err := lifecycleVersionReference("app", source, "2.0.0")
		var problem *imageDTO.OperationError
		require.ErrorAs(t, err, &problem)
		require.Equal(t, want, *problem.Problem.Binding)
	}
}
