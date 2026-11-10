# ClickHouse Kafka 入库延迟诊断 SQL 手册

> 来源：2026-10-28 生产事故排障（Kafka 中断约 2 小时恢复后，ClickHouse 侧入库长时间追不上）。
> 下面每一条 SQL 都是当轮实际执行过的，判读口径写的是**当轮被实测验证或被实测推翻**的口径，不是通用教条。
> 事故结论：吞吐被 `background_message_broker_schedule_pool_size`（默认 16）这个全局线程池卡住，
> 454 个 Kafka 消费者抢 16 个槽位，落库只有生产速率的 ~54–59%。调到 256 并重启后恢复正常。

## 0. 本文档的固定值（照抄可直接执行）

| 项 | 实际值 |
|---|---|
| ClickHouse 版本 / 规格 | 24.3.18 / 8 核 16G（镜像挂载配置里"适配 2 核 4G"的注释是过时的，别据此推断） |
| 元数据库（业务库名） | `prod_fc` |
| 目标表 / 流表 / 视图 | `prod_fc.ehub_express` / `prod_fc.ehub_express_stream` / `prod_fc.ehub_express_view` |
| Kafka broker / topic / group | `172.16.20.21:9094` / `prod_ehubExpressFcApp` / `prod_fc_ehub_express` |
| topic 分区数 / `kafka_num_consumers` | 4 / 4（一一对应） |
| 全实例 Kafka 消费者总数 | 454（`kafka_num_consumers` 之和） |
| topic 生产速率（事故当时） | 17,348,100 条 / 3600s ≈ **4,820 条/秒** |
| 修复后实测落库速率 | 2,622–3,056 行/秒（改池前）；改池后追上实时 |
| Kafka `retention.ms` | 1 小时（用户裁定不改）→ 积压超过 ~1h 的数据永久丢失 |

换库换表时只需替换 `prod_fc` / `ehub_express` 两个字面量；全文 SQL 都是只读查询，不改数据。

## 1. 5 分钟止血排查顺序

按这个顺序跑，每一步都用来**排除**一类原因，不要跳步去改参数：

1. §2 全局消费者快照 → 是"完全没有消费者在 poll"还是"都在 poll 但很慢"。
2. §3 `system.parts` 落库量对账（`level = 0`）→ ClickHouse 到底每秒写多少行，和上游速率比。
3. §4 新鲜度推进速率 → 落后在收敛还是在扩大。
4. §5 `system.metrics` 的 `*PoolSize` / `*PoolTask` 成对读数 → **哪个池满了**，这是本次事故的终点。
5. §6 写侧限流和 part 压力 → 排除 MergeTree 侧的闸门（本次全部为 0=未启用）。

如果 2 显示落库速率明显低于生产速率、4 显示落后持续扩大、而 5 里有一对 `Task == Size`，就是容量闸门问题；
此时 `SYSTEM STOP/START KAFKA CONSUMER` 和 `DETACH/ATTACH` 都**无效**，唯一动作是放大那个池并重启。

## 2. 消费者活着没有（`system.kafka_consumers`）

### 2.1 全局快照：总数、陈旧数、最差空闲

```sql
SELECT count() AS total,
       countIf(now() - last_poll_time > 30)  AS stale_over_30s,
       countIf(now() - last_poll_time > 300) AS stale_over_5min,
       max(now() - last_poll_time)           AS worst_idle
FROM system.kafka_consumers;
```

判读：`total` 是实例上所有 Kafka 消费者数（本次 454，等于各流表 `kafka_num_consumers` 之和，用来核对 §5 的池大小是否够）。
`worst_idle` 单位是秒；事故当时达到 1,793,183,251（≈ 57 年），这是**从未 poll 过的初始值**（`last_poll_time` 为 epoch 0），
属于"从没启动"，不是"停了很久"。

### 2.2 空转消费者有多少（判断"饿死"假设是否成立）

```sql
-- A. 空转消费者到底有多少，stale 是否≈空转数
--    若 partitions_held = 0 那行占大头且 stale_over_30s ≈ consumers，则"饿死"假设作废
SELECT length(assignments) AS partitions_held,
       count()             AS consumers,
       countIf(now() - last_poll_time > 30)  AS stale_over_30s,
       countIf(now() - last_poll_time > 300) AS stale_over_5min
FROM system.kafka_consumers
GROUP BY partitions_held
ORDER BY partitions_held;
```

本次实测：`partitions_held = 0` → 330 个消费者 / 288 个 stale30；`partitions_held = 4` → 124 个 / 54 个 stale30。

⚠️ **这个口径不可信，不要拿它做决策。** 单独看 `ehub_express_stream` 时它是 5 个消费者、topic 有 4 个分区、
订阅的 topic 名逐字正确，却 5 行全部 `length(assignments) = 0`；而全量分组里 124 个显示 4、330 个显示 0。
`assignments` 的语义不等于"我拥有的分区"（随采样时刻变化），由它推出来的"454 里 330 个空转（73%）"已撤回。
可以用它看分布，不能用它证明"某个消费者没分区"。

### 2.3 单表消费者明细（先看有没有异常，再谈别的）

```sql
SELECT `database`, `table`, consumer_id, last_poll_time,
       now() - last_poll_time AS idle,
       exceptions
FROM system.kafka_consumers
WHERE `table` LIKE 'ehub_express%';
```

两个必须知道的坑（24.3.18 实测）：

- `consumer_id` **恒为空串**，所以任何按 `consumer_id` 分组的逻辑（例如熔断 key）在同表多消费者时会塌成同一个 key。
- `exceptions` 是 Nested 数组，JDBC 客户端（DBeaver）渲染成 `com.clickhouse.jdbc.types.Array@...`，读不到文本。
  必须显式展开：

```sql
SELECT `table`, consumer_id,
       now() - last_poll_time AS idle,
       arrayStringConcat(`exceptions`.text, ' ||| ') AS exception_texts
FROM system.kafka_consumers
WHERE `table` LIKE 'ehub_express%';
```

⚠️ `exceptions` 非空是**唯一能证明消费者掉组**的证据；本次事故始终没取到（全为空），事后看是对的——
根因是容量闸门，消费者确实没报错。但顺序上它应该是第一个跑的检查，而且要注意：
一旦执行了 `DETACH`（界面上的「暂停入库」或手动重挂），`exceptions` 和 offset 信息会一起消失，**取证窗口被销毁**。
根因未定性前不要点任何暂停/重挂按钮。

### 2.4 全量列（首次接触该表时先做一次）

```sql
SELECT *
FROM system.kafka_consumers
WHERE `table` = 'ehub_express_stream'
ORDER BY consumer_id;
```

判读：本次这样取到 4 行完整 dump，确认了 `database` / `table` / `consumer_id` / `last_poll_time` /
`assignments` / `exceptions` 等列都存在，以及 `kafka_topic_list` 逐字正确。
`begin_offsets` 有值 + `end_offsets` 空数组 **不能**读成"一批都没提交"——`end_offsets` 语义未证，
落库量对账（§3）直接推翻了这条推理。

## 3. ClickHouse 到底落库多少行（`system.parts`，最有价值的一组）

### 3.1 近 10 分钟落库速率（正确写法）

```sql
SELECT count() AS parts,
       sum(rows) AS rows_total,
       max(modification_time) - min(modification_time) AS span_sec,
       sum(rows) / greatest(max(modification_time) - min(modification_time), 1) AS rows_per_sec
FROM system.parts
WHERE database = 'prod_fc' AND `table` = 'ehub_express'
  AND level = 0
  AND modification_time > now() - 600;
```

三个不可省的细节，每一条都是踩过坑加的：

1. **`level = 0` 必须写死。** `level > 0` 是合并产出的 part，它的行是已经被数过的 level-0 part 的副本。
   本次把合并量算进去，得出"5 个 part / 2,991,631 行 ≈ topic 速率，差 3.4% ⇒ 满速在灌"，
   据此错判"问题不在 ClickHouse"。剔除 `level = 1` 那个 1,789,003 行的 part 后，真实速率是 2,834 行/秒 = 生产速率的 59%。
2. **分母必须用实际时间跨度**（`greatest(max - min, 1)`），不能写死 600。part 数少时 `modification_time`
   只覆盖 384–436 秒，写死 600 会低估：同一个查询把 2,622 算成 1,883。
   保留 `span_sec` 输出列，就是为了能立刻发现 `span_sec` 远小于窗口。
3. `modification_time > now() - 600` 是"最近 10 分钟里改过"的 part，不是"这 10 分钟的量"；配合 2 才自洽。

### 3.2 简易版（只看 30 分钟窗口，用于快速确认量级）

```sql
SELECT count() AS parts,
       sum(rows) AS rows_30min,
       sum(rows) / 1800 AS rows_per_sec
FROM system.parts
WHERE database = 'prod_fc' AND `table` = 'ehub_express'
  AND level = 0
  AND modification_time > now() - 1800;
```

这条只在"part 多到时间跨度基本铺满窗口"时才准确。窗口内样本少就回到 §3.1。

### 3.3 对账口径（本手册最重要的一句话）

拿到 §3.1 的 `rows_per_sec` 后，唯一要做的是**和上游生产速率比**：

| 实测 rows_per_sec | 相对 topic 4,820 条/秒 | 定性 |
|---|---|---|
| ~2,622–2,834 | 54–59% | 吞吐被闸门卡住，落后每小时扩大 ~24–27 分钟 |
| ≥ 4,820 × 1.2 | >1.2× | 在追赶积压，落后会收敛 |
| ≈ 0 | — | 真的停了，走 §2.3 查 `exceptions` |

单看 ClickHouse 的任何一列（`lag_seconds`、`last_poll_time`、`end_offsets`）都给不出这个结论；
只有"落库行数 × 上游速率"对账能。上游速率从 Kafka UI 取（消息总量 / 保留时长）。

## 4. 新鲜度与积压（`_time_second_` 的正确用法）

### 4.1 最新日志时间 + 落后 + 5 分钟窗口行数

```sql
SELECT max(_time_second_) AS newest,
       now() - max(_time_second_) AS lag_seconds,
       countIf(_time_second_ >= now() - INTERVAL 5 MINUTE) AS rows_5min
FROM `prod_fc`.`ehub_express`;
```

事故当时：`newest = 17:37:38`、`lag_seconds = 3869`（64 分钟）、`rows_5min = 0`。

⚠️ **这条不是"入库停止"的证据。** `ehub_express_view` 的视图体是
`toDateTime(toInt64(time)) AS _time_second_`——取的是**日志行自带的时间**，不是到达时间。
积压追赶期灌进来的是旧日志，所以"最近 5 分钟窗口查 0 行"完全正常，`lag_seconds` 必然很大。
巡检/告警文案把这个现象写成"入库停止"是错的，会把运维推向"暂停入库/重挂"这种真正销毁取证窗口的操作。

### 4.2 追赶速率（该盯的量）

```sql
SELECT max(_time_second_) FROM prod_fc.ehub_express;
```

隔 5–10 分钟跑两次，比较 `max(_time_second_)` 前进了多少：

```
净追赶倍率 = (日志时间前进量) / (两次采样的墙钟间隔)
```

`> 1` 在收敛，`< 1` 在恶化。事故当时实测：18:46 → 18:55 的 9 分钟墙钟里，`max(_time_second_)`
从 17:37:38 走到 17:50:55，即 13.3 分钟日志时间 / 9 分钟墙钟 = **1.48×**（这个值是用含合并量的错数算的，
真实值低于 1×，见 §3.1 的教训；但"按分钟采样两次求斜率"这个方法是唯一可靠的新鲜度判据）。

不要用无排序取前 N 行来判断新鲜度：DBeaver 里取 `ehub_express_view` 前 500 行全是
`_time_second_ = 2026-10-21 18:34:41`——正好 7 天 TTL 边界，据此说"视图里比日志表多"是拿历史数据当新鲜度。

## 5. 找闸门：`system.metrics` 的池对（本次的终点）

### 5.1 一次性读完所有相关指标

```sql
SELECT metric, value
FROM system.metrics
WHERE metric ILIKE '%insert%' OR metric ILIKE '%delay%'
   OR metric ILIKE '%background%' OR metric ILIKE '%move%'
ORDER BY value DESC;
```

**读法（这是本次真正破局的方法）：成对出现的 `*PoolSize` 与 `*PoolTask`，`Task == Size` 就是那个池被打满。**
不需要任何推理，逐行读完即可。本次实测：

| metric | 值 | 读法 |
|---|---|---|
| `BackgroundMessageBrokerSchedulePoolTask` | 16 | `Task == Size` → **就是这个池满了** |
| `BackgroundMessageBrokerSchedulePoolSize` | 16 | 驱动 Kafka `streamToViews` 的线程池 |
| `KafkaBackgroundReads` | 16 | 在飞的后台读数量（gauge，不是上限） |
| `BackgroundSchedulePoolTask` | 0 | 这个池从没排过队 → 前面几轮调它全调错了 |
| `DelayedInserts` | 0 | 写侧没有被限流 |
| `PartsBeingMerged` | 0 | 没有合并压力 |

### 5.2 单独盯 Kafka 在飞读数量

```sql
SELECT now() AS t, value AS kafka_reads_in_flight
FROM system.metrics
WHERE metric = 'KafkaBackgroundReads';
```

多次采样（本次 19:23:55 等若干次）**每次都恰好是 16** → 454 个消费者抢 16 个槽，每个日志库约 3.5% 占空比。
注意：`KafkaBackgroundReads` 是**实时在飞数量的仪表读数**，不是硬上限；它恒等于 16 只是因为池大小是 16。
把它当"不可配置的硬编码闸门"是本次第 5 次误判（当时我编造了 `StorageKafka.cpp` 里的
`MAXIMUM_BACKGROUND_READS` 常量名，不存在）。真正的参数是 §5.3。

### 5.3 线程池配置项现值

```sql
SELECT name, value
FROM system.server_settings
WHERE name IN ('background_pool_size', 'background_schedule_pool_size', 'max_thread_pool_size');
```

要一并确认根因，必须把这条改成看全量（`system.server_settings` 里 `background_message_broker_schedule_pool_size`
不在上面三个里，正是漏掉它导致三轮调参打在错的池上）：

```sql
SELECT name, value, changed
FROM system.server_settings
WHERE name ILIKE '%pool%' OR name ILIKE '%thread%'
ORDER BY name;
```

判读：任何 `name ILIKE '%pool_size%'` 的行都要和 `system.metrics` 里的同名 `*PoolTask` 配对看。

## 6. 排除写侧限流与 part 压力

### 6.1 当前会话/用户的插入限流阈值

```sql
SELECT name, value
FROM system.settings
WHERE name IN ('parts_to_delay_insert', 'parts_to_throw_insert', 'max_parts_in_total');
```

判读：本项目三值全为 **0 = 该限制已关闭**，所以不存在"part 太多导致 INSERT 被延迟/抛错"。
（默认值分别是 250 / 400 / 10000，非 0 才要当真。）

### 6.2 每张表的活跃 part 数（找谁在逼近限流）

```sql
SELECT database, `table`, count() AS active_parts
FROM system.parts
WHERE active
GROUP BY database, `table`
ORDER BY active_parts DESC
LIMIT 15;
```

### 6.3 按分区看 part 数（`parts_to_delay_insert` 是按分区算的）

```sql
SELECT database, `table`, partition_id, count() AS parts_in_partition
FROM system.parts
WHERE active
GROUP BY database, `table`, partition_id
ORDER BY parts_in_partition DESC
LIMIT 15;
```

判读：本次最高是 `lbs_location` 全表 878 个 part，但按分区看只有 16 个，远低于 250；
配合 §6.1 阈值为 0，写侧完全排除。

## 7. 探底可配置项（改参数前先确认能不能改、改了生效没有）

```sql
-- 服务级：有没有 kafka 相关项（本次结果为空——kafka 参数不在 server_settings 里，而在 users.xml 的 profile 里）
SELECT name, value, changed, description
FROM system.server_settings
WHERE name ILIKE '%kafka%';

-- 用户级：kafka 可调参数清单
SELECT name, value
FROM system.settings
WHERE name ILIKE '%kafka%';

-- 改完后确认实际生效值
SELECT name, value, changed FROM system.settings WHERE name = 'kafka_max_wait_ms';
```

`kafka_max_wait_ms` 的实测结论：**不是杠杆。** 5000 → 500 后落库速率
2,834 → 3,056 → 2,659 → 2,622 行/秒，±7% 属于噪声，没有数量级改善。
这条改动留在 `users.xml` 里没回退（用户裁定）。

## 8. 修复动作与生效方式（唯一有效的一条）

`config.xml`（**服务级参数，改完必须重启进程才生效**，`system.server_settings` 里 `changed` 会显示是否被显式设置）：

```xml
<clickhouse>
    <background_message_broker_schedule_pool_size>256</background_message_broker_schedule_pool_size>
</clickhouse>
```

取值口径：`background_message_broker_schedule_pool_size` 默认 **16**，应 **≥ 实例上 Kafka 消费者总数**
（本次 454；454 全铺开不现实，256 已足够把占空比从 3.5% 提上去，改完恢复正常）。
更省事的部署约定：**每 8 个消费者配 1 个槽位**，即 `池大小 ≥ 消费者总数 / 8`，并在 §5.1 里对
`Task == Size` 设告警——这应当作为 ClickVisual 自愈巡检的部署前置条件，因为这类失败用
`SYSTEM STOP/START KAFKA CONSUMER` 和 `DETACH → ATTACH` 都修不了。

顺带确认（本次取证用）：

```sql
SELECT name, engine, create_table_query
FROM system.tables
WHERE database = 'prod_fc'
  AND name IN ('ehub_express', 'ehub_express_stream', 'ehub_express_view');
```

本次取到的 DDL 排除了三类结构性嫌疑：视图确实是 `MaterializedView ... TO prod_fc.ehub_express
FROM prod_fc.ehub_express_stream WHERE 1 = 1`（没被 DROP/DETACH、`TO` 没挂错）；
流表参数 `kafka_group_name='prod_fc_ehub_express'`、`kafka_broker_list='172.16.20.21:9094'`、
`kafka_format='JSONEachRow'`、`kafka_num_consumers=4`、`kafka_skip_broken_messages=10`
（**坏行静默丢弃，每批最多 10 条**，排查"数据少了"时记得这条）；
目标表 `PARTITION BY toYYYYMMDD(_time_second_)`、TTL 7 天、`storage_policy='hot_to_cold'`。

## 9. 本次排除掉的假设（下次别再花时间重跑）

| 假设 | 推翻它的证据 |
|---|---|
| MV 被删 / `TO` 挂错 | §8 DDL：视图存在且指向正确 |
| topic 名不匹配 | §2.4 dump：逐字等于 `prod_ehubExpressFcApp` |
| 消费者从没启动 | §2.1：`never_polled` 只有 2 个 |
| 单分区被别的成员拿走 | topic 实测 4 个分区，`kafka_num_consumers=4` |
| offset 没提交（写入侧失败） | §3：`level=0` 落库量在满量增长 |
| `background_pool_size` 不够 | 16 → 200 重启后 `stale_over_30s` 从 379 只降到 342 |
| `background_schedule_pool_size` 超订 | `BackgroundSchedulePoolTask = 0`（该池从未排队），16 → 200 → 1024 三轮全错 |
| `kafka_max_wait_ms` | §7：±7% 噪声 |
| MergeTree 插入限流 | §6.1 三值全 0；§6.3 单分区最多 16 part |
| 机器容量（CPU/IO） | idle 0.92、IOWait 0、`PartsBeingMerged` 0 |

## 10. 一页速查：现象 → 第一动作

| 现象 | 先跑 | 别做 |
|---|---|---|
| 按时间查不到新日志，但不知道是不是停了 | §3.1 落库速率 + §4.1 追赶斜率 | 别点「暂停入库」/重挂（销毁 `exceptions` 取证窗口） |
| 落库速率明显低于上游速率 | §5.1 找 `Task == Size` 的池 | 别调 `background_pool_size` / `background_schedule_pool_size` |
| `exceptions` 有文本 | 按文本定性（认证/ topic 不存在/ 解析失败） | 别看 `assignments` 判断有没有分区 |
| `stale_over_30s` 很大但没有掉组 | §3.3 对账，是容量问题不是故障 | 别写"入库停止"的告警文案 |
| Kafka 积压超过 `retention.ms` | 立刻算丢了多少（本次 1h ⇒ 17:50 前的日志永久没了） | 别指望重挂能追回来 |
