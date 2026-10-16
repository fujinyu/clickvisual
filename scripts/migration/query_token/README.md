# query_token 迁移

为早于查询令牌功能上线的存量库补齐三张表：`cv_query_token` / `cv_query_token_grant` / `cv_query_token_audit`。

## 影响范围

- 全部为**新建表**，不改动任何已有表结构与数据。
- 表结构、索引名与 [api/internal/pkg/model/db/query_token.go](../../../api/internal/pkg/model/db/query_token.go) 一致，
  与 [api/internal/service/querytoken/service.go](../../../api/internal/service/querytoken/service.go) 中的 `AutoMigrate` 兼容。

## 执行顺序

```
1) 01_create_tables.mysql.sql   -- 生产 MySQL 上执行
2) 02_verify.sql                -- 校验表和列已创建
```

`CREATE TABLE IF NOT EXISTS` 幂等，脚本可重复执行。

## 与代码 AutoMigrate 的关系

代码里保留了 `AutoMigrate` 作为兜底：先手动跑本脚本可以让 schema 变更走 DBA 审核流程，
避免服务启动时首次触发 DDL；不跑本脚本、直接升级代码也能通过 `AutoMigrate` 建表。

## 完整的生产库补齐顺序

若同时需要补齐所有代码新增的表/列（对比 `pasted-text.txt` 里生产库现状的完整差异），按以下顺序执行：

```
1) ../database.sql                                     -- 报表/加速/查询过滤/AI 设置 (6 张表)
2) ../ck_hot_cold_tiering/01_alter_cv_base_table.mysql.sql -- cv_base_table 冷热分层 3 列
3) ./01_create_tables.mysql.sql                        -- 查询令牌 3 张表
4) 分别执行各目录下的 verify 脚本做校验
```

## 未处理项（有意保留）

生产库中 `cv_pms_custom_role` / `cv_pms_default_role` / `cv_pms_role_detail` 存在
`sub_resource_arr` / `act_arr` 两列，代码已改用 JSON 类型的 `sub_resources` / `acts`，
这两列属于历史废弃字段。**不在本次迁移中删除**，如需清理请单独立项。
