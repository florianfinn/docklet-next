// Source: docklet-next/docs/design/images-updates.md
// Revision: 7df52f8f3b912be751e2283ec8d990933f5dc4b8
// Retrieved: 2026-10-25
// License: Apache-2.0
// Deviations: Original image/registry UI foundation follow-up for issue #229.

package contract

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	imageDTO "github.com/florianfinn/docklet-next/types/image"
	"github.com/stretchr/testify/require"
)

func TestV7PullFrameFixtureTracksPublicProducerTypes(t *testing.T) {
	body, err := os.ReadFile("testdata/agent-hub-v9.json")
	require.NoError(t, err)
	var fixture struct {
		ImageSafety struct {
			PullStream struct {
				Route, MediaType             string
				RegistryAuthenticationStatus int
				Frame                        struct {
					Ref   string `json:"$ref"`
					OneOf []struct {
						Ref string `json:"$ref"`
					}
				}
				Schemas map[string]struct {
					Properties map[string]any
					OneOf      []struct {
						Ref string `json:"$ref"`
					}
				}
			}
		}
	}
	require.NoError(t, json.Unmarshal(body, &fixture))
	stream := fixture.ImageSafety.PullStream
	require.Equal(t, "POST /environments/{id}/images/pull", stream.Route)
	require.Equal(t, "application/x-json-stream", stream.MediaType)
	require.Equal(t, 502, stream.RegistryAuthenticationStatus)
	variants := stream.Frame.OneOf
	if stream.Frame.Ref != "" {
		variants = stream.Schemas[strings.TrimPrefix(stream.Frame.Ref, "#/components/schemas/")].OneOf
	}
	require.Len(t, variants, 4)
	for _, value := range []any{
		imageDTO.PullStartedFrame{Type: "activity", ActivityID: "activity"},
		imageDTO.PullProgressFrame{Status: "Downloading", ID: "aabbccddeeff", ProgressDetail: &imageDTO.PullProgressCounts{Current: 1, Total: 2}},
		imageDTO.PullErrorFrame{Error: imageDTO.Problem{Code: "image.pull-failed"}},
		imageDTO.PullDoneFrame{Done: true},
	} {
		encoded, err := json.Marshal(value)
		require.NoError(t, err)
		var fields map[string]any
		require.NoError(t, json.Unmarshal(encoded, &fields))
		matched := 0
		for _, variant := range variants {
			properties := stream.Schemas[strings.TrimPrefix(variant.Ref, "#/components/schemas/")].Properties
			if len(properties) != len(fields) {
				continue
			}
			equal := true
			for name := range fields {
				_, present := properties[name]
				equal = equal && present
			}
			if equal {
				matched++
			}
		}
		require.Equal(t, 1, matched, string(encoded))
	}
}
