# log_id / time_ns 精度迁移（Float64 → Int64）运行手册

## 1. 背景与目标

日志按 `_time_nanosecond_` 排序，采集端在同一 `time_ns` 内可能产生多条记录，
单键排序不稳定导致乱序、翻页重复/丢行。采集端新增 `log_id`（单调递增整数）作为
同一时刻内的次序依据。

代码侧已完成（本次改动，无需再动）：

- 查询排序增加次级键：`ORDER BY _time_nanosecond_ {dir}, log_id {dir}`，
  表无 `log_id` 列时自动降级为单键（老表查询不报错）。
- 新建表：`log_id` / `time_ns` 大整数自动推断为 `Int64` 物化；
  `_time_nanosecond_` 由 `JSONExtractInt(time_ns)` 精确提取。

**本手册只负责“存量表”的精度迁移**：把已存在的 `Float64` 列改为 `Int64`，
并重建物化视图（MV），使迁移后写入的新数据不再丢精度。

> 根因：19 位大整数超出 Float64 精确范围（2^53 ≈ 9e15），存储时末尾被舍入，
> 例如 `log_id ...379xxx → ...379000`、`time_ns ...732700 → ...732672`。

## 2. 影响范围（连生产库 172.16.128.162 实测，ClickHouse 24.3.18.7，单机无集群）

| 数据库 | Float64 的 log_id/time_ns 列数 | 说明 |
| --- | --- | --- |
| test_fc | 231 | 先迁移（测试） |
| test_k8s | 264 | 先迁移（测试） |
| prod_fc | 295 | 后迁移（生产） |
| prod_k8s | 303 | 后迁移（生产） |
| prod_56linked / test_56linked | 0 | 无 log_id 列，无需迁移（代码已降级单键） |

按引擎分布（Float64 的 log_id/time_ns）：

- MergeTree（目标表 `<t>`）：log_id 343、time_ns 200
- Kafka（流表 `<t>_stream`，即 JSONEachRow 模式）：log_id 39、time_ns 42
- MaterializedView（`<t>_view`）：log_id 270、time_ns 199（MV 用 DROP+CREATE 重建，不做 ALTER）

两种建表模式：

- **JSONAsString（多数）**：流表只有 `_log String`；log_id/time_ns 仅在目标表，
  MV 用 `JSONExtractFloat(_log, 'xxx')` 提取。
- **JSONEachRow（约 40 张）**：流表本身含 log_id/time_ns 列，MV 读裸列。

### 必须排除 / 需人工确认的表

- `prod_fc.elog_wms_startup_fc_poc2`（及其 `_view`）：`log_id` 是 `String` / `Nullable(String)`，
  **不是数值**，脚本已按类型过滤排除，切勿转 Int64。
- 少数表 log_id 以自定义表达式列存储，例如
  `prod_k8s.forest_alarm_prod_view` 中列名直接是
  `` `toFloat64(JSONExtractString(_log, 'log_id'))` ``。
  这类表 **没有** 名为 `log_id` 的普通列，`01_inspect.sql` 会单独列出，
  需人工确认是否/如何迁移（本套脚本只安全处理其 `time_ns`，不动这类怪异 log_id 列）。

## 3. 迁移原理（每张表三步）

1. **改列类型（ClickHouse ALTER）**：目标表 + 流表的 `log_id`/`time_ns`
   由 `Float64`/`Nullable(Float64)` 改为 `Int64`/`Nullable(Int64)`。
2. **重建 MV（DROP + CREATE）**：把 MV 里 `JSONExtractFloat(..., 'log_id'|'time_ns')`
   改为 `JSONExtractInt(...)`，并把 MV 列声明中的 `log_id`/`time_ns` 由 `Float64` 改为 `Int64`。
   - 推荐路径 A（最稳）：改完 `any_json`（第 4 步）后，通过 ClickVisual 触发表更新，
     由 `updateSwitcher` 用最新代码重建 MV（自动兼容父子字段、索引等所有情况）。
   - 备选路径 B（自包含）：用 `03_gen_rebuild_mv.sql` 生成的 DROP+CREATE 语句执行；
     该生成器只改写标准写法的 log_id/time_ns，怪异写法保持原样。
   - 路径 C（单表、界面）：日志页右键日志库选“重建采集结构”，
     后端 `POST /api/v1/tables/:id/rebuild` 自动按序完成上述三步
     （修正 any_json → ALTER 目标表/流表列 → 经 updateSwitcher 重建全部 MV），
     失败时回滚 any_json。适合逐表/小批量迁移；大批量仍建议路径 A/B。
3. **更新元数据（MySQL any_json）**：把建表映射里 log_id/time_ns 的 `Float64` 改为 `Int64`，
   否则日后在 ClickVisual 里编辑该表索引会按旧类型重建 MV，精度又退化。

> 为什么 MV 必须重建：MV 的列声明与 SELECT 决定了写入目标表的值类型。
> 只 ALTER 目标表列而不重建 MV，MV 仍产出 Float64（提取阶段已丢精度），再被强转 Int64，
> 精度并不会恢复。

## 4. 执行顺序（务必 test → prod，维护窗口内进行）

> ALTER 列类型会触发数据 mutation（重写历史数据），大表耗时/耗 IO；
> DROP→CREATE MV 期间该表 Kafka 消费短暂暂停。**Kafka 会按消费组 offset 保留消息，
> 重建后自动续消费，不丢数据**，仅 ingestion 有延迟。请逐表或小批量执行以缩短单表停顿。

单表推荐顺序（对两种模式都安全）：

```
1) DROP MV            <t>_view          -- 先停消费/解绑
2) ALTER 流表列        <t>_stream        -- 仅 JSONEachRow 有 log_id/time_ns 列，JSONAsString 自动跳过
3) ALTER 目标表列      <t>
4) CREATE MV          <t>_view          -- 用 Int64 版本重建
5)（全部表完成后）更新 MySQL any_json
```

批次建议：`test_k8s` → `test_fc` 全量验证通过后，再 `prod_k8s` → `prod_fc` 按库分批。

## 5. 脚本清单与用法

所有 `*.gen.sql` 都是**只读的“生成器”**：在 ClickHouse 上执行后**输出**真正的 DDL 文本，
不直接改库。请先跑生成器、把输出保存为 `.sql`、**人工审阅**后再执行。

| 文件 | 作用 | 运行位置 |
| --- | --- | --- |
| `01_inspect.sql` | 只读盘点：受影响表/列、按库按引擎分类、列出怪异写法与 String 排除项 | ClickHouse |
| `02_gen_alter_columns.sql` | 生成器：输出所有 `ALTER TABLE ... MODIFY COLUMN ... Int64` | ClickHouse |
| `03_gen_rebuild_mv.sql` | 生成器：输出所有 MV 的 `DROP + CREATE`（Int64 版） | ClickHouse |
| `04_update_anyjson.mysql.sql` | 更新 ClickVisual 元数据 `base_table.any_json`（含先查后改） | MySQL |
| `05_verify.sql` | 迁移后校验：列类型、MV 提取函数、排序稳定性抽样 | ClickHouse |

用 HTTP 接口跑生成器并保存输出（示例，Windows PowerShell）：

```powershell
$auth = "admin:<password>"; $url = "http://172.16.128.162:8123/"
# 生成 ALTER 脚本（只读）
curl.exe -sS --user $auth $url --data-binary (Get-Content -Raw .\02_gen_alter_columns.sql) |
  Set-Content -Encoding utf8 .\out_alter_columns.sql
# 生成 MV 重建脚本（只读）
curl.exe -sS --user $auth $url --data-binary (Get-Content -Raw .\03_gen_rebuild_mv.sql) |
  Set-Content -Encoding utf8 .\out_rebuild_mv.sql
```

或用 `clickhouse-client`：`clickhouse-client --user admin --password '<password>' < 02_gen_alter_columns.sql > out_alter_columns.sql`。

> 生成器默认覆盖全部 4 个库。若要**只针对某个库/某批表**，在每个生成器顶部的
> `database IN (...)` 白名单里增删即可（见各文件注释）。

## 6. 历史数据精度说明（务必向业务同步）

`Float64` 时期写入的历史值**已经丢精度**，`MODIFY COLUMN` 只是把这些“已舍入的浮点”
转成最接近的整数（如 `...379000`），**无法还原真实末位**。

- 迁移**之后**经 MV 写入的新数据：`log_id`/`time_ns`/`_time_nanosecond_` 精确。
- 迁移**之前**的历史数据：精度不可恢复；同一 `_time_nanosecond_` 内的老记录，
  其次级排序键 `log_id` 仍可能因历史丢精度而相同（排序稳定但不保证与真实先后完全一致）。

## 7. 回滚

- 列类型：Int64 → Float64 可再次 `MODIFY COLUMN` 回退（数据为整数值，回退无损）。
- MV：`03_gen_rebuild_mv.sql` 生成的是 `DROP + CREATE`；执行前请先用
  `SELECT create_table_query FROM system.tables WHERE ...` 导出**当前** MV 原始 DDL 备份，
  回滚时用备份 DDL 重新 CREATE 即可。
- any_json：`04` 执行前先备份相关行（脚本内含备份查询）。

## 8. 验证（迁移后）

见 `05_verify.sql`：确认列类型已为 Int64、MV 已用 `JSONExtractInt`、
并抽取同一 `_time_nanosecond_` 的多条记录确认按 `log_id` 稳定有序、翻页不重复。
