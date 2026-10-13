-- Source: getarcaneapp/arcane/backend/resources/migrations/sqlite/071_rename_volume_workspace_legacy_keys.sql
-- Revision: 095cb778daba1034e2a39961199e712a75d4ba29
-- Retrieved: 2026-10-13
-- License: BSD-3-Clause
-- Deviations: None

-- +goose Up
INSERT OR IGNORE INTO settings (key, value)
SELECT 'volumeHelperIdleTimeout', value
FROM settings
WHERE key = 'volumeBrowserHelperIdleTimeout';

DELETE FROM settings WHERE key = 'volumeBrowserHelperIdleTimeout';

UPDATE roles
SET permissions = (
    SELECT json_group_array(permission)
    FROM (
        SELECT DISTINCT CASE value
            WHEN 'volumes:browse' THEN 'volumes:read'
            ELSE value
        END AS permission
        FROM json_each(roles.permissions)
    )
)
WHERE EXISTS (
    SELECT 1 FROM json_each(roles.permissions) WHERE value = 'volumes:browse'
);

DELETE FROM api_key_permissions AS legacy
WHERE legacy.permission = 'volumes:browse'
  AND EXISTS (
      SELECT 1
      FROM api_key_permissions AS current
      WHERE current.api_key_id = legacy.api_key_id
        AND current.permission = 'volumes:read'
        AND COALESCE(current.environment_id, '') = COALESCE(legacy.environment_id, '')
  );

UPDATE api_key_permissions
SET permission = 'volumes:read'
WHERE permission = 'volumes:browse';

-- +goose Down
INSERT OR IGNORE INTO settings (key, value)
SELECT 'volumeBrowserHelperIdleTimeout', value
FROM settings
WHERE key = 'volumeHelperIdleTimeout';

DELETE FROM settings WHERE key = 'volumeHelperIdleTimeout';

-- Permission grants are intentionally not reversed because volumes:read may
-- have existed before this migration and cannot be distinguished safely.
