-- Verification for 01_json_to_text.mysql.sql. Run after the migration.

-- [1] Must return 0 rows: no JSON column left in the current schema.
SELECT TABLE_NAME, COLUMN_NAME, DATA_TYPE
FROM information_schema.COLUMNS
WHERE TABLE_SCHEMA = DATABASE()
  AND DATA_TYPE = 'json';

-- [2] Must return 7 rows, all with DATA_TYPE = 'text'.
SELECT TABLE_NAME, COLUMN_NAME, DATA_TYPE, IS_NULLABLE
FROM information_schema.COLUMNS
WHERE TABLE_SCHEMA = DATABASE()
  AND ((TABLE_NAME IN ('cv_pms_custom_role', 'cv_pms_default_role', 'cv_pms_role_detail')
        AND COLUMN_NAME IN ('sub_resources', 'acts'))
    OR (TABLE_NAME = 'cv_query_filter_profile' AND COLUMN_NAME = 'conditions_json'))
ORDER BY TABLE_NAME, COLUMN_NAME;

-- [3] Must be 0: the alarm queries now match table_ids as a plain-text array ("[1,7]").
SELECT COUNT(*) AS alarm_rows_not_in_array_form
FROM `cv_alarm`
WHERE `table_ids` NOT LIKE '[%]';
