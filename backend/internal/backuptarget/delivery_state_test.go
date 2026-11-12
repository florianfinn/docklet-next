// Source: docklet-next/issues/289
// Revision: b9b59ae92f1b09eb828940860971002eab9267b6
// Retrieved: 2026-11-01
// License: Apache-2.0
// Deviations: Original M16 bind identity and admission rules; M16 bind mounts and backup completion.

package backuptarget

import (
	"context"
	"encoding/json"
	"errors"
	dto "github.com/florianfinn/docklet-next/types/backup"
	"github.com/florianfinn/docklet-next/types/base"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func deliveryHost(t *testing.T, s *Store) {
	t.Helper()
	require.NoError(t, s.DB.Exec("CREATE TABLE environments (id TEXT PRIMARY KEY, enabled BOOLEAN, credential_revision INTEGER NOT NULL)").Error)
	require.NoError(t, s.DB.Exec("INSERT INTO environments VALUES ('host-a',true,1)").Error)
}
func echoReceipt(t *testing.T, payload []byte, out any) dto.TargetSet {
	t.Helper()
	var set dto.TargetSet
	require.NoError(t, json.Unmarshal(payload, &set))
	*out.(*base.ApiResponse[dto.TargetSetReceipt]) = base.ApiResponse[dto.TargetSetReceipt]{Success: true, Data: dto.TargetSetReceipt{Authority: set.Authority, SetVersion: set.SetVersion, ContentHash: set.ContentHash, Conflicts: []string{}}}
	return set
}
func readDelivery(t *testing.T, s *Store) deliveryRecord {
	t.Helper()
	value, err := readSetting(s.DB.DB, "backupTargetDelivery:host-a")
	require.NoError(t, err)
	var record deliveryRecord
	if value != "" {
		require.NoError(t, json.Unmarshal([]byte(value), &record))
	}
	return record
}
func TestDeliveryLateRegistrationAndSetResponses(t *testing.T) {
	for _, rotate := range []bool{false, true} {
		t.Run(map[bool]string{false: "older-set", true: "rotated-registration"}[rotate], func(t *testing.T) {
			store := testStore(t)
			deliveryHost(t, store)
			svc := NewService(Dependencies{Store: store})
			target, err := svc.Create(t.Context(), dto.TargetInput{TargetLocation: dto.TargetLocation{Kind: "smb", Server: "nas.example.com", Share: "archive"}, Name: "Archive"})
			require.NoError(t, err)
			sent, release := make(chan struct{}), make(chan struct{})
			done := make(chan error, 1)
			go func() {
				_, e := SyncHost(t.Context(), store.DB, "host-a", func(_ context.Context, _, _, _ string, payload []byte, out any) error {
					echoReceipt(t, payload, out)
					close(sent)
					<-release
					return nil
				})
				done <- e
			}()
			<-sent
			if rotate {
				require.NoError(t, store.DB.Exec("UPDATE environments SET credential_revision=2 WHERE id='host-a'").Error)
			} else {
				_, err = svc.Update(t.Context(), target.ID, dto.TargetInput{TargetLocation: target.location(), Name: "Changed"})
				require.NoError(t, err)
				_, err = SyncHost(t.Context(), store.DB, "host-a", func(_ context.Context, _, _, _ string, payload []byte, out any) error {
					echoReceipt(t, payload, out)
					return nil
				})
				require.NoError(t, err)
			}
			close(release)
			require.NoError(t, <-done)
			record := readDelivery(t, store)
			if rotate {
				require.Empty(t, record.Authority)
				require.Nil(t, record.LastAttempt)
			} else {
				require.EqualValues(t, target.ConfigVersion+1, record.SetVersion)
				require.EqualValues(t, target.ConfigVersion+1, record.Targets[target.ID])
				require.EqualValues(t, target.ConfigVersion+1, record.LastAttempt.SetVersion)
			}
		})
	}
}
func TestDeliveryProjectionIdentityAndPrecedence(t *testing.T) {
	now := time.Now().UTC()
	target := Target{ID: "target", ConfigVersion: 2}
	record := deliveryRecord{TargetSetReceipt: dto.TargetSetReceipt{Authority: "authority", SetVersion: 3, Conflicts: []string{"target"}}, RegistrationRevision: 1, Targets: map[string]int64{"target": 2}, At: now, LastAttempt: &deliveryAttempt{Authority: "authority", RegistrationRevision: 1, SetVersion: 3, Outcome: "backup.target.sync-failed", At: now}}
	require.Equal(t, "conflict", projectDelivery(record, "authority", 1, 3, target, "host-a").State)
	rotated := projectDelivery(record, "authority", 2, 3, target, "host-a")
	require.Equal(t, "pending", rotated.State)
	require.Empty(t, rotated.Code)
	record.Conflicts = nil
	current := projectDelivery(record, "authority", 1, 3, target, "host-a")
	require.Equal(t, "current", current.State)
	require.Empty(t, current.Code, "offline after a current receipt stays current")
	record.LastAttempt.Outcome = "backup.target.wrong-host"
	require.Equal(t, "conflict", projectDelivery(record, "authority", 1, 3, target, "host-a").State)
	target.ConfigVersion = 4
	record.LastAttempt.Outcome = "backup.target.sync-failed"
	pending := projectDelivery(record, "authority", 1, 4, target, "host-a")
	require.Equal(t, "pending", pending.State)
	require.Equal(t, "backup.target.sync-failed", pending.Code)
	require.Equal(t, "pending", projectDelivery(record, "new-authority", 1, 1, target, "host-a").State)
}
func TestDeliveryNewAuthorityAcceptsLowerCounter(t *testing.T) {
	store := testStore(t)
	deliveryHost(t, store)
	_, _, err := store.Authority(t.Context())
	require.NoError(t, err)
	old := deliveryRecord{TargetSetReceipt: dto.TargetSetReceipt{Authority: "old", SetVersion: 99}, RegistrationRevision: 1}
	data, err := json.Marshal(old)
	require.NoError(t, err)
	require.NoError(t, writeSetting(store.DB.DB, "backupTargetDelivery:host-a", string(data)))
	_, err = SyncHost(t.Context(), store.DB, "host-a", func(_ context.Context, _, _, _ string, payload []byte, out any) error {
		echoReceipt(t, payload, out)
		return nil
	})
	require.NoError(t, err)
	require.EqualValues(t, 1, readDelivery(t, store).SetVersion)
}

func TestDeliveryLateFailureCannotReplaceNewerAttempt(t *testing.T) {
	store := testStore(t)
	deliveryHost(t, store)
	svc := NewService(Dependencies{Store: store})
	target, err := svc.Create(t.Context(), dto.TargetInput{TargetLocation: dto.TargetLocation{Kind: "smb", Server: "nas.example.com", Share: "archive"}, Name: "Archive"})
	require.NoError(t, err)
	entered, release := make(chan struct{}), make(chan struct{})
	done := make(chan error, 1)
	go func() {
		_, e := SyncHost(t.Context(), store.DB, "host-a", func(context.Context, string, string, string, []byte, any) error {
			close(entered)
			<-release
			return dto.NewProblem(422, "backup.target.wrong-host", nil)
		})
		done <- e
	}()
	<-entered
	_, err = svc.Update(t.Context(), target.ID, dto.TargetInput{TargetLocation: target.location(), Name: "Changed"})
	require.NoError(t, err)
	_, err = SyncHost(t.Context(), store.DB, "host-a", func(context.Context, string, string, string, []byte, any) error { return errors.New("offline") })
	require.Error(t, err)
	close(release)
	require.Error(t, <-done)
	record := readDelivery(t, store)
	require.NotNil(t, record.LastAttempt)
	require.EqualValues(t, target.ConfigVersion+1, record.LastAttempt.SetVersion)
	require.Equal(t, "backup.target.sync-failed", record.LastAttempt.Outcome)
}
