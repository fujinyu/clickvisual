-- =============================================================================
-- 02_gen_alter_columns.sql  —  生成器（只读！在 ClickHouse 执行只“输出”DDL，不改库）
-- 输出：把 log_id / time_ns 由 Float64|Nullable(Float64) 改为 Int64|Nullable(Int64) 的
--       ALTER TABLE ... MODIFY COLUMN 语句，覆盖 MergeTree(目标表) 与 Kafka(流表)。
-- 说明：
--   * 不处理 MaterializedView（MV 用 03_gen_rebuild_mv.sql 走 DROP+CREATE 重建）。
--   * 自动排除字符串型 log_id（type 过滤只认 Float64/Nullable(Float64)）。
--   * replaceOne(type,'Float64','Int64') 同时正确处理 Nullable(Float64)->Nullable(Int64)。
--   * 排序：test 库在前、prod 库在后，便于分批执行。
-- 用法：
--   curl.exe -sS --user admin:<pwd> http://<host>:8123/ --data-binary (Get-Content -Raw .\02_gen_alter_columns.sql) > out_alter_columns.sql
--   然后【人工审阅】 out_alter_columns.sql，再在维护窗口内执行。
-- 只迁移部分库时：修改下方 database IN (...) 白名单即可。
-- =============================================================================
SELECT concat(
           'ALTER TABLE `', c.database, '`.`', c.table, '` MODIFY COLUMN `', c.name, '` ',
           replaceOne(c.type, 'Float64', 'Int64'),
           ';  -- engine=', t.engine, ' oldType=', c.type
       ) AS alter_stmt
FROM system.columns AS c
INNER JOIN system.tables AS t ON c.database = t.database AND c.table = t.name
WHERE c.name IN ('log_id', 'time_ns')
  AND c.type IN ('Float64', 'Nullable(Float64)')
  AND t.engine IN ('MergeTree', 'Kafka')                       -- 目标表 + 流表；MV 不在此处理
  AND c.database IN ('test_k8s', 'test_fc', 'prod_k8s', 'prod_fc')
ORDER BY
    if(c.database LIKE 'test%', 0, 1) ASC,                     -- 测试库优先
    c.database ASC,
    c.table ASC,
    c.name ASC
FORMAT TabSeparatedRaw;
