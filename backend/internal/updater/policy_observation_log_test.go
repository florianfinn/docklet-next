// Source: docklet-next/issues/341
// Revision: b476f0aec8d6f94a0bdd3087834cdbea23de1f3f
// Retrieved: 2026-11-05
// License: Apache-2.0
// Deviations: A failed native observation keeps a neutral record and logs its cause.

package updater

import (
	"bytes"
	"log/slog"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/imageupdate"
	"github.com/stretchr/testify/require"
)

func TestFailedNativeObservationLogsCause(t *testing.T) {
	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	s := &UpdaterService{deps: updaterDependenciesInternal{ImageUpdates: imageupdate.NewImageUpdateService(nil, nil, nil, nil, nil, nil, nil)}}
	record := imageupdate.ImageUpdateRecord{ServiceName: "web", CheckTime: time.Now().UTC(), HasUpdate: true}
	s.enrichPolicyObservation(t.Context(), "registry.example.com/app:1.0.0", "", "", "stack", &record)
	require.False(t, record.HasUpdate)
	require.NotNil(t, record.LastError)
	require.Contains(t, logs.String(), "Native update observation failed")
	require.Contains(t, logs.String(), "image=registry.example.com/app:1.0.0")
	require.Contains(t, logs.String(), "service=web")
	require.Contains(t, logs.String(), "error=")
}
