// Source: getarcaneapp/arcane/backend/pkg/scheduler/image_update_watcher_test.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; replace retired-trigger tests with zero-effect ownership guards

package scheduler

import (
	"context"
	"github.com/florianfinn/docklet-next/types/containerregistry"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	checkDTO "github.com/florianfinn/docklet-next/types/imageupdate"
	"github.com/stretchr/testify/require"
	"testing"
)

type retiredScanner struct{ calls int }

func (f *retiredScanner) CheckAllImages(context.Context, int, []containerregistry.Credential) (map[string]*checkDTO.Response, error) {
	f.calls++
	return nil, nil
}
func TestLegacyImageWatcherCannotCheckIndependently(t *testing.T) {
	fake := &retiredScanner{}
	watcher := &ImageUpdateWatcher{imageUpdateService: fake}
	for _, invoke := range []func(context.Context) error{watcher.Start, watcher.RunNow} {
		err := invoke(t.Context())
		require.Error(t, err)
		require.Equal(t, "update.agent-local-job-disabled", imageDTO.Neutral(err, "unexpected", 500).Problem.Code)
	}
	watcher.Trigger()
	watcher.RefreshSchedule()
	require.NoError(t, watcher.Stop(t.Context()))
	require.Zero(t, fake.calls, "hub and agents must not retain continuous/event trigger sources")
}
