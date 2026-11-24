// Source: docklet-next/issues/594
// Revision: 48d8f45d867a7a41e571b59382b291c350d3ce42
// Retrieved: 2026-11-24
// License: Apache-2.0
// Deviations: Original read-only image binding predicates and eligibility projection.

package project

import (
	"context"
	"errors"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	life "github.com/florianfinn/docklet-next/backend/internal/project/children/lifecycle"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	dto "github.com/florianfinn/docklet-next/types/updater"
	"go.yaml.in/yaml/v4"
)

func bindingFailure(service, reason string) *imageDTO.OperationError {
	problem := imageDTO.Failure("update.image-binding-unsupported", 409)
	scope := "project"
	if service != "" {
		scope = "service"
	}
	problem.Problem.Binding = &imageDTO.Binding{Scope: scope, Service: service, Reason: reason}
	return problem
}

var bindingVariable = regexp.MustCompile(`\$(?:\{)?([A-Za-z_][A-Za-z0-9_]{0,127})`)

func referenceBindingFailure(service, source string) *imageDTO.OperationError {
	if strings.Contains(source, "$") {
		problem := bindingFailure(service, "variable-reference")
		if match := bindingVariable.FindStringSubmatch(source); len(match) == 2 {
			problem.Problem.Binding.Variable = match[1]
		}
		return problem
	}
	if strings.Contains(source, "@") {
		return bindingFailure(service, "digest-pinned")
	}
	return nil
}

// modelBindingFailure shares the same source predicates with the version writer;
// resolved Compose values alone cannot prove that a field is literal.
func modelBindingFailure(snapshot *life.Snapshot, service string) error {
	document, err := lifecyclePrimaryDocument(snapshot)
	if err != nil {
		return err
	}
	return modelBindingFailureIn(snapshot, document, service)
}

// lifecyclePrimaryDocument parses the single authoritative Compose file once.
func lifecyclePrimaryDocument(snapshot *life.Snapshot) (*yaml.Node, error) {
	if snapshot == nil || snapshot.Model == nil {
		return nil, bindingFailure("", "unparsable")
	}
	if len(snapshot.Model.ComposeFiles) == 0 {
		return nil, bindingFailure("", "file-missing")
	}
	if len(snapshot.Model.ComposeFiles) != 1 {
		return nil, bindingFailure("", "multiple-compose-files")
	}
	primary := filepath.Base(snapshot.Model.ComposeFiles[0])
	for _, file := range snapshot.Files {
		if file.Path != primary {
			continue
		}
		var document yaml.Node
		if yaml.Unmarshal(file.Content, &document) != nil || len(document.Content) != 1 {
			return nil, bindingFailure("", "unparsable")
		}
		return &document, nil
	}
	return nil, bindingFailure("", "file-missing")
}

// modelBindingFailureIn applies the per-service source predicates to an already parsed document.
func modelBindingFailureIn(snapshot *life.Snapshot, document *yaml.Node, service string) error {
	svc, ok := snapshot.Model.Services[service]
	if !ok {
		return bindingFailure(service, "unparsable")
	}
	if svc.Build != nil {
		return bindingFailure(service, "build-service")
	}
	if failure := referenceBindingFailure(service, svc.Image); failure != nil {
		return failure
	}
	services := lifecycleYAMLField(document.Content[0], "services", 0)
	node := lifecycleYAMLField(services, service, 0)
	image := lifecycleYAMLField(node, "image", 0)
	if image == nil {
		return bindingFailure(service, "unparsable")
	}
	if image.Kind == yaml.AliasNode {
		image = image.Alias
	}
	if failure := referenceBindingFailure(service, image.Value); failure != nil {
		return failure
	}
	return lifecycleSharedImageSourceIn(document, []string{service})
}

// ImageUpdateEligibility reads only saved model/filesystem inputs. It neither
// observes Docker nor contacts a registry, records a baseline, or writes files.
func (s *ProjectService) ImageUpdateEligibility(ctx context.Context, id string) (dto.UpdateEligibility, error) {
	result := dto.UpdateEligibility{Services: []dto.ServiceEligibility{}}
	p, err := s.GetProjectFromDatabaseByID(ctx, id)
	if err != nil || p == nil {
		return result, lifecycleFailure("update.target-not-found")
	}
	snapshot, err := s.lifecycleInputs(ctx, p)
	if err != nil {
		var problem *imageDTO.OperationError
		if errors.As(err, &problem) && problem.Problem.Binding != nil {
			result.Project = problem.Problem.Binding
			return result, nil
		}
		return result, err
	}
	if err := s.preflightLifecycleWrites(ctx, p, snapshot); err != nil {
		var problem *imageDTO.OperationError
		if errors.As(err, &problem) && problem.Problem.Binding != nil {
			result.Project = problem.Problem.Binding
			return result, nil
		}
		return result, err
	}
	names := make([]string, 0, len(snapshot.Model.Services))
	for name := range snapshot.Model.Services {
		names = append(names, name)
	}
	slices.Sort(names)
	document, err := lifecyclePrimaryDocument(snapshot)
	if err != nil {
		var problem *imageDTO.OperationError
		if !errors.As(err, &problem) {
			return result, err
		}
		result.Project = problem.Problem.Binding
		return result, nil
	}
	entries := make([]dto.ServiceEligibility, 0, len(names))
	candidates := make([]string, 0, len(names))
	for _, name := range names {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		entry := dto.ServiceEligibility{Service: name, Eligible: true}
		if err := modelBindingFailureIn(snapshot, document, name); err != nil {
			var problem *imageDTO.OperationError
			if !errors.As(err, &problem) {
				return result, err
			}
			entry.Eligible, entry.Binding = false, problem.Problem.Binding
		} else {
			candidates = append(candidates, name)
		}
		entries = append(entries, entry)
	}
	// The admission writer is the proof for the rest: one read-only check of the
	// document, then of each service alone (closing review M38-CL-R2-01, R3-01).
	if len(candidates) > 0 {
		refusals, document := lifecycleWriterRefusals(snapshot, candidates)
		if document != nil {
			result.Project = document.Problem.Binding
			return result, nil
		}
		for i := range entries {
			if refusal := refusals[entries[i].Service]; refusal != nil {
				entries[i].Eligible, entries[i].Binding = false, refusal.Problem.Binding
			}
		}
	}
	result.Services = entries
	return result, nil
}

// Compose rejects some external inputs before returning dependency metadata.
// Classify those literal paths before loading, without reading their contents.
func outsideBindingInput(document *yaml.Node, projectPath string) bool {
	outside := func(value string) bool {
		if value == "" || strings.Contains(value, "$") {
			return false
		}
		path := value
		if !filepath.IsAbs(path) {
			path = filepath.Join(projectPath, path)
		}
		rel, err := filepath.Rel(projectPath, path)
		return err != nil || rel == ".." || strings.HasPrefix(rel, "../")
	}
	var values func(*yaml.Node) bool
	values = func(node *yaml.Node) bool {
		if node == nil {
			return false
		}
		if node.Kind == yaml.ScalarNode {
			return outside(node.Value)
		}
		if node.Kind == yaml.MappingNode {
			return values(lifecycleYAMLField(node, "path", 0))
		}
		for _, child := range node.Content {
			if values(child) {
				return true
			}
		}
		return false
	}
	var scan func(*yaml.Node) bool
	scan = func(node *yaml.Node) bool {
		if node.Kind == yaml.MappingNode {
			for i := 0; i+1 < len(node.Content); i += 2 {
				if node.Content[i].Value == "env_file" && values(node.Content[i+1]) {
					return true
				}
			}
		}
		for _, child := range node.Content {
			if scan(child) {
				return true
			}
		}
		return false
	}
	if scan(document) {
		return true
	}
	if len(document.Content) == 1 {
		for _, kind := range []string{"configs", "secrets"} {
			definitions := lifecycleYAMLField(document.Content[0], kind, 0)
			if definitions != nil && definitions.Kind == yaml.MappingNode {
				for i := 1; i < len(definitions.Content); i += 2 {
					file := lifecycleYAMLField(definitions.Content[i], "file", 0)
					if file != nil && outside(file.Value) {
						return true
					}
				}
			}
		}
	}
	return false
}
