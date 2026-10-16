// Source: docklet-next/docs/design/hub-backup.md
// Revision: 8f8c8f89bc6ebb7261862d0c35a6ddbc1513e9fa
// Retrieved: 2026-10-15
// License: Apache-2.0
// Deviations: Original HTTP error mapping tests for issue #32.

package backup

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	sharedbackup "github.com/florianfinn/docklet-next/backend/internal/backup"
	backuptypes "github.com/florianfinn/docklet-next/types/backup"
	"github.com/stretchr/testify/require"
)

func TestCreateRejectsUnavailableTargetAsValidation(t *testing.T) {
	handler := &Handler{service: &Service{}}
	_, err := handler.Create(t.Context(), &CreateSystemBackupInput{Body: backuptypes.CreateSystemBackupRequest{TargetID: "target-1"}})
	var statusErr huma.StatusError
	require.ErrorAs(t, err, &statusErr)
	require.Equal(t, http.StatusUnprocessableEntity, statusErr.GetStatus())
}

func TestUpdatePoliciesDistinguishesValidationFromInternalFailures(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want int
	}{
		{name: "target", err: ErrSystemBackupTargetUnavailable, want: http.StatusUnprocessableEntity},
		{name: "policy", err: sharedbackup.ErrPolicyValidation, want: http.StatusBadRequest},
		{name: "storage", err: ErrSystemBackupUnavailable, want: http.StatusServiceUnavailable},
		{name: "tools", err: ErrSystemBackupToolsUnavailable, want: http.StatusServiceUnavailable},
		{name: "database", err: errors.New("database unavailable"), want: http.StatusInternalServerError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			service := &Service{store: Store{ReconcilePolicies: func(context.Context, []backuptypes.UpdateSystemBackupPolicy,
				func(context.Context, *backuptypes.SystemBackupPolicy, backuptypes.UpdateSystemBackupPolicy) error,
				func(context.Context, string), func(context.Context, *backuptypes.SystemBackupPolicy)) error {
				return tc.err
			}}}
			handler := &Handler{service: service}
			_, err := handler.UpdatePolicies(t.Context(), &UpdateSystemBackupPoliciesInput{})
			var statusErr huma.StatusError
			require.ErrorAs(t, err, &statusErr)
			require.Equal(t, tc.want, statusErr.GetStatus())
		})
	}
}
