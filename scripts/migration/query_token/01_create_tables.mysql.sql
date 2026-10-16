-- ClickVisual query token migration.
-- Creates cv_query_token / cv_query_token_grant / cv_query_token_audit for
-- production databases that predate the query-token feature.
--
-- Read this file, verify against your MySQL version, and run it manually.
-- This script is NOT executed automatically by ClickVisual startup; the code
-- keeps an AutoMigrate fallback in api/internal/service/querytoken/service.go,
-- running the DDL here first just avoids surprise schema writes at boot time.
--
-- Compatible with MySQL 5.7+ / 8.0. Uses CREATE TABLE IF NOT EXISTS so the
-- script is safe to re-run.
--
-- Data compatibility:
--   * All three tables are new; no existing rows are touched.
--   * Column set matches api/internal/pkg/model/db/query_token.go exactly,
--     including index names used by GORM AutoMigrate.

CREATE TABLE IF NOT EXISTS `cv_query_token` (
    `id`           bigint       NOT NULL AUTO_INCREMENT COMMENT '自增id',
    `ctime`        bigint                DEFAULT NULL COMMENT '创建时间',
    `utime`        bigint                DEFAULT NULL COMMENT '更新时间',
    `dtime`        bigint                DEFAULT 0 COMMENT '删除时间',
    `name`         varchar(128) NOT NULL COMMENT '令牌名称',
    `token_hash`   varchar(64)  NOT NULL COMMENT '令牌 SHA-256 摘要',
    `token_prefix` varchar(32)  NOT NULL COMMENT '令牌前缀,用于列表展示',
    `status`       tinyint(1)   NOT NULL DEFAULT 1 COMMENT '1 启用 2 禁用',
    `expire_at`    bigint       NOT NULL DEFAULT 0 COMMENT '过期时间戳,0 表示不过期',
    `last_used_at` bigint       NOT NULL DEFAULT 0 COMMENT '最近一次使用时间戳',
    `created_by`   int(11)      NOT NULL DEFAULT 0 COMMENT '创建者 uid',
    `desc`         varchar(255)          DEFAULT NULL COMMENT '描述',
    PRIMARY KEY (`id`),
    UNIQUE KEY `idx_query_token_hash` (`token_hash`),
    KEY `idx_query_token_prefix` (`token_prefix`),
    KEY `idx_query_token_status` (`status`),
    KEY `idx_query_token_created_by` (`created_by`)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4 COLLATE = utf8mb4_general_ci;

CREATE TABLE IF NOT EXISTS `cv_query_token_grant` (
    `id`       bigint  NOT NULL AUTO_INCREMENT COMMENT '自增id',
    `ctime`    bigint           DEFAULT NULL COMMENT '创建时间',
    `utime`    bigint           DEFAULT NULL COMMENT '更新时间',
    `dtime`    bigint           DEFAULT 0 COMMENT '删除时间',
    `token_id` int(11) NOT NULL COMMENT 'cv_query_token.id',
    `tid`      int(11) NOT NULL COMMENT '被授权的 cv_base_table.id',
    PRIMARY KEY (`id`),
    UNIQUE KEY `idx_query_token_grant` (`token_id`, `tid`),
    KEY `idx_query_token_grant_tid` (`tid`)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4 COLLATE = utf8mb4_general_ci;

CREATE TABLE IF NOT EXISTS `cv_query_token_audit` (
    `id`            bigint           NOT NULL AUTO_INCREMENT COMMENT '自增id',
    `ctime`         bigint                    DEFAULT NULL COMMENT '创建时间',
    `utime`         bigint                    DEFAULT NULL COMMENT '更新时间',
    `dtime`         bigint                    DEFAULT 0 COMMENT '删除时间',
    `token_id`      int(11)          NOT NULL COMMENT 'cv_query_token.id',
    `token_name`    varchar(128)     NOT NULL COMMENT '令牌名称快照',
    `tid`           int(11)          NOT NULL COMMENT 'cv_base_table.id',
    `database_name` varchar(255)     NOT NULL COMMENT '数据库名快照',
    `table_name`    varchar(255)     NOT NULL COMMENT '表名快照',
    `query_json`    text                      COMMENT '查询条件 JSON',
    `st`            bigint           NOT NULL DEFAULT 0 COMMENT '查询起始时间戳',
    `et`            bigint           NOT NULL DEFAULT 0 COMMENT '查询结束时间戳',
    `page`          int(11) unsigned NOT NULL DEFAULT 0 COMMENT '页码',
    `page_size`     int(11) unsigned NOT NULL DEFAULT 0 COMMENT '每页大小',
    `result_count`  bigint unsigned  NOT NULL DEFAULT 0 COMMENT '命中行数',
    `cost_ms`       bigint           NOT NULL DEFAULT 0 COMMENT '查询耗时 ms',
    `status`        varchar(32)      NOT NULL COMMENT 'success / failed',
    `error_message` text                      COMMENT '失败原因',
    `client_ip`     varchar(64)               DEFAULT NULL COMMENT '来源 IP',
    `user_agent`    varchar(255)              DEFAULT NULL COMMENT '来源 UA',
    PRIMARY KEY (`id`),
    KEY `idx_query_token_audit_token` (`token_id`),
    KEY `idx_query_token_audit_tid` (`tid`),
    KEY `idx_query_token_audit_status` (`status`)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4 COLLATE = utf8mb4_general_ci;
