// Source: docklet-next/docs/design/images-updates.md
// Revision: 7df52f8f3b912be751e2283ec8d990933f5dc4b8
// Retrieved: 2026-10-25
// License: Apache-2.0
// Deviations: Original image/registry UI foundation follow-up for issue #229.

package image

import (
	"errors"
	"fmt"
	"net"
	"testing"

	"github.com/containerd/errdefs"
	"github.com/florianfinn/docklet-next/backend/internal/image/guard"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	"github.com/stretchr/testify/require"
)

func TestImageWriteErrorClassificationDropsDiagnostics(t *testing.T) {
	for _, test := range []struct {
		name   string
		err    error
		code   string
		status int
	}{
		{"typed", fmt.Errorf("synthetic-private wrapper: %w", imageDTO.Failure("image.busy", 409)), "image.busy", 409},
		{"missing", fmt.Errorf("synthetic-private: %w", errdefs.ErrNotFound), "image.not-found", 404},
		{"unavailable", fmt.Errorf("synthetic-private: %w", errdefs.ErrUnavailable), "image.usage-unavailable", 503},
		{"transport", &net.DNSError{Err: "synthetic-private lookup failure"}, "image.usage-unavailable", 503},
		{"upstream", errors.New("synthetic-private daemon failure"), "image.tag-failed", 502},
	} {
		t.Run(test.name, func(t *testing.T) {
			problem := guard.DaemonError(test.err, "image.tag-failed")
			require.Equal(t, test.code, problem.Problem.Code)
			require.Equal(t, test.status, problem.Status)
			require.NotContains(t, problem.Error(), "synthetic-private")
		})
	}
	problem := imageDTO.Neutral(errors.New("synthetic-private internal failure"), "image.remove-failed", 500)
	require.Equal(t, 500, problem.Status)
	require.Equal(t, "The operation could not be completed.", problem.Error())
}
