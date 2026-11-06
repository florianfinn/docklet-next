// Source: docklet-next/issues/348
// Revision: b5068aff3da7eac11f208d1f4d8c9068d4a8c073
// Retrieved: 2026-11-06
// License: Apache-2.0
// Deviations: Original bounded one-time stack import implementation.

package stackimport

import (
	"strings"
	"testing"

	dto "github.com/florianfinn/docklet-next/types/stackimport"
	"github.com/stretchr/testify/require"
)

func TestProposalStructureAndOriginalContent(t *testing.T) {
	data := "# exact proposal\nservices:\n  app:\n    image: alpine:3\n    x-kept: {future: untouched}\n"
	result, err := parseProposal(t.Context(), []byte(data))
	require.NoError(t, err)
	require.Equal(t, data, result.ComposeContent)
	require.Empty(t, result.Missing)
	require.NotNil(t, result.EnvNames)
	for _, invalid := range []string{"", "[]", "services: {}", "services: []", "services:\n  app: null", "services:\n  app:\n    image: [invalid]", data + "---\nservices: {}\n", string([]byte{0xff, 0xfe})} {
		_, err := parseProposal(t.Context(), []byte(invalid))
		requireProblem(t, err, "import.invalid-compose", 422)
	}
}
func TestProposalMalformedYAMLLine(t *testing.T) {
	_, err := parseProposal(t.Context(), []byte("services:\n  app: [\n"))
	requireProblem(t, err, "import.invalid-compose", 422)
	require.Positive(t, err.(interface{ GetStatus() int }).GetStatus())
	require.Contains(t, err.Error(), "import.invalid-compose")
	require.Greater(t, err.(*dto.Problem).Line, 0)
}
func TestProposalAliasBomb(t *testing.T) {
	bomb := "x-a: &a [x,x,x,x,x,x,x,x,x,x]\n"
	previous := "a"
	for _, letter := range []string{"b", "c", "d", "e", "f", "g", "h", "i"} {
		bomb += "x-" + letter + ": &" + letter + " [" + strings.TrimSuffix(strings.Repeat("*"+previous+",", 10), ",") + "]\n"
		previous = letter
	}
	bomb += validCompose
	_, err := parseProposal(t.Context(), []byte(bomb))
	requireProblem(t, err, "import.invalid-compose", 422)
}
func TestProposalByteBound(t *testing.T) {
	_, err := parseProposal(t.Context(), []byte(validCompose+"#"+strings.Repeat("x", fileLimit)))
	requireProblem(t, err, "import.too-large", 413)
}
func TestProposalVariablesAndRequirements(t *testing.T) {
	data := "include: [missing.yaml]\nservices:\n  app:\n    extends: {file: external.yaml, service: base}\n    env_file: [missing.env]\n    label_file: [missing.labels]\n    build: ./app\n    environment:\n      A: ${REQUIRED:?x}\n      B: ${PLAIN}\n      C: ${DEFAULT:-ok}\n      D: ${EMPTY:-}\n      E: $UNBRACED\n      F: $${ESCAPED}\n      G: ${OPTIONAL:-${NESTED}}\n      H: ${REQUIRED:-another-occurrence}\n"
	result, err := parseProposal(t.Context(), []byte(data))
	require.NoError(t, err)
	require.Equal(t, data, result.ComposeContent)
	require.Equal(t, []string{"NESTED", "PLAIN", "REQUIRED", "UNBRACED"}, result.EnvNames)
	require.Equal(t, []string{"import.missing.build-context", "import.missing.env-file", "import.missing.extends", "import.missing.include", "import.missing.label-file", "import.missing.variables"}, result.Missing)
}
func TestProposalExtendsOnly(t *testing.T) {
	data := "services:\n  app:\n    extends: {file: external.yaml, service: base}\n"
	result, err := parseProposal(t.Context(), []byte(data))
	require.NoError(t, err)
	require.Equal(t, data, result.ComposeContent)
	require.Equal(t, []string{"import.missing.extends"}, result.Missing)
}
