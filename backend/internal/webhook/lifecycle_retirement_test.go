// Source: docklet-next/docs/design/images-updates.md
// Revision: 5a7a7ad39c1817a3d222bc84c0cf91d433d48f0d
// Retrieved: 2026-10-25
// License: Apache-2.0
// Deviations: Legacy webhook image mutations refuse before local or remote dispatch.

package webhook

import (
	"github.com/florianfinn/docklet-next/types/image"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
)

func TestRetiredWebhookActionsCannotDispatch(t *testing.T) {
	for _, env := range []string{"0", "example-remote"} {
		for _, kind := range []string{WebhookTargetTypeProject, WebhookTargetTypeContainer, WebhookTargetTypeUpdater} {
			t.Run(env+"/"+kind, func(t *testing.T) {
				action := WebhookActionTypeUpdate
				if kind == WebhookTargetTypeUpdater {
					action = WebhookActionTypeRun
				}
				svc := &WebhookService{}
				out, err := svc.executeWebhookActionInternal(t.Context(), &Webhook{EnvironmentID: env, TargetType: kind, TargetID: "example-target"}, action)
				require.Nil(t, out)
				require.Equal(t, "update.path-retired", image.Neutral(err, "unexpected", 500).Problem.Code)
			})
		}
	}
}

func TestRetiredWebhookTriggerHasNoAcceptanceEffects(t *testing.T) {
	for _, env := range []string{"0", "example-remote"} {
		for _, kind := range []string{WebhookTargetTypeContainer, WebhookTargetTypeProject, WebhookTargetTypeUpdater} {
			t.Run(env+"/"+kind, func(t *testing.T) {
				db := setupWebhookServiceTestDB(t)
				svc := newTestWebhookService(db)
				raw := "arc_wh_" + strings.ReplaceAll(uuid.NewString(), "-", "")
				action := WebhookActionTypeUpdate
				if kind == WebhookTargetTypeUpdater {
					action = WebhookActionTypeRun
				}
				wh := insertWebhookDirect(t, t.Context(), db, raw, kind, action, "example-target", env)
				err := svc.TriggerByToken(t.Context(), raw)
				saved := fetchWebhook(t, db, wh.ID)
				require.Nil(t, saved.LastTriggeredAt)
				require.Equal(t, "update.path-retired", image.Neutral(err, "unexpected", 500).Problem.Code)
				require.NoError(t, svc.DrainActions(t.Context()))
			})
		}
	}
}
