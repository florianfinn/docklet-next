// Source: docklet-next/docs/design/images-updates.md
// Revision: 7df52f8f3b912be751e2283ec8d990933f5dc4b8
// Retrieved: 2026-10-25
// License: Apache-2.0
// Deviations: Original image/registry UI foundation follow-up for issue #229.

package image

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/stretchr/testify/require"
)

func TestPullProducerFramesMatchRegisteredSchema(t *testing.T) {
	for _, outcome := range []string{"success", "failure", "busy"} {
		t.Run(outcome, func(t *testing.T) {
			s, api, router, _ := imageWireFixture(t, func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"status":"Downloading","id":"aabbccddeeff","progressDetail":{"current":12,"total":20}}`+"\n")
				if outcome == "failure" {
					_, _ = io.WriteString(w, `{"error":"synthetic-private daemon failure"}`+"\n")
				}
			})
			if outcome == "busy" {
				release, err := s.safety.Acquire(t.Context())
				require.NoError(t, err)
				defer release()
			}
			rec := imageWireRequest(router, "POST", "/pull", `{"imageName":"registry.example.com/app:1"}`)
			require.Equal(t, 200, rec.Code)
			require.Equal(t, "application/x-json-stream", rec.Header().Get("Content-Type"))
			media, present := api.OpenAPI().Paths["/environments/{id}/images/pull"].Post.Responses["200"].Content["application/x-json-stream"]
			require.True(t, present, "pull response must publish its stream media type")
			require.NotNil(t, media)
			schema := media.Schema
			require.NotNil(t, schema)
			lines := strings.Split(strings.TrimSpace(rec.Body.String()), "\n")
			want := 3
			if outcome == "busy" {
				want = 2
			}
			require.Len(t, lines, want, rec.Body.String())
			for i, line := range lines {
				var frame map[string]any
				require.NoError(t, json.Unmarshal([]byte(line), &frame))
				result := &huma.ValidateResult{}
				huma.Validate(api.OpenAPI().Components.Schemas, schema, huma.NewPathBuffer(nil, 0), huma.ModeWriteToServer, frame, result)
				require.Empty(t, result.Errors, "frame %d: %s", i, line)
				if i == 0 {
					require.Equal(t, "activity", frame["type"])
					require.NotEmpty(t, frame["activityId"])
				}
				if i == len(lines)-1 {
					if outcome == "success" {
						require.Equal(t, true, frame["done"])
						require.Nil(t, frame["error"])
					} else {
						code := "image.pull-failed"
						if outcome == "busy" {
							code = "image.busy"
						}
						require.Equal(t, code, frame["error"].(map[string]any)["code"])
						require.Nil(t, frame["done"])
					}
				}
			}
			require.NotContains(t, rec.Body.String(), "synthetic-private")
		})
	}
}
