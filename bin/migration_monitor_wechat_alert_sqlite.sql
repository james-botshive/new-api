-- 资源监控·微信告警 功能集成脚本 (SQLite)
-- Resource Monitor WeChat Alert integration script (SQLite)
--
-- 说明：
--   1. 本功能不新增任何表或字段（经与上游 model/ 逐文件比对验证），
--      应用启动时 GORM AutoMigrate 即可，无需执行任何 DDL。
--   2. 本脚本将功能的默认配置项写入 options 表，幂等（可重复执行）。
--   3. 末尾可选段落用于让旧库与上游最新 schema 前向兼容。
--      注意：SQLite 的 ALTER TABLE ADD COLUMN 不支持 IF NOT EXISTS，
--      该段只能执行一次，重复执行会报 "duplicate column name"。
--
-- Notes:
--   1. This feature adds NO tables/columns (verified by diffing model/ against upstream);
--      GORM AutoMigrate at startup covers everything, no DDL required.
--   2. This script inserts the feature's default settings rows into options. Idempotent.
--   3. The optional section at the end adds forward-compat columns from upstream's latest schema.
--      NOTE: SQLite has no ADD COLUMN IF NOT EXISTS — that section must run only once;
--      re-running it fails with "duplicate column name".

-- ============ 功能默认配置 (feature default settings) ============
INSERT OR IGNORE INTO options (`key`, `value`) VALUES
    ('monitor_setting.channel_failure_monitor_enabled', 'false'),
    ('monitor_setting.channel_failure_threshold', '5'),
    ('monitor_setting.channel_failure_cooldown_minutes', '10'),
    ('monitor_setting.email_notify_enabled', 'false'),
    ('monitor_setting.email_recipients', ''),
    ('monitor_setting.wechat_bot_base_url', ''),
    ('monitor_setting.wechat_bot_token', ''),
    ('monitor_setting.wechat_bot_app_id', ''),
    ('monitor_setting.wechat_bot_client_version', ''),
    ('monitor_setting.wechat_bot_channel_version', '1.0.0'),
    ('monitor_setting.wechat_bot_agent', 'NewAPI/1.0'),
    ('monitor_setting.wechat_bot_to_user_id', '');

-- ============ 可选：与上游最新 schema 前向兼容（只能执行一次） ============
-- 上游新增了 midjourneys 的两个计费字段（默认 0，不影响本功能）。
-- 执行后旧库可直接运行合并了上游代码的版本。
-- Optional: forward-compat with upstream's latest schema (run only once).
-- Upstream added two billing columns on midjourneys (default 0, unrelated to this feature).
-- After running this, the database can also run builds that merge upstream code.
ALTER TABLE midjourneys ADD COLUMN token_id integer DEFAULT 0;
ALTER TABLE midjourneys ADD COLUMN billing_channel_id integer DEFAULT 0;
