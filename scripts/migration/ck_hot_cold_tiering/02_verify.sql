-- Post-migration verification for ck_hot_cold_tiering.
-- Expected output: 3 rows with Column_name in {storage_policy, cold_volume, hot_days}.
SELECT COLUMN_NAME, DATA_TYPE, COLUMN_TYPE, IS_NULLABLE, COLUMN_DEFAULT, COLUMN_COMMENT
FROM INFORMATION_SCHEMA.COLUMNS
WHERE TABLE_SCHEMA = DATABASE()
  AND TABLE_NAME = 'cv_base_table'
  AND COLUMN_NAME IN ('storage_policy', 'cold_volume', 'hot_days')
ORDER BY ORDINAL_POSITION;

-- Sanity check: no legacy row should have been affected (all defaults empty/0).
SELECT COUNT(*) AS rows_with_non_default_tiering
FROM `cv_base_table`
WHERE `storage_policy` <> '' OR `cold_volume` <> '' OR `hot_days` <> 0;
