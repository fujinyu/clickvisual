-- ClickVisual "pause log store ingestion" migration for cv_base_table.
--
-- Adds one flag column recording whether the Kafka stream table of a log store
-- is currently DETACHed in ClickHouse (ingestion paused, messages pile up in the
-- Kafka topic and are re-ingested on resume).
--
-- Read this file, verify against your MySQL version, and run it manually.
-- This script is NOT executed automatically by ClickVisual startup.
--
-- Deployment order matters: the Go entity selects this column, so the new
-- binary must not start before the ALTER has been applied, otherwise every
-- base_table query fails with "Unknown column 'suspend_status'".
--
-- Compatible with MySQL 5.6 / 5.7 / 8.0 (plain tinyint, no JSON, no index).
-- MySQL 5.6 and 5.7 do not support IF NOT EXISTS on ADD COLUMN, so the
-- statement is not idempotent: run it once. Use 02_verify.sql to check whether
-- it has already been applied.
--
-- Data compatibility:
--   * DEFAULT 0 means every existing row is "not suspended" after the ALTER,
--     so no backfill is required and behavior is unchanged for all log stores.
--   * 1 is only ever written by the suspend API, and always together with a
--     successful DETACH TABLE on the ClickHouse side.

ALTER TABLE `cv_base_table`
    ADD COLUMN `suspend_status` TINYINT(1) NOT NULL DEFAULT 0 COMMENT '0 = stream table attached, ingestion running; 1 = stream table detached, ingestion paused';
