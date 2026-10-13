-- Source: getarcaneapp/arcane/backend/resources/migrations/postgres/081_normalize_user_identity.sql
-- Revision: 095cb778daba1034e2a39961199e712a75d4ba29
-- Retrieved: 2026-10-13
-- License: BSD-3-Clause
-- Deviations: None

-- +goose Up
-- +goose StatementBegin
DO $$
BEGIN
    IF current_setting('server_encoding') = 'UTF8' THEN
        -- Match Go's Unicode whitespace trimming before NFC normalization.
        WITH whitespace AS (
            SELECT U&'\0009\000A\000B\000C\000D\0020\0085\00A0\1680\2000\2001\2002\2003\2004\2005\2006\2007\2008\2009\200A\2028\2029\202F\205F\3000' AS chars
        )
        UPDATE users
        SET display_name = normalize(btrim(display_name, whitespace.chars), NFC)
        FROM whitespace;
    ELSE
        RAISE NOTICE 'Skipping normalization: server_encoding is %', current_setting('server_encoding');
    END IF;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- +goose Down
-- Original text cannot be reconstructed; restore it from a pre-upgrade backup.
