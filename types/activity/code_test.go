// Source: florianfinn/docklet-next/types/activity/code.go
// Revision: d4e7845e461f51d56a63d3a43937679d6449cf3c
// Retrieved: 2026-10-14
// License: Apache-2.0
// Deviations: Keep every published code enum and the design catalogue aligned.

package activity_test

import (
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/florianfinn/docklet-next/types/activity"
	"github.com/florianfinn/docklet-next/types/event"
	"github.com/stretchr/testify/require"
)

func TestMessageCodeEnumsAndDocumentationMatchCatalogue(t *testing.T) {
	values := activity.Values()
	want := make([]string, len(values))
	for i, code := range values {
		want[i] = string(code)
	}
	for _, field := range []struct {
		typeOf reflect.Type
		name   string
	}{
		{reflect.TypeFor[activity.Activity](), "MessageCode"},
		{reflect.TypeFor[activity.Message](), "Code"},
		{reflect.TypeFor[activity.StepTraceEntry](), "Code"},
		{reflect.TypeFor[event.Event](), "MessageCode"},
	} {
		actual, ok := field.typeOf.FieldByName(field.name)
		require.True(t, ok)
		require.Equal(t, want, strings.Split(actual.Tag.Get("enum"), ","), field.typeOf.Name()+"."+field.name)
	}
	status, ok := reflect.TypeFor[activity.StepTraceEntry]().FieldByName("Status")
	require.True(t, ok)
	require.Equal(t, []string{"queued", "running", "success", "failed", "cancelled"}, strings.Split(status.Tag.Get("enum"), ","))
	doc, err := os.ReadFile("../../docs/design/api-contract.md")
	require.NoError(t, err)
	for _, code := range want {
		require.Contains(t, string(doc), "`"+code+"`")
	}
}
