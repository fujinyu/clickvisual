-- =============================================================================
-- 04_update_anyjson.mysql.sql  —  在【ClickVisual 元数据库 MySQL】执行（不是 ClickHouse！）
-- 目的：把建表映射 any_json 里 log_id / time_ns 的类型由 Float64 改为 Int64。
--       否则日后在 ClickVisual 里编辑该表索引时，updateSwitcher 会按旧类型(Float64)
--       重建 MV，精度又退化回 Float64。
-- 目标表：cv_base_table.any_json（TEXT，内容为 json.Marshal(ReqStorageCreate)）。
-- JSON 形态（Go 默认紧凑无空格）："SourceMapping":{"data":[{"key":"log_id","value":"Float64","parent":""}, ...]}
--
-- ⚠️ 执行前务必：
--   1) 备份 cv_base_table（至少备份将被修改的行）：
--        CREATE TABLE cv_base_table_bak_20261009 AS SELECT * FROM cv_base_table;
--   2) 先跑【步骤 A】确认实际 JSON 文本格式与受影响行数；若你的数据里冒号后带空格
--      （如 "key": "log_id"），请把下方 REPLACE/LIKE 的模式同步加上空格再执行。
--   3) 全程在事务中执行，确认 rowcount 合理后再 COMMIT。
-- =============================================================================

-- ---------- 步骤 A：只读盘点（先看，不改）----------
-- A1. 看一条真实样本，确认 JSON 文本格式（重点看 log_id/time_ns 附近）
SELECT id, SUBSTRING(any_json, GREATEST(1, LOCATE('log_id', any_json) - 20), 120) AS around_log_id
FROM cv_base_table
WHERE any_json LIKE '%"key":"log_id","value":"Float64"%'
LIMIT 5;

-- A2. 受影响行数统计
SELECT
    SUM(any_json LIKE '%"key":"log_id","value":"Float64"%')  AS rows_log_id_float,
    SUM(any_json LIKE '%"key":"time_ns","value":"Float64"%') AS rows_time_ns_float
FROM cv_base_table;

-- ---------- 步骤 B：事务内更新（确认 A 的结果无误后再执行）----------
START TRANSACTION;

UPDATE cv_base_table
SET any_json = REPLACE(any_json, '"key":"log_id","value":"Float64"', '"key":"log_id","value":"Int64"')
WHERE any_json LIKE '%"key":"log_id","value":"Float64"%';

UPDATE cv_base_table
SET any_json = REPLACE(any_json, '"key":"time_ns","value":"Float64"', '"key":"time_ns","value":"Int64"')
WHERE any_json LIKE '%"key":"time_ns","value":"Float64"%';

-- 检查两次 UPDATE 影响的行数是否与 A2 统计一致；一致再提交，否则回滚：
--   COMMIT;
--   ROLLBACK;
COMMIT;

-- ---------- 步骤 C：更新后复核（应为 0）----------
SELECT
    SUM(any_json LIKE '%"key":"log_id","value":"Float64"%')  AS remain_log_id_float,
    SUM(any_json LIKE '%"key":"time_ns","value":"Float64"%') AS remain_time_ns_float
FROM cv_base_table;
