// Source: docklet-next/issues/330
// Revision: 20a03b67fbeb6bcc82c758e6f2ece937d58f037a
// Retrieved: 2026-11-02
// License: Apache-2.0
// Deviations: Original regressions for startup pruning of internal backup rows (M16 candidate F05, review CFR1-1).

package settings

import (
	"context"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/florianfinn/docklet-next/backend/internal/backuptarget"
	dto "github.com/florianfinn/docklet-next/types/backup"
	"github.com/florianfinn/docklet-next/types/base"
	"github.com/stretchr/testify/require"
)

func preservedSettingKeyInternal(key string) bool {
	if _, ok := allowedSettingKeys()[key]; ok {
		return true
	}
	for _, prefix := range internalSettingPrefixes {
		if strings.HasPrefix(key, prefix) {
			return true
		}
	}
	return false
}

func TestSettingsService_PruneUnknownSettings_KeepsInternalBackupState(t *testing.T) {
	ctx := t.Context()
	db := setupSettingsTestDB(t)
	svc, err := newSettingsServiceForTestInternal(t, ctx, db)
	require.NoError(t, err)
	keys := []string{"hubHostId", "backupTargetAuthority", "backupTargetSetVersion", "backupTargetContentHash", "volumeBackupRepositoryCheck", "unknownKey"}
	for _, key := range keys {
		require.NoError(t, svc.db.WithContext(ctx).Create(&SettingVariable{Key: key, Value: "synced-" + key}).Error)
	}

	require.NoError(t, svc.PruneUnknownSettings(ctx))

	var kept []string
	require.NoError(t, svc.db.WithContext(ctx).Model(&SettingVariable{}).Where("key IN ?", keys).Order("key").Pluck("key", &kept).Error)
	require.Equal(t, []string{"backupTargetAuthority", "backupTargetContentHash", "backupTargetSetVersion", "hubHostId", "volumeBackupRepositoryCheck"}, kept)
}

// The hub rows come from the real producers: the authority and set version, and a receipt from an actual SyncHost.
func TestSettingsService_PruneUnknownSettings_KeepsHubTargetState(t *testing.T) {
	ctx := t.Context()
	db := setupSettingsTestDB(t)
	svc, err := newSettingsServiceForTestInternal(t, ctx, db)
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&backuptarget.Target{}))
	store := &backuptarget.Store{DB: db}
	authority, _, err := store.Authority(ctx)
	require.NoError(t, err)
	require.NoError(t, db.Model(&SettingVariable{}).Where("key = ?", "backup_target_set_version").Update("value", "42").Error)
	_, err = backuptarget.SyncHost(ctx, db, "host-example", func(_ context.Context, _, _, _ string, payload []byte, out any) error {
		var set dto.TargetSet
		if e := json.Unmarshal(payload, &set); e != nil {
			return e
		}
		*out.(*base.ApiResponse[dto.TargetSetReceipt]) = base.ApiResponse[dto.TargetSetReceipt]{Success: true, Data: dto.TargetSetReceipt{Authority: set.Authority, SetVersion: set.SetVersion, ContentHash: set.ContentHash, Conflicts: []string{}}}
		return nil
	})
	require.NoError(t, err)

	require.NoError(t, svc.PruneUnknownSettings(ctx))

	after, version, err := store.Authority(ctx)
	require.NoError(t, err)
	require.Equal(t, []any{authority, int64(42)}, []any{after, version})
	var receipts int64
	require.NoError(t, db.Model(&SettingVariable{}).Where("key = ?", "backupTargetDelivery:host-example").Count(&receipts).Error)
	require.EqualValues(t, 1, receipts)
}

// Every backend file outside this package that touches the settings table, with its reviewed role. A new file fails
// until it is classified here; writers must keep their keys in internalSettingKeys or internalSettingPrefixes.
var reviewedSettingFilesInternal = map[string]string{
	"internal/activity/runtime_restore.go":                "reads instanceId",
	"internal/backuptarget/delivery_state.go":             "writes backupTargetDelivery:<host>",
	"internal/backuptarget/service.go":                    "writes backup_target_set_version",
	"internal/backuptarget/store.go":                      "writes hub authority/version and agent sync rows",
	"internal/environment/agent_pairing_read.go":          "reads agentToken",
	"internal/schema/models.go":                           "registers the SettingVariable model",
	"internal/session/sign_in.go":                         "writes authLastMode (Settings field)",
	"internal/volume/bindpath/rules.go":                   "reads hubHostId",
	"internal/volume/children/backup/metadata_routes.go":  "reads volumeBackupRepositoryCheck",
	"internal/volume/children/backup/repository.go":       "reads hubHostId",
	"internal/volume/children/backup/repository_check.go": "reads hubHostId, backupTargetSetVersion; writes volumeBackupRepositoryCheck",
}

var settingSinkInternal = regexp.MustCompile(`Table\("settings"\)|(INTO|FROM|UPDATE)\s+settings\b|settings\.SettingVariable|writeSetting\(|return "settings"`)

// Literal key forms of the reviewed writers; each key found must survive pruning.
var settingWriteFormsInternal = []*regexp.Regexp{
	regexp.MustCompile(`writeSetting\(tx, "([^"]+)"`),
	regexp.MustCompile(`range map\[string\]string\{([^}]*)\}`),
	regexp.MustCompile(`INTO settings[^"]*",\s*"([^"]+)"`),
	regexp.MustCompile(`SettingVariable\{Key: "([^"]+)"`),
}

var mapKeyInternal = regexp.MustCompile(`"([^"]+)":`)

func TestPruneKeepsEveryInternalSettingWriter(t *testing.T) {
	root := filepath.Join("..", "..")
	var found []string
	seen := map[string]bool{}
	require.NoError(t, filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		source, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		text := string(source)
		if strings.HasPrefix(rel, "internal/settings/") || !settingSinkInternal.MatchString(text) {
			return nil
		}
		_, reviewed := reviewedSettingFilesInternal[rel]
		require.True(t, reviewed, "%s touches the settings table but is not classified for startup pruning", rel)
		seen[rel] = true
		for i, form := range settingWriteFormsInternal {
			for _, match := range form.FindAllStringSubmatch(text, -1) {
				if i == 1 {
					if !strings.Contains(text[strings.Index(text, match[0]):], "writeSetting(tx, key") {
						continue
					}
					for _, key := range mapKeyInternal.FindAllStringSubmatch(match[1], -1) {
						found = append(found, key[1])
					}
					continue
				}
				found = append(found, match[1])
			}
		}
		return nil
	}))
	for rel := range reviewedSettingFilesInternal {
		require.True(t, seen[rel], "%s no longer touches the settings table; remove its classification", rel)
	}
	require.GreaterOrEqual(t, len(found), 9, "the guard must see the known writers")
	for _, key := range found {
		require.True(t, preservedSettingKeyInternal(key), "startup pruning deletes %q", key)
	}
}
