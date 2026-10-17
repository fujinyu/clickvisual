# MySQL 5.6 兼容迁移

目标库是 MySQL 5.6（本地 `clickvisual_test` 与正式库均为 5.6）。ClickVisual 上游基线是 5.7+：
JSON 列类型（5.7.8 引入）与 `JSON_CONTAINS` / `JSON_EXTRACT` 函数在 5.6 上不存在。
本目录记录为把元数据库降到 5.6 而做的 schema 变更；代码侧改造见下方"关联代码改动"。

## 影响范围

- 只动 **ClickVisual 元数据库（MySQL）**，不涉及 ClickHouse / Databend。
- 7 个 JSON 列改为 TEXT：`cv_pms_custom_role`、`cv_pms_default_role`、`cv_pms_role_detail` 各 2 列，
  `cv_query_filter_profile.conditions_json` 1 列。
- 存量数据不丢失：Go 侧 `Strings` / `Ints` 的 `Value()` 写入的本来就是 JSON 文本串
  （见 [api/internal/pkg/model/db/interface.go:155](../../../api/internal/pkg/model/db/interface.go)），
  `Scan()` 同时接受 `[]byte` 与 `string`，所以 JSON → TEXT 后读写路径不变。

## 执行顺序

```
1) 01_json_to_text.mysql.sql   -- 生产 MySQL 上手动执行（仅存量 5.7/8.0 库需要）
2) 02_verify.sql               -- 校验：无 json 列残留、7 列为 text、table_ids 仍是 "[...]" 形态
```

全新 5.6 库不需要跑本脚本：`config/database.sql` 现在直接建 TEXT 列，用 `init` 建库即可。

## 行为差异（必须知道）

1. **失去 JSON 合法性校验**。数据库不再拒绝非法 JSON；值由 Go `json.Marshal` 生成，形态有保证。
2. **`AlarmListByDidPage` 的 JOIN 结果会变宽**。原 5.7 写法是
   `cv_base_table.id IN (replace(replace(JSON_EXTRACT(table_ids,'$[*]'),'[',''),']',''))`，
   整个 `JSON_EXTRACT` 结果是一个字符串 `'1,7'`，`IN ('1,7')` 会被 MySQL 隐式截断成 `1`，
   即**只有数组首元素能命中**。改为文本列表包含判断后，数组中**任一**元素都会命中——
   这是代码注释与本意（告警关联该库下任意表）一致的修正。
   **待验证**：请在有数据的库上对比改前/改后同一 `did` 的告警列表条数。
3. **文本匹配走不了索引**。`LIKE '%,1,%'` 与原 `JSON_CONTAINS` 同样无法用索引，`cv_alarm` 是元数据库
   小表，量级上不改变复杂度；告警条数上万后需复核列表接口耗时。**待验证**。

## 关联代码改动

- [config/database.sql:451](../../../config/database.sql) 等 6 处 `json` → `text`。
- [scripts/migration/database.sql:98](../database.sql)：`conditions_json` → `text`；
  同时 `idx_query_filter_scope` 改为前缀索引 `(instance_id, database_name(191), table_name(191))`。
  原因：`database_name` / `table_name` 是 `varchar(255) utf8mb4`，单列索引键 1020 字节，
  超过 5.6 InnoDB 默认 767 字节前缀上限，建表会直接报 `Error 1071`；191×4=764 是 utf8mb4 的安全前缀。
  若某套 5.7 库已由 GORM AutoMigrate 建出**全列**索引，schema 拷贝回 5.6 时需用
  `ALTER TABLE cv_query_filter_profile DROP INDEX idx_query_filter_scope, ADD INDEX idx_query_filter_scope (instance_id, database_name(191), table_name(191));`
- [api/internal/pkg/model/db/pms.go:59](../../../api/internal/pkg/model/db/pms.go) 等 6 处 `type:json` → `type:text`。
- [api/internal/pkg/model/db/query_filter_profile.go:15](../../../api/internal/pkg/model/db/query_filter_profile.go)：
  `type:json` → `type:text`，并给两个索引列加 `length:191`，与上面 DDL 保持一致（GORM 会生成前缀索引）。
- [api/internal/pkg/model/db/alarm.go:359](../../../api/internal/pkg/model/db/alarm.go)：
  `alarmTidListExpr` / `alarmTidArrCond` / `alarmDidTableJoinCond` 用文本包含判断替代 JSON 函数。

## 其余 5.7+/8.0 特性排查结论

- 元数据库 SQL 中已无 `JSON_*` 函数、无 `CAST(... AS JSON)`、无 `->` / `->>`。
  （`api/internal/service/inquiry/databend/*` 里的 `json_extract_path_text` 是 Databend 方言，与 MySQL 无关。）
- 无 CTE（`WITH`）、窗口函数（`ROW_NUMBER() OVER`）、`JSON_TABLE`、`utf8mb4_0900_ai_ci`、
  生成列、`INVISIBLE` 索引等 8.0 语法。
- `config/database.sql` 其余被索引的 varchar 列均 ≤128 字符（≤512 字节），不触发 767 前缀限制；
  `cv_pms_casbin_rule` 的 9 列联合唯一索引合计 3000 字节，单列 400 字节，
  在 5.6 默认 16K 页 + COMPACT 行格式下成立（**待验证**：若该库 `innodb_page_size` 为 8K 会报 1071）。
- `docker-compose*.yml` 仍是 `image: mysql:5.7.37`，属基础设施文件，未改动，需人工确认后再调整。

## 校验命令

```bash
D:/develop/go1.23/bin/go.exe build ./...
D:/develop/go1.23/bin/go.exe test ./api/internal/pkg/model/db/... ./api/internal/service/queryfilter/...

# 5.6 上重新建库（脚本硬编码 USE clickvisual，必须带 --database）
D:/develop/go1.23/bin/go.exe run main.go init -c config/default-dev.toml --database clickvisual_test

# 谓词语义自测（5.6 可直接跑，不碰业务表）
# SELECT CONCAT(',', REPLACE(REPLACE(REPLACE('[1,17]', '[', ''), ']', ''), ' ', ''), ',') LIKE '%,1,%';  -- 期望 1
# SELECT CONCAT(',', REPLACE(REPLACE(REPLACE('[1,17]', '[', ''), ']', ''), ' ', ''), ',') LIKE '%,7,%';   -- 期望 0
```
