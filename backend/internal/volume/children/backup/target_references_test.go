// Source: docklet-next/issues/283
// Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original actual authenticated all-host references and nested admission fence proofs.

package backup

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/backuptarget"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/environment"
	"github.com/florianfinn/docklet-next/backend/internal/volume/children/backup/children/policies"
	dto "github.com/florianfinn/docklet-next/types/backup"
	"github.com/florianfinn/docklet-next/types/base"
	"github.com/florianfinn/docklet-next/types/contract"
	"github.com/florianfinn/docklet-next/types/version"
	"github.com/stretchr/testify/require"
	"go.getarcane.app/sys/crypto"
)

type targetRoundTrip func(*http.Request) (*http.Response, error)

func (f targetRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestTargetRetirementCollectsEveryHostAndRefusesUnknownOrReferences(t *testing.T) {
	f := newRuntimePlan(t)
	require.NoError(t, f.service.deps.DB.AutoMigrate(&environment.Environment{}, &backuptarget.Target{}, &backuptarget.Check{}, &policies.VolumeBackupPolicy{}))
	crypto.InitEncryption(&crypto.Config{Environment: "test", EncryptionKey: "synthetic-encryption-key-at-least-32-bytes"})
	for _, id := range []string{"a", "b"} {
		token := strings.Repeat(id, 32)
		require.NoError(t, f.service.deps.DB.Create(&environment.Environment{BaseModel: database.BaseModel{ID: id}, Name: id, ApiUrl: "http://" + id + ".example.com", Enabled: true, AccessToken: &token}).Error)
	}
	state := "policy"
	observed := map[string]int{}
	transport := targetRoundTrip(func(request *http.Request) (*http.Response, error) {
		status := 200
		var payload any
		switch {
		case request.URL.Path == "/api/app-version":
			payload = version.Info{AgentContractVersion: contract.AgentHubVersion, AgentContractFingerprint: contract.AgentHubFingerprint}
		case strings.HasSuffix(request.URL.Path, "/system/health"):
			payload = map[string]bool{"ok": true}
		case strings.HasSuffix(request.URL.Path, "/references"):
			observed[request.URL.Host]++
			refs := dto.TargetReferences{Policies: []string{}, Backups: []string{}, Running: []string{}}
			if request.URL.Host == "b.example.com" {
				switch state {
				case "policy":
					refs.Policies = []string{"b-data"}
				case "backup":
					refs.Backups = []string{"b-data"}
				case "running":
					refs.Running = []string{"b-data"}
				case "offline":
					status = 503
				}
			}
			payload = base.ApiResponse[dto.TargetReferences]{Success: true, Data: refs}
		case request.Method == http.MethodPut && strings.HasSuffix(request.URL.Path, "/volumes/backups/targets"):
			var set dto.TargetSet
			require.NoError(t, json.NewDecoder(request.Body).Decode(&set))
			payload = base.ApiResponse[dto.TargetSetReceipt]{Success: true, Data: dto.TargetSetReceipt{Authority: set.Authority, SetVersion: set.SetVersion, ContentHash: set.ContentHash, Conflicts: []string{}}}
		default:
			t.Errorf("unexpected remote request %s %s", request.Method, request.URL.Path)
			status = 500
		}
		encoded, err := json.Marshal(payload)
		require.NoError(t, err)
		return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(string(encoded))), Request: request}, nil
	})
	environments := environment.NewEnvironmentService(f.service.deps.DB, &http.Client{Transport: transport}, nil, nil, nil, nil)
	target, err := f.service.targets.Create(t.Context(), dto.TargetInput{TargetLocation: dto.TargetLocation{Kind: "smb", Server: "nas.example.com", Share: "archive"}, Name: "Archive"})
	require.NoError(t, err)
	f.service.configureTargetDelivery(environments)
	for _, condition := range []string{"policy", "backup", "running", "offline"} {
		state = condition
		beforeA, beforeB := observed["a.example.com"], observed["b.example.com"]
		err = f.service.targets.Retire(t.Context(), target.ID)
		if condition == "offline" {
			require.ErrorContains(t, err, "backup.target.hosts-unknown")
		} else {
			require.ErrorContains(t, err, "backup.target.in-use")
		}
		require.Greater(t, observed["a.example.com"], beforeA)
		require.Greater(t, observed["b.example.com"], beforeB, "the last host cannot be skipped")
		var row backuptarget.Target
		require.NoError(t, f.service.deps.DB.First(&row, "id = ?", target.ID).Error)
		require.Nil(t, row.RetiredAt)
	}
	state = "clear"
	require.NoError(t, f.service.targets.Retire(t.Context(), target.ID))
	var row backuptarget.Target
	require.NoError(t, f.service.deps.DB.First(&row, "id = ?", target.ID).Error)
	require.NotNil(t, row.RetiredAt)
	require.Empty(t, f.creates, "authoritative reference collection and delivery do not mount storage")
}

func TestNestedTargetAdmissionReusesHeldFenceWhileRetirementWaits(t *testing.T) {
	f := newRuntimePlan(t)
	require.NoError(t, f.service.deps.DB.AutoMigrate(&backuptarget.Target{}, &backuptarget.Check{}))
	crypto.InitEncryption(&crypto.Config{Environment: "test", EncryptionKey: "synthetic-encryption-key-at-least-32-bytes"})
	target, err := f.service.targets.Create(t.Context(), dto.TargetInput{TargetLocation: dto.TargetLocation{Kind: "smb", Server: "nas.example.com", Share: "archive"}, Name: "Archive"})
	require.NoError(t, err)
	f.service.targets.ConfigureDelivery(func(context.Context, backuptarget.Target) (dto.TargetReferences, []string, error) {
		return dto.TargetReferences{}, nil, nil
	}, nil)
	retired := make(chan error, 1)
	var saved context.Context
	err = f.service.withActiveTarget(t.Context(), target.ID, func(ctx context.Context) error {
		saved = ctx
		go func() { retired <- f.service.targets.Retire(t.Context(), target.ID) }()
		select {
		case <-retired:
			return errors.New("retire passed the outer admission fence")
		case <-time.After(50 * time.Millisecond):
		}
		nested := make(chan error, 1)
		go func() {
			nested <- f.service.withActiveTarget(ctx, target.ID, func(context.Context) error { ; return nil })
		}()
		select {
		case err := <-nested:
			return err
		case <-time.After(time.Second):
			return errors.New("nested capture blocked behind retirement while already owning its read fence")
		}
	})
	require.NoError(t, err)
	require.NoError(t, <-retired)
	called := false
	err = f.service.withActiveTarget(saved, target.ID, func(context.Context) error { called = true; return nil })
	require.ErrorContains(t, err, "backup.target.retired", "an old admission context cannot bypass a completed retirement")
	require.False(t, called)
}
