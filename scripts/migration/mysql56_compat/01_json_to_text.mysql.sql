-- ClickVisual MySQL 5.6 compatibility migration.
-- Converts the JSON columns introduced after the PMS / query-filter features to TEXT,
-- because the JSON data type only exists from MySQL 5.7.8 onwards.
--
-- Read this file, verify against your MySQL version, and run it manually.
-- This script is NOT executed automatically by ClickVisual startup.
--
-- When you need it:
--   * Only for databases that were already initialized on MySQL 5.7 / 8.0 and are about to be
--     moved to (or dumped/restored onto) 5.6. A database created on 5.6 by the current
--     config/database.sql already has TEXT columns and needs nothing from this file.
--   * MODIFY COLUMN json -> TEXT rewrites the column in place; the stored value keeps its
--     JSON text form (e.g. ["a","b"]), which is exactly what the Go models read and write
--     (Strings / Ints in api/internal/pkg/model/db/interface.go). No row is rewritten or lost.
--   * ALTER TABLE ... MODIFY is NOT idempotent-safe in the sense of "do nothing": re-running it
--     on a column that is already TEXT succeeds and is a no-op, so the script can be re-run.
--   * Losing the JSON type also loses the server-side "value must be valid JSON" check. Values
--     are produced by json.Marshal in Go, so they stay well-formed; nothing validates them in
--     the database anymore.

ALTER TABLE `cv_pms_custom_role`
    MODIFY COLUMN `sub_resources` text NOT NULL COMMENT '角色所属refer_id资源的子资源列表',
    MODIFY COLUMN `acts` text NOT NULL COMMENT '对资源列表中各资源的actions';

ALTER TABLE `cv_pms_default_role`
    MODIFY COLUMN `sub_resources` text NOT NULL COMMENT '角色所属belongType资源下的子资源列表',
    MODIFY COLUMN `acts` text NOT NULL COMMENT '对资源列表中各资源的actions';

ALTER TABLE `cv_pms_role_detail`
    MODIFY COLUMN `sub_resources` text NOT NULL COMMENT '授权目标资源的子资源列表',
    MODIFY COLUMN `acts` text NOT NULL COMMENT '准许动作列表';

ALTER TABLE `cv_query_filter_profile`
    MODIFY COLUMN `conditions_json` text NOT NULL;
