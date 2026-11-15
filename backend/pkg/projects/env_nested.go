// Source: docklet-next/issues/406
// Revision: 6e3afa6a78f4f60ffd74858d9a1eaefbb1b73bda
// Retrieved: 2026-11-12
// License: Apache-2.0
// Deviations: Original classification of nested COMPOSE_FILE base files.

package projects

import (
	"errors"
	"fmt"
)

// ErrComposeFileNestedBase marks a COMPOSE_FILE selection whose base file sits in a
// subdirectory; callers that cannot load such layouts report it as unsupported.
var ErrComposeFileNestedBase = errors.New("nested COMPOSE_FILE base file")

type nestedComposeBaseError struct{ entry string }

func (e nestedComposeBaseError) Error() string {
	return fmt.Sprintf("the first COMPOSE_FILE entry %q must be in the project directory", e.entry)
}

func (e nestedComposeBaseError) Is(target error) bool { return target == ErrComposeFileNestedBase }
