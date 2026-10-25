// Source: docklet-next/docs/design/images-updates.md
// Revision: f6033a46c9ef4ad490661addd7dac41ab0ed5a45
// Retrieved: 2026-10-25
// License: Apache-2.0
// Deviations: Retained image-read status and diagnostic privacy regressions.

package image

import (
	"fmt"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	"github.com/stretchr/testify/require"
	"net/http"
	"testing"
)

func TestImageReadsKeepStatusAndNeutralDiagnostics(t *testing.T) {
	s, _, _ := safetyFixture(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		p := imageDockerTestPathInternal(r.URL.Path)
		switch p {
		case "/images/search":
			if r.URL.Query().Get("term") == "Broken Search" {
				w.WriteHeader(500)
				_, _ = fmt.Fprint(w, `{"message":"synthetic private search diagnostic"}`)
			} else {
				_, _ = fmt.Fprint(w, `[{"name":"example"}]`)
			}
		case "/images/json", "/containers/json":
			_, _ = fmt.Fprint(w, `[]`)
		default:
			w.WriteHeader(404)
			_, _ = fmt.Fprint(w, `{"message":"synthetic missing image"}`)
		}
	})
	h := &ImageHandler{imageService: s}
	_, err := h.GetImage(t.Context(), &GetImageInput{ImageID: "missing"})
	require.Error(t, err)
	problem := imageDTO.Neutral(err, "unexpected", 500)
	require.Equal(t, 404, problem.Status)
	require.Equal(t, "image.not-found", problem.Problem.Code)
	out, err := h.SearchImages(t.Context(), &SearchImagesInput{Term: "Example Search"})
	require.NoError(t, err)
	require.Len(t, out.Body.Data, 1)
	_, err = h.SearchImages(t.Context(), &SearchImagesInput{Term: " "})
	require.Error(t, err)
	require.Equal(t, 400, imageDTO.Neutral(err, "unexpected", 500).Status)
	_, err = h.SearchImages(t.Context(), &SearchImagesInput{Term: "Broken Search"})
	require.Error(t, err)
	require.Equal(t, 503, imageDTO.Neutral(err, "unexpected", 500).Status)
	require.NotContains(t, err.Error(), "synthetic private search diagnostic")
	_, err = h.SearchImages(t.Context(), &SearchImagesInput{Term: "DOCKLET-RETENTION.INVALID/points/p:saved"})
	require.Error(t, err)
	require.Equal(t, "image.reserved-reference", imageDTO.Neutral(err, "unexpected", 500).Problem.Code)
}
