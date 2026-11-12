// Source: docklet-next/issues/406
// Revision: 6e3afa6a78f4f60ffd74858d9a1eaefbb1b73bda
// Retrieved: 2026-11-12
// License: Apache-2.0
// Deviations: Original typed target refusal for stack layouts updates cannot load.

package activity

import (
	"errors"

	"github.com/florianfinn/docklet-next/backend/pkg/projects"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
)

const runtimeLayoutUnsupported = "update.image-binding-unsupported"

// runtimeLayoutProblem is the executor's typed answer when a stack target has a
// nested COMPOSE_FILE base; the hub decides per operation whether it applies.
func runtimeLayoutProblem(kind string, err error) error {
	if kind == "stack" && errors.Is(err, projects.ErrComposeFileNestedBase) {
		return imageDTO.Failure(runtimeLayoutUnsupported, 409)
	}
	return nil
}

// runtimeTargetFailure keeps the layout refusal for image updates and restores;
// other operations keep runtime.target-unavailable without the relayed problem.
func runtimeTargetFailure(op RuntimeOperation, err error) error {
	var problem *imageDTO.OperationError
	relayed := errors.As(err, &problem) && problem.Problem.Code == runtimeLayoutUnsupported
	if runtimeImageOperation(op) && (relayed || errors.Is(err, projects.ErrComposeFileNestedBase)) {
		return imageDTO.Failure(runtimeLayoutUnsupported, 409)
	}
	if relayed {
		err = errors.New(err.Error())
	}
	return &RuntimeTargetError{Code: "runtime.target-unavailable", Cause: err}
}
