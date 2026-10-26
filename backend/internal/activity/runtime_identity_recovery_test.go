// Source: docklet-next/docs/design/images-updates-identity.md
// Revision: fcf8c8d3a079154c299912a7e22e0e903e5e4e38
// Retrieved: 2026-10-26
// License: Apache-2.0
// Deviations: Frozen IDs survive reopened receipt recovery without rediscovery.

package activity

import (
	"context"
	"encoding/json"
	dto "github.com/florianfinn/docklet-next/types/updater"
	"github.com/stretchr/testify/require"
	"io"
	"sync/atomic"
	"testing"
)

func TestIdentityFrozenIDRecovery(t *testing.T) {
	for _, corrupt := range []bool{false, true} {
		t.Run(map[bool]string{false: "frozen", true: "corrupt"}[corrupt], func(t *testing.T) {
			f := b1Fixture(t)
			c := imageCommand(f.command(), RuntimeUpdateProject)
			run := b3Admit(t, f, c)
			var stored RuntimeImageIntent
			require.NoError(t, f.store.svc.db.First(&stored, "run_id = ?", run).Error)
			var saved dto.LifecycleIntent
			require.NoError(t, json.Unmarshal([]byte(stored.Payload), &saved))
			require.Equal(t, c.Options.Lifecycle.Selections[0].ImageID, saved.Selections[0].ImageID)
			if corrupt {
				saved.Selections[0].ImageID = ""
				body, e := json.Marshal(saved)
				require.NoError(t, e)
				require.NoError(t, f.store.svc.db.Model(&stored).Update("payload", string(body)).Error)
			}
			b3Phase(t, f, c, run, "executing")
			require.NoError(t, f.store.svc.db.Exec("INSERT INTO image_lifecycle_runs(id,point_id,selected_point_id,had_effect,settled) VALUES(?,?,?,1,0)", run, "example-point", "").Error)
			reopened := f.contender(t, f.resolver)
			var calls atomic.Int32
			require.NoError(t, reopened.ConfigureLocal(RuntimeLocalConfig{ApplicationContext: t.Context(), Store: c.Store, Executor: b2ExecutorFunc(func(context.Context, RuntimeCommand, io.Writer) error { calls.Add(1); return nil })}))
			require.NoError(t, reopened.InitializeLocalStartup(t.Context(), c.Store))
			retained, e := reopened.Admit(t.Context(), c)
			require.NoError(t, e)
			require.Equal(t, run, retained.ActivityID)
			require.Zero(t, calls.Load())
			var after RuntimeImageIntent
			require.NoError(t, f.store.svc.db.First(&after, "run_id = ?", run).Error)
			var frozen dto.LifecycleIntent
			require.NoError(t, json.Unmarshal([]byte(after.Payload), &frozen))
			require.Equal(t, saved.Selections[0].ImageID, frozen.Selections[0].ImageID)
			row := b3Receipt(t, f, run)
			require.NotNil(t, row.SettledAt)
			require.Equal(t, "runtime.interrupted", *row.ExecutorResultCode)
		})
	}
}

func TestIdentityIntentValidationStores(t *testing.T) {
	f := b1Fixture(t)
	for _, field := range []string{"imageId", "digest", "platform", "duplicate", "bounds"} {
		t.Run(field, func(t *testing.T) {
			c := imageCommand(f.command(), RuntimeUpdateProject)
			switch field {
			case "imageId":
				c.Options.Lifecycle.Selections[0].ImageID = ""
			case "digest":
				c.Options.Lifecycle.Selections[0].Digest = "sha256:invalid"
			case "platform":
				c.Options.Lifecycle.Selections[0].Platform = "linux"
			case "duplicate":
				c.Options.Lifecycle.Selections = append(c.Options.Lifecycle.Selections, c.Options.Lifecycle.Selections[0])
			case "bounds":
				for range 256 {
					c.Options.Lifecycle.Selections = append(c.Options.Lifecycle.Selections, c.Options.Lifecycle.Selections[0])
				}
			}
			_, err := runtimeCanonicalizeInternal(c)
			require.Error(t, err)
		})
	}
}
