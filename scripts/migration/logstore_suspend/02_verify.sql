-- Post-migration verification for logstore_suspend.
-- Expected output: 1 row, Column_name = suspend_status, DATA_TYPE = tinyint,
-- COLUMN_TYPE = tinyint(1), IS_NULLABLE = NO, COLUMN_DEFAULT = 0.
SELECT COLUMN_NAME, DATA_TYPE, COLUMN_TYPE, IS_NULLABLE, COLUMN_DEFAULT, COLUMN_COMMENT
FROM INFORMATION_SCHEMA.COLUMNS
WHERE TABLE_SCHEMA = DATABASE()
  AND TABLE_NAME = 'cv_base_table'
  AND COLUMN_NAME = 'suspend_status';

-- Sanity check: the column is added with DEFAULT 0, so no pre-existing log store
-- may report a detached stream table right after the migration.
-- Expected output: 0.
SELECT COUNT(*) AS rows_already_detached
FROM `cv_base_table`
WHERE `suspend_status` <> 0;
