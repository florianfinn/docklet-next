// Source: docklet-next/issues/440
// Revision: 6e3afa6a78f4f60ffd74858d9a1eaefbb1b73bda
// Retrieved: 2026-11-12
// License: Apache-2.0
// Deviations: Original frozen stability requirement construction.

package stability

import (
	"fmt"
	"slices"

	compose "github.com/compose-spec/compose-go/v2/types"
)

// Kind distinguishes continuous services from declared completion dependencies.
type Kind uint8

const (
	Continuous Kind = iota
	CompletionJob
)

type requirement struct {
	project string
	service string
	id      string
	want    int
	kind    Kind
}

// RequiredSet owns a frozen copy of the intended membership, never engine counts.
// Its zero value is empty and passes immediately, as for a stopped container.
type RequiredSet struct{ requirements []requirement }

// NewContainerSet requires one exact replacement ID, or none for a stopped target.
// Capture the ID from the completed create; do not resolve a mutable name here.
func NewContainerSet(id string) RequiredSet {
	if id == "" {
		return RequiredSet{}
	}
	return RequiredSet{requirements: []requirement{{id: id, want: 1}}}
}

// NewStackSet freezes the selected/present services in the effective deploy model.
// The caller must prune selection, presence and dependencies before calling it;
// recovery passes the point's running services. Empty selection requires nothing.
func NewStackSet(model *compose.Project, services []string) (RequiredSet, error) {
	if model == nil {
		return RequiredSet{}, fmt.Errorf("stability: missing compose model")
	}
	if model.Name == "" {
		return RequiredSet{}, fmt.Errorf("stability: missing compose project name")
	}
	jobs := map[string]bool{}
	for _, service := range model.Services {
		for name, dependency := range service.DependsOn {
			if dependency.Condition == compose.ServiceConditionCompletedSuccessfully {
				jobs[name] = true
			}
		}
	}
	names := slices.Clone(services)
	slices.Sort(names)
	names = slices.Compact(names)
	set := RequiredSet{}
	for _, name := range names {
		service, ok := model.Services[name]
		if !ok {
			return RequiredSet{}, fmt.Errorf("stability: selected service is absent from model")
		}
		want := service.GetScale()
		if want < 0 {
			return RequiredSet{}, fmt.Errorf("stability: negative service scale")
		}
		if want == 0 {
			continue
		}
		kind := Continuous
		if jobs[name] {
			kind = CompletionJob
		}
		set.requirements = append(set.requirements, requirement{project: model.Name, service: name, want: want, kind: kind})
	}
	return set, nil
}
