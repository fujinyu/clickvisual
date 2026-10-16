-- Post-migration verification for query_token.
-- Expected output: 3 rows, one per new table.
SELECT TABLE_NAME, TABLE_ROWS, TABLE_COMMENT
FROM INFORMATION_SCHEMA.TABLES
WHERE TABLE_SCHEMA = DATABASE()
  AND TABLE_NAME IN ('cv_query_token', 'cv_query_token_grant', 'cv_query_token_audit')
ORDER BY TABLE_NAME;

-- Expected output: 12 rows for cv_query_token columns.
SELECT COLUMN_NAME, COLUMN_TYPE, IS_NULLABLE, COLUMN_DEFAULT
FROM INFORMATION_SCHEMA.COLUMNS
WHERE TABLE_SCHEMA = DATABASE()
  AND TABLE_NAME = 'cv_query_token'
ORDER BY ORDINAL_POSITION;

-- Expected output: 7 rows for cv_query_token_grant columns.
SELECT COLUMN_NAME, COLUMN_TYPE, IS_NULLABLE, COLUMN_DEFAULT
FROM INFORMATION_SCHEMA.COLUMNS
WHERE TABLE_SCHEMA = DATABASE()
  AND TABLE_NAME = 'cv_query_token_grant'
ORDER BY ORDINAL_POSITION;

-- Expected output: 20 rows for cv_query_token_audit columns.
SELECT COLUMN_NAME, COLUMN_TYPE, IS_NULLABLE, COLUMN_DEFAULT
FROM INFORMATION_SCHEMA.COLUMNS
WHERE TABLE_SCHEMA = DATABASE()
  AND TABLE_NAME = 'cv_query_token_audit'
ORDER BY ORDINAL_POSITION;
