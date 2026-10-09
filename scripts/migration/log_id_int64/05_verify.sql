-- =============================================================================
-- 05_verify.sql  —  迁移后校验（只读，在 ClickHouse 执行）
-- 期望：[1][2][3] 均为 0；[4] 有值；[5][6] 按需替换 <db>.<table> 后人工确认。
-- =============================================================================

-- [1] 仍为 Float64 的 log_id/time_ns 列（MergeTree/Kafka）——迁移后应为 0
SELECT c.database AS db, t.engine AS engine, c.name AS col, c.type AS type, count(*) AS cnt
FROM system.columns AS c
INNER JOIN system.tables AS t ON c.database = t.database AND c.table = t.name
WHERE c.name IN ('log_id', 'time_ns')
  AND c.type IN ('Float64', 'Nullable(Float64)')
  AND t.engine IN ('MergeTree', 'Kafka')
  AND c.database IN ('test_k8s', 'test_fc', 'prod_k8s', 'prod_fc')
GROUP BY db, engine, col, type
ORDER BY db, engine, col
FORMAT PrettyCompact;

-- [2] MV 中仍用 JSONExtractFloat 提取 log_id/time_ns 的（应为 0）
SELECT database AS db, name AS mv_name
FROM system.tables
WHERE engine = 'MaterializedView'
  AND match(create_table_query, 'JSONExtractFloat\\((?:_log|JSONExtractRaw\\(_log, \'[^\']+\'\\)), \'(log_id|time_ns)\'\\)')
  AND database IN ('test_k8s', 'test_fc', 'prod_k8s', 'prod_fc')
ORDER BY db, mv_name
FORMAT PrettyCompact;

-- [3] MV 列声明里 log_id/time_ns 仍为 Float64 的（应为 0）
SELECT database AS db, name AS mv_name
FROM system.tables
WHERE engine = 'MaterializedView'
  AND (create_table_query LIKE '%`log_id` Float64%' OR create_table_query LIKE '%`time_ns` Float64%')
  AND database IN ('test_k8s', 'test_fc', 'prod_k8s', 'prod_fc')
ORDER BY db, mv_name
FORMAT PrettyCompact;

-- [4] 已改为 Int64 的列统计（迁移后应有值）
SELECT c.database AS db, t.engine AS engine, c.name AS col, c.type AS type, count(*) AS cnt
FROM system.columns AS c
INNER JOIN system.tables AS t ON c.database = t.database AND c.table = t.name
WHERE c.name IN ('log_id', 'time_ns')
  AND c.type IN ('Int64', 'Nullable(Int64)')
  AND t.engine IN ('MergeTree', 'Kafka')
  AND c.database IN ('test_k8s', 'test_fc', 'prod_k8s', 'prod_fc')
GROUP BY db, engine, col, type
ORDER BY db, engine, col
FORMAT PrettyCompact;

-- [5] 排序稳定性抽样：把 <db>.<table> 换成一张已迁移且有 log_id 的表
-- 5a. 找出同一 _time_nanosecond_ 下有多条记录的时刻
-- SELECT _time_nanosecond_, count(*) AS c FROM <db>.<table>
-- GROUP BY _time_nanosecond_ HAVING c > 1 ORDER BY c DESC LIMIT 5 FORMAT PrettyCompact;
-- 5b. 取上面某个 _time_nanosecond_ 值代入，确认按 log_id 稳定有序（升/降与查询方向一致）
-- SELECT _time_nanosecond_, log_id FROM <db>.<table>
-- WHERE _time_nanosecond_ = toDateTime64('<某时刻>', 9) ORDER BY log_id DESC FORMAT PrettyCompact;

-- [6] 新数据精度抽查：迁移后新写入的 log_id 应为完整 19 位整数（末位不再是 000 之类的舍入）
-- SELECT log_id, time_ns, _time_nanosecond_ FROM <db>.<table>
-- ORDER BY _time_nanosecond_ DESC LIMIT 20 FORMAT PrettyCompact;
