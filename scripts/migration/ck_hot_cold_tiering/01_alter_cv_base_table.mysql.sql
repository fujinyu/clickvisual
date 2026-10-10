-- ClickVisual hot/cold tiering migration for cv_base_table.
-- Adds three nullable/default columns so that existing rows keep the legacy
-- single-layer behavior (storage_policy / cold_volume empty, hot_days = 0).
--
-- Read this file, verify against your MySQL version, and run it manually.
-- This script is NOT executed automatically by ClickVisual startup.
--
-- Compatible with MySQL 5.7+ / 8.0. MariaDB 10.2+ also accepts IF NOT EXISTS
-- on ADD COLUMN; on strict MySQL 5.7 remove IF NOT EXISTS if the parser
-- rejects it (MySQL 5.7 does not support IF NOT EXISTS for ADD COLUMN).
--
-- Data compatibility:
--   * storage_policy  empty  -> CREATE TABLE omits SETTINGS storage_policy,
--                                falls back to ClickHouse server default.
--   * cold_volume     empty  -> TTL stays single-layer (no TO VOLUME clause).
--   * hot_days        0      -> TTL keeps legacy `INTERVAL days DAY` form.
-- Existing tables therefore behave identically before and after the ALTER.

ALTER TABLE `cv_base_table`
    ADD COLUMN `storage_policy` VARCHAR(64) NOT NULL DEFAULT '' COMMENT 'ClickHouse storage policy name, empty = use server default',
    ADD COLUMN `cold_volume`    VARCHAR(64) NOT NULL DEFAULT '' COMMENT 'target volume name for TTL TO VOLUME, empty = single layer',
    ADD COLUMN `hot_days`       INT(11)     NOT NULL DEFAULT 0  COMMENT 'days kept on hot volume before moving to cold, 0 = single layer';
