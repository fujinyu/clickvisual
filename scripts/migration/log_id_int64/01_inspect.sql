-- =============================================================================
-- 01_inspect.sql  —  只读盘点（在 ClickHouse 执行，不改任何数据）
-- 目的：迁移前确认影响范围、区分两种建表模式、列出必须排除/需人工确认的表。
-- 用法：整份文件可直接执行；或用 curl/clickhouse-client 逐段跑。
-- 库白名单：test_k8s / test_fc / prod_k8s / prod_fc（prod_56linked、test_56linked 无 log_id，不涉及）。
-- =============================================================================

-- [1] 受影响的列总览：按 库 / 引擎 / 列名 / 类型 统计（Float64 的 log_id、time_ns）
SELECT
    c.database                       AS db,
    t.engine                         AS engine,
    c.name                           AS col,
    c.type                           AS type,
    count(*)                         AS cnt
FROM system.columns AS c
INNER JOIN system.tables AS t ON c.database = t.database AND c.table = t.name
WHERE c.name IN ('log_id', 'time_ns')
  AND c.type IN ('Float64', 'Nullable(Float64)')
  AND c.database IN ('test_k8s', 'test_fc', 'prod_k8s', 'prod_fc')
GROUP BY db, engine, col, type
ORDER BY db, engine, col, type
FORMAT PrettyCompact;

-- [2] 必须排除：log_id 为字符串类型的表（不是数值，切勿转 Int64）
SELECT c.database AS db, c.table AS tbl, c.name AS col, c.type AS type
FROM system.columns AS c
WHERE c.name = 'log_id'
  AND c.type LIKE '%String%'
  AND c.database NOT IN ('system', 'INFORMATION_SCHEMA', 'information_schema')
ORDER BY db, tbl
FORMAT PrettyCompact;

-- [3] JSONEachRow 模式识别：流表(<t>_stream)本身含 log_id/time_ns 列的表
--     这些表的“流表列”也需要 ALTER；其余（JSONAsString）流表只有 _log，无需改流表。
SELECT c.database AS db, c.table AS stream_tbl, c.name AS col, c.type AS type
FROM system.columns AS c
INNER JOIN system.tables AS t ON c.database = t.database AND c.table = t.name
WHERE t.engine = 'Kafka'
  AND c.name IN ('log_id', 'time_ns')
  AND c.type IN ('Float64', 'Nullable(Float64)')
  AND c.database IN ('test_k8s', 'test_fc', 'prod_k8s', 'prod_fc')
ORDER BY db, stream_tbl, col
FORMAT PrettyCompact;

-- [4] 需人工确认：MV 里引用了 'log_id'，但没有标准的 `log_id` 列声明
--     （例如列名是 `toFloat64(JSONExtractString(_log, 'log_id'))` 这类自定义表达式列）。
--     本套脚本不会改这类怪异 log_id 列，需人工评估是否单独处理。
SELECT m.database AS db, m.name AS mv_name
FROM system.tables AS m
WHERE m.engine = 'MaterializedView'
  AND m.create_table_query LIKE '%''log_id''%'
  AND m.create_table_query NOT LIKE '%`log_id` %'
  AND m.database IN ('test_k8s', 'test_fc', 'prod_k8s', 'prod_fc')
ORDER BY db, mv_name
FORMAT PrettyCompact;

-- [5] 信息项：有 log_id 但没有 time_ns 的目标表（其 _time_nanosecond_ 来自秒级 time，
--     迁移只改 log_id；这类表次级排序键仍可用 log_id）。
--     注：ClickHouse 不支持父作用域列的相关子查询，这里用 GROUP BY + HAVING countIf 实现。
SELECT c.database AS db, c.table AS tbl
FROM system.columns AS c
INNER JOIN system.tables AS t ON c.database = t.database AND c.table = t.name
WHERE t.engine = 'MergeTree'
  AND c.name IN ('log_id', 'time_ns')
  AND c.database IN ('test_k8s', 'test_fc', 'prod_k8s', 'prod_fc')
GROUP BY c.database, c.table
HAVING countIf(c.name = 'log_id') > 0 AND countIf(c.name = 'time_ns') = 0
ORDER BY db, tbl
FORMAT PrettyCompact;

-- [6] 迁移前 MV 原始 DDL 备份（回滚用）：把结果保存下来！
--     建议：curl ... --data-binary "本段" > backup_mv_ddl.tsv
SELECT m.database AS db, m.name AS mv_name, m.create_table_query AS ddl
FROM system.tables AS m
WHERE m.engine = 'MaterializedView'
  AND (m.create_table_query LIKE '%''log_id''%' OR m.create_table_query LIKE '%''time_ns''%')
  AND m.database IN ('test_k8s', 'test_fc', 'prod_k8s', 'prod_fc')
ORDER BY db, mv_name
FORMAT TabSeparatedRaw;
