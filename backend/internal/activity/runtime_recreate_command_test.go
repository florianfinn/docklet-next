// Source: docklet-next/issues/315
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-30
// License: Apache-2.0
// Deviations: Original recreate digest ownership, bounds and semantic distinctions.

package activity

import (
	"encoding/json"
	"strings"
	"testing"

	activityDTO "github.com/florianfinn/docklet-next/types/activity"
	dto "github.com/florianfinn/docklet-next/types/container"
	"github.com/stretchr/testify/require"
)

func TestRecreateCanonicalEditOwnershipAndDigest(t *testing.T) {
	f := b1Fixture(t)
	command := f.command()
	command.Operation = RuntimeRecreateContainer
	command.Target = RuntimeTarget{Kind: "container", ID: strings.Repeat("a", 64)}
	labels := map[string]string{"b": "2", "a": "1"}
	environment := []string{"EXAMPLE=first"}
	command.Options = RuntimeOptions{Recreate: &dto.Edit{Labels: &labels, Environment: &environment}}
	canonical, err := runtimeCanonicalizeInternal(command)
	require.NoError(t, err)
	labels["a"] = "changed"
	environment[0] = "EXAMPLE=changed"
	require.Equal(t, "1", (*canonical.command.Options.Recreate.Labels)["a"])
	require.Equal(t, "EXAMPLE=first", (*canonical.command.Options.Recreate.Environment)[0])
	equivalent := command
	equivalent.Options = RuntimeOptions{Recreate: &dto.Edit{Labels: new(map[string]string{"a": "1", "b": "2"}), Environment: new([]string{"EXAMPLE=first"})}}
	same, err := runtimeCanonicalizeInternal(equivalent)
	require.NoError(t, err)
	require.Equal(t, canonical.digest, same.digest)
	changed, err := runtimeCanonicalizeInternal(command)
	require.NoError(t, err)
	require.NotEqual(t, canonical.digest, changed.digest)
	command.Options = RuntimeOptions{Recreate: &dto.Edit{}}
	absent, err := runtimeCanonicalizeInternal(command)
	require.NoError(t, err)
	command.Options.Recreate.Command = new([]string{})
	empty, err := runtimeCanonicalizeInternal(command)
	require.NoError(t, err)
	require.NotEqual(t, absent.digest, empty.digest)
	tree := runtimeRecreateSemantic(dto.Edit{}).(map[string]any)
	require.Contains(t, tree, "command")
	require.Nil(t, tree["command"])
	command.Options = RuntimeOptions{Recreate: &dto.Edit{Environment: new([]string{strings.Repeat("x", 64<<10)})}}
	_, err = runtimeCanonicalizeInternal(command)
	require.Error(t, err)
}
func TestRecreateStrictNestedEditWire(t *testing.T) {
	for _, body := range []string{`{"edit":{"unknown":true}}`, `{"edit":{"hostConfig":{"unknown":true}}}`, `{"edit":{"command":[1]}}`, `{"edit":{},"unknown":true}`, `{"edit":{},"edit":{}}`, `{"edit":null}`} {
		var request dto.RecreateRequest
		require.Error(t, activityDTO.DecodeRuntimeJSON([]byte(body), &request), body)
	}
	var request dto.RecreateRequest
	require.NoError(t, activityDTO.DecodeRuntimeJSON([]byte(`{"edit":{"labels":{},"command":[]}}`), &request))
	require.NotNil(t, request.Edit.Labels)
	require.NotNil(t, request.Edit.Command)
	body, err := json.Marshal(request)
	require.NoError(t, err)
	require.Contains(t, string(body), `"command":[]`)
}
