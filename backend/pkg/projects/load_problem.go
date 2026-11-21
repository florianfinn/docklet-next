// Source: docklet-next/docs/design/stack-env-and-peers.md
// Revision: 1d8ef389b0a40f3ba27fb09f8f59d8ac993180ff
// Retrieved: 2026-11-21
// License: Apache-2.0
// Deviations: Original typed Compose load problem for issue 547. M34 #548: recorded Compose environment inputs.

package projects

import (
	"errors"
	"io/fs"
	"regexp"

	"github.com/compose-spec/compose-go/v2/template"
)

// LoadProblem is the only printable form of a Compose load failure. Raw
// interpolation and dotenv errors can carry operator text or env values, so
// logs and responses use Reason and Variable instead of err.Error().
type LoadProblem struct {
	Reason   string // Classified variable/file/selection, recorded-label or identity problem.
	Variable string // a safe variable name for compose.variable-missing, else empty
}

var safeVariable = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,127}$`)

// ClassifyLoadError maps a load error to its LoadProblem without reading its text.
func ClassifyLoadError(err error) LoadProblem {
	if err == nil {
		return LoadProblem{}
	}
	var safe *SafeLoadError
	if errors.As(err, &safe) {
		return safe.Problem
	}
	var missing *template.MissingRequiredError
	if errors.As(err, &missing) {
		problem := LoadProblem{Reason: "compose.variable-missing"}
		if safeVariable.MatchString(missing.Variable) {
			problem.Variable = missing.Variable
		}
		return problem
	}
	var env *MissingServiceEnvironmentError
	if errors.As(err, &env) || errors.Is(err, fs.ErrNotExist) {
		return LoadProblem{Reason: "compose.env-file-missing"}
	}
	if errors.Is(err, ErrComposeFileNestedBase) {
		return LoadProblem{Reason: "compose.selection-unsupported"}
	}
	return LoadProblem{Reason: "compose.identity-unreadable"}
}

// String is safe for logs: it never includes the wrapped error text.
func (p LoadProblem) String() string {
	if p.Variable != "" {
		return p.Reason + " (" + p.Variable + ")"
	}
	return p.Reason
}

// SafeLoadError keeps the cause for errors.Is/As while its text is only the
// classified problem.
type SafeLoadError struct {
	Problem LoadProblem
	cause   error
}

// SafeError wraps err so that printing it never reveals raw load text.
func SafeError(err error) error {
	if err == nil {
		return nil
	}
	var safe *SafeLoadError
	if errors.As(err, &safe) {
		return err
	}
	return &SafeLoadError{Problem: ClassifyLoadError(err), cause: err}
}

func (e *SafeLoadError) Error() string { return "load compose project: " + e.Problem.String() }
func (e *SafeLoadError) Unwrap() error { return e.cause }

// errMalformedEnv replaces dotenv parser errors, whose text can quote env lines.
var errMalformedEnv = errors.New("malformed env file")

var lenientVariable = regexp.MustCompile(`^\$\{?([A-Za-z_][A-Za-z0-9_]*)`)

// lenientVariableName keeps only the variable name of a ${VAR:?message} expression.
func lenientVariableName(expression string) string {
	if m := lenientVariable.FindStringSubmatch(expression); m != nil {
		return m[1]
	}
	return ""
}
