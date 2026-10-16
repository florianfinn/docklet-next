# SQLite removal map for the PostgreSQL baseline

Archived on 2026-10-16 from the pre-SQLite design at
`1ead6d2c7e17c79b46c42eb8567c3a1d70730fd5`. This records the M1 SQLite removal
inventory, not the current database decision. See the
[SQLite database contract](../design/sqlite-database.md).

Paths below are relative to Arcane v2.15.1 at
`095cb778daba1034e2a39961199e712a75d4ba29`, inspected on 2026-10-13.
This is the removal input to #7, not a claim that the code was changed here.
The [adoption decision](../design/arcane-adoption.md) owns the selected revision.

## Runtime and migration paths

| Source path | Required cut or adaptation |
| --- | --- |
| `backend/go.mod` | Remove direct `github.com/libtnb/sqlite` and `modernc.org/sqlite`; remove indirect `github.com/mattn/go-sqlite3` when no retained import needs it |
| `backend/go.sum`, `go.work.sum` | Regenerate after the import cut; remove unused modernc libc/mathutil/memory and other SQLite-only dependencies through Go tooling, not an indiscriminate name deletion |
| `backend/internal/database/database.go` | Remove SQLite provider constant, `file:` selection, dialector and scalar registration, directory creation, DSN parser/pragmas, SQLite Goose dialect, sqlite_master queries and SQLite placeholder/legacy-version branches; keep PostgreSQL migration and locking semantics |
| `backend/pkg/utils/sqlite/sqlite.go` | Remove the modernc scalar-function registration package |
| `backend/resources/migrations/sqlite/` | Remove all 90 files listed below; the fresh PostgreSQL baseline does not import SQLite migration history |
| `backend/resources/embed.go` | Ensure the `migrations` embed no longer contains SQLite assets; retain other embedded resources |
| `backend/pkg/utils/francis/provider.go` | Remove SQLite provider import, `StorePath`, SQLite `StoreURL` branch, `WithSQLiteProvider`, `NewSQLiteProvider`, pragma handling and legacy sidecar migration; keep PostgreSQL actor persistence |
| `backend/pkg/utils/francis/runtime.go` | Remove the SQLite GORM branch and busy-timeout DSN from actor runtime construction |
| `backend/pkg/utils/francis/testing/host.go` | Replace the test helper's SQLite URL assumption; preserve pure actor/runtime tests and separately gated PostgreSQL persistence tests |
| `backend/pkg/libarcane/startup/runtime_identity.go` | Remove SQLite database/sidecar path resolution and file creation/chown assumptions; retain project/volume runtime identity behaviour |
| `backend/internal/system/children/backup/service.go` | Replace SQLite-only provider support, database-file discovery, recovery eligibility and scheduled-backup assumptions; report capability unavailable until PostgreSQL backup/recovery is implemented |
| `backend/internal/system/children/backup/children/snapshots/service.go` | Remove SQLite sidecar suffix handling, file-based database manifest interpretation and database-file restoration; retain generic archive validation where applicable |
| `backend/cli/recovery/recovery.go` | Remove SQLite database opening and provider validation; no import of existing Arcane recovery archives |
| `types/recovery/recovery.go` | Remove SQLite-specific archive layout assumptions from the recovery contract; coordinate retained backup DTOs with the new implementation |
| `backend/internal/diagnostics/model.go` | Remove SQLite epoch timestamp compatibility from Francis timestamp scanning; preserve PostgreSQL timestamps |
| `backend/internal/activity/service.go` | Reassess SQLite contention/fsync assumptions and comments; retain batching/retry only where justified for PostgreSQL |
| `backend/internal/project/service.go` | Review the old database-specific commentary while retaining project behaviour |
| `backend/internal/variable/service.go` | The SQLite FK-pragma comment belongs to a feature removed by the functional cut; do not preserve its SQLite rationale in retained code |

The helper `go.getarcane.app/kit` also exports `SQLitePathFromDSN`; remove unused
SQLite-only helpers in the locally adopted Kit module after removing callers.
Its path is relative to Kit, not Arcane, and its separate source SHA is in the
module map. Francis remains an external dependency: removing the import of
`github.com/italypaleale/francis/components/sqlite` is essential even when the
main database is PostgreSQL. Verify the **actual binary graph** afterwards;
`go.mod` text alone cannot prove that a driver is absent.

## Configuration, builds and non-Go references

| Source path | Required change |
| --- | --- |
| `backend/internal/config/config.go` | Keep `DATABASE_URL`/`Config.DatabaseURL` with PostgreSQL-only validation; remove the default `file:data/arcane.db` DSN and `_pragma=journal_mode(WAL)`, `_pragma=busy_timeout(2500)`, `_txlock=immediate` values |
| `.env.example` | Replace the SQLite `DATABASE_URL` example with a reserved PostgreSQL example; `ALLOW_DOWNGRADE` belongs to the separate forward-only migration cut, not a SQLite toggle |
| `docker/compose.dev.yaml` | Replace both manager/agent SQLite `DATABASE_URL` settings and database-file persistence assumptions |
| `Justfile` | Remove the edge-test-agent SQLite fallback DSN and database-file assumptions |
| `.github/workflows/ci.yml`, `.depot/workflows/ci.yml` | Remove SQLite matrix entries and adapt persistence testing to the PostgreSQL baseline |
| `.goreleaser.yaml` | There is **no SQLite build tag** to remove; recheck both hub and agent dependency graphs for their target configurations |
| `docker/Dockerfile`, `docker/Dockerfile-agent` | `BUILD_TAGS` defaults are empty and `exclude_frontend`, respectively. Neither disables SQLite; validate graph removal for both |
| `docker/Dockerfile-static`, `docker/Dockerfile-agent-static` | Consume compiled binaries; confirm that packaging contains no SQLite database assets |
| `tests/spec/runtime.spec.ts` | Replace database-file creation/ownership assertions on `arcane.db`; retain relevant runtime UID/GID and project-file assertions |
| `.github/ISSUE_TEMPLATE/bug.yml` | Remove SQLite-specific diagnostic prompt when adapting the upstream issue template |
| `CHANGELOG.md` | Historical upstream SQLite references are not adopted product documentation; do not rewrite upstream history as evidence of a runtime change |

No `//go:build` directive in Arcane selects SQLite versus PostgreSQL. Relevant
existing tags are `exclude_frontend`, `buildables`, `playwright` and platform/
architecture tags. They must not be described as SQLite-removal switches.
No separate `DATABASE_TYPE`, `DB_DRIVER` or SQLite environment toggle exists in
the inspected config: provider selection follows `DATABASE_URL`. The DSN parser
in `database.go` also handles memory-mode and pragma options; remove that whole
SQLite-only interface, not just the default string.

## Test files

The following exact paths contain SQLite drivers, database-file assumptions or
SQLite fixture setup. Rewrite retained behaviour checks; delete tests only when
their covered feature is removed. Default tests must continue without a real
service or Docker socket. PostgreSQL integration checks belong behind explicit
integration setup, not silent skips masquerading as default persistence coverage.

- `backend/cli/admin/reset_mfa_test.go`
- `backend/cli/admin/reset_password_test.go`
- `backend/cli/recovery/recovery_test.go`
- `backend/internal/activity/service_test.go`
- `backend/internal/apikey/service_test.go`
- `backend/internal/auth/handler_test.go`
- `backend/internal/auth/service_test.go`
- `backend/internal/backup/service_test.go`
- `backend/internal/bootstrap/bootstrap_test.go`
- `backend/internal/build/children/workspace/service_test.go`
- `backend/internal/build/service_test.go`
- `backend/internal/container/service_test.go`
- `backend/internal/dashboard/service_test.go`
- `backend/internal/database/database_test.go`
- `backend/internal/di/di_test.go`
- `backend/internal/environment/service_test.go`
- `backend/internal/event/service_test.go`
- `backend/internal/federated/service_test.go`
- `backend/internal/gitops/children/sync/service_test.go`
- `backend/internal/gitops/service_test.go`
- `backend/internal/gitrepo/service_test.go`
- `backend/internal/image/children/patch/service_test.go`
- `backend/internal/image/service_test.go`
- `backend/internal/imageupdate/service_test.go`
- `backend/internal/job/service_test.go`
- `backend/internal/kv/service_test.go`
- `backend/internal/notification/service_test.go`
- `backend/internal/oidc/service_test.go`
- `backend/internal/passkey/service_test.go`
- `backend/internal/project/service_test.go`
- `backend/internal/registry/service_test.go`
- `backend/internal/role/service_test.go`
- `backend/internal/s3/service_test.go`
- `backend/internal/session/service_test.go`
- `backend/internal/settings/service_test.go`
- `backend/internal/swarm/children/nodes/service_test.go`
- `backend/internal/swarm/children/stacks/service_test.go`
- `backend/internal/system/children/backup/children/snapshots/service_test.go`
- `backend/internal/system/children/backup/service_test.go`
- `backend/internal/system/children/upgrade/service_test.go`
- `backend/internal/system/service_test.go`
- `backend/internal/template/service_test.go`
- `backend/internal/updater/service_test.go`
- `backend/internal/user/service_test.go`
- `backend/internal/variable/service_test.go`
- `backend/internal/version/service_test.go`
- `backend/internal/volume/children/backup/children/policies/service_test.go`
- `backend/internal/volume/children/backup/helpers_test.go`
- `backend/internal/volume/children/backup/service_test.go`
- `backend/internal/volume/service_test.go`
- `backend/internal/vulnerability/children/query/service_test.go`
- `backend/internal/vulnerability/children/scan/children/trivy/service_test.go`
- `backend/internal/vulnerability/children/scan/service_test.go`
- `backend/internal/vulnerability/service_test.go`
- `backend/internal/webhook/service_test.go`
- `backend/pkg/libarcane/startup/runtime_identity_test.go`
- `backend/pkg/pagination/reflect_test.go`
- `backend/pkg/pagination/skipcount_test.go`
- `backend/pkg/scheduler/actor_test.go`
- `backend/pkg/scheduler/analytics_job_test.go`
- `backend/pkg/scheduler/auto_heal_job_test.go`
- `backend/pkg/scheduler/job_schedule_integration_test.go`
- `backend/pkg/scheduler/runs/actors_test.go`
- `backend/pkg/scheduler/runs/resolution_test.go`
- `backend/pkg/scheduler/scheduled_prune_job_test.go`
- `backend/pkg/utils/dbutil/dbutil_test.go`
- `backend/pkg/utils/francis/provider_test.go`
- `backend/pkg/utils/francis/runtime_test.go`

Two further keyword matches are **generic file fixtures**, not database engines:
- `backend/pkg/projects/workspace_test.go`
- `backend/pkg/utils/backupbrowser/browser_test.go`

These exercise copying or browsing a file with a database-like filename. Keep
the behaviour tests; rename fixtures only if clearer. A grep match alone is not
a reason to remove unrelated file operations.

## Migration files

- `backend/resources/migrations/sqlite/001_initial_schema.sql`
- `backend/resources/migrations/sqlite/002_create_events_table.sql`
- `backend/resources/migrations/sqlite/003_remove_events_environment_fk.sql`
- `backend/resources/migrations/sqlite/004_project_cache.sql`
- `backend/resources/migrations/sqlite/005_drop_user_sessions_table.sql`
- `backend/resources/migrations/sqlite/006_rename_tables.sql`
- `backend/resources/migrations/sqlite/007_add_user_oidc_tokens.sql`
- `backend/resources/migrations/sqlite/008_clean_compose_templates.sql`
- `backend/resources/migrations/sqlite/009_add_image_update_auth_fields.sql`
- `backend/resources/migrations/sqlite/010_rework_environments.sql`
- `backend/resources/migrations/sqlite/011_add_environment_name.sql`
- `backend/resources/migrations/sqlite/012_add_user_locale.sql`
- `backend/resources/migrations/sqlite/013_stacks_to_projects.sql`
- `backend/resources/migrations/sqlite/014_containers_project_fk.sql`
- `backend/resources/migrations/sqlite/015_rename_stacks_directory_setting.sql`
- `backend/resources/migrations/sqlite/016_drop_image_table.sql`
- `backend/resources/migrations/sqlite/017_drop_unused_docker_tables.sql`
- `backend/resources/migrations/sqlite/018_add_project_status_reason.sql`
- `backend/resources/migrations/sqlite/019_add_user_requires_password_change.sql`
- `backend/resources/migrations/sqlite/020_remove_mobile_navigation_scroll_to_hide_setting.sql`
- `backend/resources/migrations/sqlite/021_add_notifications.sql`
- `backend/resources/migrations/sqlite/022_unique_oidc_subject_index.sql`
- `backend/resources/migrations/sqlite/023_add_apprise_settings.sql`
- `backend/resources/migrations/sqlite/024_add_api_keys.sql`
- `backend/resources/migrations/sqlite/025_add_environment_api_key_pairing.sql`
- `backend/resources/migrations/sqlite/026_add_notification_sent_to_image_updates.sql`
- `backend/resources/migrations/sqlite/027_add_missing_indexes.sql`
- `backend/resources/migrations/sqlite/028_add_gitops.sql`
- `backend/resources/migrations/sqlite/029_add_ssh_host_key_verification.sql`
- `backend/resources/migrations/sqlite/030_remove_gitops_sync_enabled.sql`
- `backend/resources/migrations/sqlite/031_add_environment_is_edge.sql`
- `backend/resources/migrations/sqlite/032_add_volume_backups.sql`
- `backend/resources/migrations/sqlite/033_add_vulnerability_scans.sql`
- `backend/resources/migrations/sqlite/034_add_vulnerability_ignores.sql`
- `backend/resources/migrations/sqlite/035_backfill_missing_settings.sql`
- `backend/resources/migrations/sqlite/036_notification_log_image_ref_text.sql`
- `backend/resources/migrations/sqlite/037_add_image_builds.sql`
- `backend/resources/migrations/sqlite/038_create_kv_table.sql`
- `backend/resources/migrations/sqlite/039_add_ecr_registry_support.sql`
- `backend/resources/migrations/sqlite/040_add_api_key_managed_by.sql`
- `backend/resources/migrations/sqlite/041_add_query_performance_indexes.sql`
- `backend/resources/migrations/sqlite/042_add_directory_sync.sql`
- `backend/resources/migrations/sqlite/043_add_hidden_swarm_node_agents.sql`
- `backend/resources/migrations/sqlite/044_allow_nested_project_paths.sql`
- `backend/resources/migrations/sqlite/045_add_webhooks.sql`
- `backend/resources/migrations/sqlite/046_nullable_api_key_user_id.sql`
- `backend/resources/migrations/sqlite/047_add_gitops_sync_target_type.sql`
- `backend/resources/migrations/sqlite/048_add_compose_project_name.sql`
- `backend/resources/migrations/sqlite/049_add_projects_name_index.sql`
- `backend/resources/migrations/sqlite/050_add_project_archived.sql`
- `backend/resources/migrations/sqlite/051_add_user_sessions.sql`
- `backend/resources/migrations/sqlite/052_add_project_image_refs.sql`
- `backend/resources/migrations/sqlite/053_drop_deprecated_v2.sql`
- `backend/resources/migrations/sqlite/054_add_rbac.sql`
- `backend/resources/migrations/sqlite/055_add_activities.sql`
- `backend/resources/migrations/sqlite/056_add_federated_credentials.sql`
- `backend/resources/migrations/sqlite/057_add_user_font_size.sql`
- `backend/resources/migrations/sqlite/058_add_environment_last_edge_transport.sql`
- `backend/resources/migrations/sqlite/059_add_api_key_kind.sql`
- `backend/resources/migrations/sqlite/060_add_environment_update_jobs.sql`
- `backend/resources/migrations/sqlite/061_add_user_avatar.sql`
- `backend/resources/migrations/sqlite/062_add_gitops_sync_pre_deploy_hook.sql`
- `backend/resources/migrations/sqlite/063_drop_notification_logs.sql`
- `backend/resources/migrations/sqlite/064_add_user_time_format.sql`
- `backend/resources/migrations/sqlite/065_add_project_build_image_refs.sql`
- `backend/resources/migrations/sqlite/066_add_global_variables.sql`
- `backend/resources/migrations/sqlite/067_add_activity_batch_id.sql`
- `backend/resources/migrations/sqlite/068_add_user_preferences.sql`
- `backend/resources/migrations/sqlite/069_add_container_registry_repository_names.sql`
- `backend/resources/migrations/sqlite/070_add_passkeys_and_mfa.sql`
- `backend/resources/migrations/sqlite/071_rename_volume_workspace_legacy_keys.sql`
- `backend/resources/migrations/sqlite/072_add_project_tags.sql`
- `backend/resources/migrations/sqlite/073_add_backup_support.sql`
- `backend/resources/migrations/sqlite/074_add_gitops_sync_pull_redeploy.sql`
- `backend/resources/migrations/sqlite/075_add_image_patches.sql`
- `backend/resources/migrations/sqlite/076_move_vulnerability_reports_to_db.sql`
- `backend/resources/migrations/sqlite/077_add_apns.sql`
- `backend/resources/migrations/sqlite/078_add_vulnerability_scan_items.sql`
- `backend/resources/migrations/sqlite/079_add_vulnerability_scan_policy.sql`
- `backend/resources/migrations/sqlite/080_drop_legacy_require_password_change.sql`
- `backend/resources/migrations/sqlite/081_normalize_user_identity.sql`
- `backend/resources/migrations/sqlite/082_remove_job_key_versions.sql`
- `backend/resources/migrations/sqlite/083_add_container_image_updates.sql`
- `backend/resources/migrations/sqlite/084_route_job_activities_to_environment.sql`
- `backend/resources/migrations/sqlite/085_add_gitops_backup_mode.sql`
- `backend/resources/migrations/sqlite/086_add_git_repository_commit_identity.sql`
- `backend/resources/migrations/sqlite/087_add_volume_backup_remote_instance.sql`
- `backend/resources/migrations/sqlite/088_add_vulnerability_risk.sql`
- `backend/resources/migrations/sqlite/089_add_vulnerability_scoring_version.sql`
- `backend/resources/migrations/sqlite/090_add_event_deduplication_key.sql`

## Verification when implementing #7

Enumerate tracked matches with `git grep -ilE
'sqlite|arcane\.db|_pragma|journal_mode|busy_timeout'`, list the entire migrations
directory with `git ls-files backend/resources/migrations/sqlite`, and inspect
provider selection and all `//go:build` directives. Review keyword matches by
meaning, including generated sums and historical text. After the cut, repeat
`go list -deps` for both command build configurations and confirm the absence
of SQLite driver/provider packages. Run retained behaviour tests and the
separately configured PostgreSQL migration/persistence checks. The inventory
from #5 is a pre-cut reference and must not be relabelled as post-cut evidence.


Acceptance for #7 explicitly requires fresh linux/amd64 hub (CGO=1, default tags)
and agent (CGO=0, `exclude_frontend`) `go list -deps -json` evidence showing **zero
packages whose module path is `modernc.org/libc` or `modernc.org/sqlite`** in each
graph. Record the commands, counts and post-cut revision. This removes the two
compiled glibc LGPL-2.1-or-later header notices and the UUID header's GCC notice
recorded in the adoption inventory; the narrow pre-cut header assessment does not
permit retaining those modules in the product. A missing driver import alone,
a module-name deletion from sums, or only one measured target is insufficient.
