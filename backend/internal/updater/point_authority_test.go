// Source: docklet-next/docs/design/images-updates.md
// Revision: beec37b5b4339a07a946bb56d19fb8062ba8bc66
// Retrieved: 2026-10-27
// License: Apache-2.0
// Deviations: Current executor point evidence, reused identities and malformed envelopes for #229.

package updater

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	a "github.com/florianfinn/docklet-next/types/activity"
	"github.com/florianfinn/docklet-next/types/base"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	dto "github.com/florianfinn/docklet-next/types/updater"
	"github.com/stretchr/testify/require"
)

type pointAuthorityProxy func(context.Context, string, string, string, []byte, any) error

func (p pointAuthorityProxy) ProxyJSONRequest(ctx context.Context, env, method, path string, body []byte, out any) error {
	return p(ctx, env, method, path, body, out)
}
func TestClosingPointAuthorityEvidence(t *testing.T) {
	for _, tc := range []string{"current", "orphan", "reused-id", "wrong-target", "duplicate", "null", "failed", "malformed", "transport", "empty"} {
		t.Run(tc, func(t *testing.T) {
			point := dto.RestorePoint{ID: "point", RegistrationID: "example", TargetID: "stack", TargetIdentity: strings.Repeat("a", 64), SourceRunID: "run", CreatedAt: time.Now().UTC(), SourceState: "settled", HoldState: dto.HoldNone, Images: []dto.PointImage{}}
			points := []dto.RestorePoint{point}
			success := true
			switch tc {
			case "orphan", "reused-id":
				points[0].Orphaned = true
			case "wrong-target":
				points[0].TargetID = "foreign"
			case "duplicate":
				points = append(points, point)
			case "null":
				points = nil
			case "failed":
				success = false
			case "empty":
				points = []dto.RestorePoint{}
			}
			calls := 0
			proxy := pointAuthorityProxy(func(_ context.Context, env, method, path string, body []byte, out any) error {
				calls++
				require.Equal(t, "example", env)
				require.Equal(t, "GET", method)
				require.Equal(t, "/api/environments/0/updater/project/stack/restore-points", path)
				require.Nil(t, body)
				if tc == "transport" {
					return errors.New("synthetic transport failure")
				}
				raw, err := json.Marshal(map[string]any{"success": success, "data": points})
				require.NoError(t, err)
				if tc == "malformed" {
					raw = []byte(`{"success":true,"data":[{"orphaned":false}]}`)
				}
				return json.Unmarshal(raw, out)
			})
			err := (*UpdaterService)(nil).CheckRestorePointAccess(t.Context(), "example", "stack", "", proxy)
			if tc == "current" || tc == "empty" {
				require.NoError(t, err)
			} else {
				var p *imageDTO.OperationError
				require.ErrorAs(t, err, &p)
				if tc == "orphan" || tc == "reused-id" {
					require.Equal(t, "update.invalid-target", p.Problem.Code)
				} else {
					require.Equal(t, "update.check-unavailable", p.Problem.Code)
				}
			}
			require.Equal(t, 1, calls)
		})
	}
}

func TestClosingSelectedPointAuthority(t *testing.T) {
	point := dto.RestorePoint{ID: "old", RegistrationID: "example", TargetID: "stack", TargetIdentity: strings.Repeat("a", 64), SourceRunID: "run", CreatedAt: time.Now().UTC(), SourceState: "settled", HoldState: dto.HoldNone, Images: []dto.PointImage{}, Orphaned: true}
	current := point
	current.ID = "current"
	current.Orphaned = false
	current.TargetIdentity = strings.Repeat("b", 64)
	proxy := pointAuthorityProxy(func(_ context.Context, _, _, _ string, _ []byte, out any) error {
		raw, err := json.Marshal(map[string]any{"success": true, "data": []dto.RestorePoint{point, current}})
		require.NoError(t, err)
		return json.Unmarshal(raw, out)
	})
	for _, selected := range []string{"", "old", "current", "missing"} {
		err := (*UpdaterService)(nil).CheckRestorePointAccess(t.Context(), "example", "stack", selected, proxy)
		if selected == "current" {
			require.NoError(t, err)
		} else {
			require.Error(t, err)
		}
	}
}

func TestClosingPolicyUnknownEnvelope(t *testing.T) {
	snapshot := a.RuntimeSnapshot{Activity: a.Activity{Status: a.StatusFailed, ImageLifecycle: &dto.LifecycleResult{Code: "runtime.outcome-unknown", HTTPStatus: 503}}, RuntimeKnowledge: a.RuntimeKnowledge{OutcomeState: a.RuntimeOutcomeKnown}}
	target, uncertain := imagePolicySnapshotTarget("stack", "run", snapshot)
	require.True(t, uncertain)
	require.Equal(t, "runtime.outcome-unknown", target.Code)
}

func TestClosingPolicyBaselineReason(t *testing.T) {
	for _, code := range []string{"", "update.baseline-unavailable", "undeclared"} {
		baseline := dto.BaselineStatus{TargetIdentity: strings.Repeat("a", 64), SavedRevision: strings.Repeat("b", 64), ObservedRevision: strings.Repeat("c", 64), Available: true, Code: code}
		out, err := checkedBaselineResponse(base.ApiResponse[dto.BaselineStatus]{Success: true, Data: baseline})
		require.NoError(t, err)
		if code == "undeclared" {
			require.False(t, out.Available)
			require.Equal(t, "runtime.outcome-unknown", out.Code)
		} else {
			require.Equal(t, baseline, out)
		}
	}
}
