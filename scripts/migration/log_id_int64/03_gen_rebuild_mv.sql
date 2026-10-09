-- =============================================================================
-- 03_gen_rebuild_mv.sql  —  生成器（只读！在 ClickHouse 执行只“输出”DDL，不改库）
-- 输出：每个相关物化视图(MV)的  DROP TABLE IF EXISTS ...;  +  CREATE MATERIALIZED VIEW ...;
--       其中把 log_id / time_ns 的提取与列声明由 Float64 改为 Int64：
--         * SELECT 中  JSONExtractFloat(<_log 或 JSONExtractRaw(_log,'父字段')>, 'log_id'|'time_ns')
--                      -> JSONExtractInt(...)
--         * 列声明中   `log_id` Float64 -> `log_id` Int64 ;  `time_ns` Float64 -> `time_ns` Int64
-- 安全性（已连生产库对 ehub / importg / forest_alarm 三样例只读验证）：
--   * 只改写 log_id / time_ns；'time'(秒)、其它字段、索引、where 一律不动。
--   * 兼容父子字段(parent 前缀)写法；JSONEachRow 的裸列 SELECT 体不变（列声明改 Int64 即可）。
--   * 怪异写法（如列名本身是 `toFloat64(JSONExtractString(_log,'log_id'))`）不会被误改，
--     这类表只会安全地修好 time_ns，其怪异 log_id 列保持原样 —— 需人工另行评估（见 01 [4]）。
-- 重要：MV 的 CREATE 文本来自当前 system.tables，务必【先跑本生成器保存输出】再执行任何 DROP，
--       否则 DROP 后源 DDL 就查不到了。执行前请用 01 [6] 备份原始 DDL 以便回滚。
-- 用法：
--   curl.exe -sS --user admin:<pwd> http://<host>:8123/ --data-binary (Get-Content -Raw .\03_gen_rebuild_mv.sql) > out_rebuild_mv.sql
--   然后【人工审阅】 out_rebuild_mv.sql，再在维护窗口内按“先 DROP 后 CREATE、逐表”执行。
-- 推荐执行序（单表）：DROP MV -> (02 的该表 ALTER：先流表后目标表) -> CREATE MV。
-- 只迁移部分库时：修改下方 database IN (...) 白名单即可。
-- =============================================================================
SELECT concat(
           'DROP TABLE IF EXISTS `', database, '`.`', name, '`;\n',
           replaceAll(
               replaceAll(
                   replaceRegexpAll(
                       create_table_query,
                       'JSONExtractFloat\\(((?:JSONExtractRaw\\(_log, \'[^\']+\'\\)|_log)), \'(log_id|time_ns)\'\\)',
                       'JSONExtractInt(\\1, \'\\2\')'
                   ),
                   '`log_id` Float64', '`log_id` Int64'
               ),
               '`time_ns` Float64', '`time_ns` Int64'
           ),
           ';'
       ) AS rebuild_stmt
FROM system.tables
WHERE engine = 'MaterializedView'
  AND (create_table_query LIKE '%''log_id''%' OR create_table_query LIKE '%''time_ns''%')
  AND database IN ('test_k8s', 'test_fc', 'prod_k8s', 'prod_fc')
ORDER BY
    if(database LIKE 'test%', 0, 1) ASC,                       -- 测试库优先
    database ASC,
    name ASC
FORMAT TabSeparatedRaw;
