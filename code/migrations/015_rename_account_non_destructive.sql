-- 015_rename_account_non_destructive.sql
--
-- Sc_us -> sc_us_2（联营），sc_us -> sc_us_1（自营）。
-- 账号改名只能更新不会撞主键的行；冲突行原地保留，绝不 DELETE。
-- 冲突按表记录，worker 启动时只禁用引用该表的 endpoint，HTTP 和其他接口继续运行。

CREATE TABLE IF NOT EXISTS migration_account_conflicts (
    table_name      VARCHAR(64) NOT NULL,
    old_account_id  VARCHAR(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
    new_account_id  VARCHAR(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
    conflict_count  INT NOT NULL,
    detected_at     DATETIME NOT NULL,
    PRIMARY KEY (table_name, old_account_id, new_account_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- sync_tasks 的主键只有自增 id，不会因 account_id 改名冲突。
UPDATE sync_tasks
SET account_id = 'sc_us_2'
WHERE account_id COLLATE utf8mb4_bin = 'Sc_us';
UPDATE sync_tasks
SET account_id = 'sc_us_1'
WHERE account_id COLLATE utf8mb4_bin = 'sc_us';

-- 每张表先记录冲突，再只改没有对应目标主键的旧行。
INSERT INTO migration_account_conflicts (table_name, old_account_id, new_account_id, conflict_count, detected_at)
SELECT 'ls_stores', 'Sc_us', 'sc_us_2', COUNT(*), NOW()
FROM ls_stores old JOIN ls_stores target
  ON target.account_id COLLATE utf8mb4_bin = 'sc_us_2' AND old.sid <=> target.sid
WHERE old.account_id COLLATE utf8mb4_bin = 'Sc_us'
HAVING COUNT(*) > 0
ON DUPLICATE KEY UPDATE conflict_count = VALUES(conflict_count), detected_at = VALUES(detected_at);
UPDATE ls_stores old LEFT JOIN ls_stores target
  ON target.account_id COLLATE utf8mb4_bin = 'sc_us_2' AND old.sid <=> target.sid
SET old.account_id = 'sc_us_2'
WHERE old.account_id COLLATE utf8mb4_bin = 'Sc_us' AND target.account_id IS NULL;

INSERT INTO migration_account_conflicts (table_name, old_account_id, new_account_id, conflict_count, detected_at)
SELECT 'ls_stores', 'sc_us', 'sc_us_1', COUNT(*), NOW()
FROM ls_stores old JOIN ls_stores target
  ON target.account_id COLLATE utf8mb4_bin = 'sc_us_1' AND old.sid <=> target.sid
WHERE old.account_id COLLATE utf8mb4_bin = 'sc_us'
HAVING COUNT(*) > 0
ON DUPLICATE KEY UPDATE conflict_count = VALUES(conflict_count), detected_at = VALUES(detected_at);
UPDATE ls_stores old LEFT JOIN ls_stores target
  ON target.account_id COLLATE utf8mb4_bin = 'sc_us_1' AND old.sid <=> target.sid
SET old.account_id = 'sc_us_1'
WHERE old.account_id COLLATE utf8mb4_bin = 'sc_us' AND target.account_id IS NULL;

INSERT INTO migration_account_conflicts (table_name, old_account_id, new_account_id, conflict_count, detected_at)
SELECT 'ls_sales_orders', 'Sc_us', 'sc_us_2', COUNT(*), NOW()
FROM ls_sales_orders old JOIN ls_sales_orders target
  ON target.account_id COLLATE utf8mb4_bin = 'sc_us_2' AND old.amazon_order_id <=> target.amazon_order_id
WHERE old.account_id COLLATE utf8mb4_bin = 'Sc_us'
HAVING COUNT(*) > 0
ON DUPLICATE KEY UPDATE conflict_count = VALUES(conflict_count), detected_at = VALUES(detected_at);
UPDATE ls_sales_orders old LEFT JOIN ls_sales_orders target
  ON target.account_id COLLATE utf8mb4_bin = 'sc_us_2' AND old.amazon_order_id <=> target.amazon_order_id
SET old.account_id = 'sc_us_2'
WHERE old.account_id COLLATE utf8mb4_bin = 'Sc_us' AND target.account_id IS NULL;

INSERT INTO migration_account_conflicts (table_name, old_account_id, new_account_id, conflict_count, detected_at)
SELECT 'ls_sales_orders', 'sc_us', 'sc_us_1', COUNT(*), NOW()
FROM ls_sales_orders old JOIN ls_sales_orders target
  ON target.account_id COLLATE utf8mb4_bin = 'sc_us_1' AND old.amazon_order_id <=> target.amazon_order_id
WHERE old.account_id COLLATE utf8mb4_bin = 'sc_us'
HAVING COUNT(*) > 0
ON DUPLICATE KEY UPDATE conflict_count = VALUES(conflict_count), detected_at = VALUES(detected_at);
UPDATE ls_sales_orders old LEFT JOIN ls_sales_orders target
  ON target.account_id COLLATE utf8mb4_bin = 'sc_us_1' AND old.amazon_order_id <=> target.amazon_order_id
SET old.account_id = 'sc_us_1'
WHERE old.account_id COLLATE utf8mb4_bin = 'sc_us' AND target.account_id IS NULL;

INSERT INTO migration_account_conflicts (table_name, old_account_id, new_account_id, conflict_count, detected_at)
SELECT 'ls_inventory', 'Sc_us', 'sc_us_2', COUNT(*), NOW()
FROM ls_inventory old JOIN ls_inventory target
  ON target.account_id COLLATE utf8mb4_bin = 'sc_us_2' AND old.sid <=> target.sid AND old.fnsku <=> target.fnsku
WHERE old.account_id COLLATE utf8mb4_bin = 'Sc_us'
HAVING COUNT(*) > 0
ON DUPLICATE KEY UPDATE conflict_count = VALUES(conflict_count), detected_at = VALUES(detected_at);
UPDATE ls_inventory old LEFT JOIN ls_inventory target
  ON target.account_id COLLATE utf8mb4_bin = 'sc_us_2' AND old.sid <=> target.sid AND old.fnsku <=> target.fnsku
SET old.account_id = 'sc_us_2'
WHERE old.account_id COLLATE utf8mb4_bin = 'Sc_us' AND target.account_id IS NULL;

INSERT INTO migration_account_conflicts (table_name, old_account_id, new_account_id, conflict_count, detected_at)
SELECT 'ls_inventory', 'sc_us', 'sc_us_1', COUNT(*), NOW()
FROM ls_inventory old JOIN ls_inventory target
  ON target.account_id COLLATE utf8mb4_bin = 'sc_us_1' AND old.sid <=> target.sid AND old.fnsku <=> target.fnsku
WHERE old.account_id COLLATE utf8mb4_bin = 'sc_us'
HAVING COUNT(*) > 0
ON DUPLICATE KEY UPDATE conflict_count = VALUES(conflict_count), detected_at = VALUES(detected_at);
UPDATE ls_inventory old LEFT JOIN ls_inventory target
  ON target.account_id COLLATE utf8mb4_bin = 'sc_us_1' AND old.sid <=> target.sid AND old.fnsku <=> target.fnsku
SET old.account_id = 'sc_us_1'
WHERE old.account_id COLLATE utf8mb4_bin = 'sc_us' AND target.account_id IS NULL;

-- 其余表按相同规则处理；这些表的主键在 010/011 迁移后已固定。
INSERT INTO migration_account_conflicts (table_name, old_account_id, new_account_id, conflict_count, detected_at)
SELECT 'ls_ads_daily', 'Sc_us', 'sc_us_2', COUNT(*), NOW()
FROM ls_ads_daily old JOIN ls_ads_daily target ON target.account_id COLLATE utf8mb4_bin = 'sc_us_2' AND old.report_id <=> target.report_id
WHERE old.account_id COLLATE utf8mb4_bin = 'Sc_us' HAVING COUNT(*) > 0
ON DUPLICATE KEY UPDATE conflict_count = VALUES(conflict_count), detected_at = VALUES(detected_at);
UPDATE ls_ads_daily old LEFT JOIN ls_ads_daily target ON target.account_id COLLATE utf8mb4_bin = 'sc_us_2' AND old.report_id <=> target.report_id
SET old.account_id = 'sc_us_2' WHERE old.account_id COLLATE utf8mb4_bin = 'Sc_us' AND target.account_id IS NULL;
INSERT INTO migration_account_conflicts (table_name, old_account_id, new_account_id, conflict_count, detected_at)
SELECT 'ls_ads_daily', 'sc_us', 'sc_us_1', COUNT(*), NOW()
FROM ls_ads_daily old JOIN ls_ads_daily target ON target.account_id COLLATE utf8mb4_bin = 'sc_us_1' AND old.report_id <=> target.report_id
WHERE old.account_id COLLATE utf8mb4_bin = 'sc_us' HAVING COUNT(*) > 0
ON DUPLICATE KEY UPDATE conflict_count = VALUES(conflict_count), detected_at = VALUES(detected_at);
UPDATE ls_ads_daily old LEFT JOIN ls_ads_daily target ON target.account_id COLLATE utf8mb4_bin = 'sc_us_1' AND old.report_id <=> target.report_id
SET old.account_id = 'sc_us_1' WHERE old.account_id COLLATE utf8mb4_bin = 'sc_us' AND target.account_id IS NULL;

INSERT INTO migration_account_conflicts (table_name, old_account_id, new_account_id, conflict_count, detected_at)
SELECT 'ls_sc_sales_report', 'Sc_us', 'sc_us_2', COUNT(*), NOW()
FROM ls_sc_sales_report old JOIN ls_sc_sales_report target ON target.account_id COLLATE utf8mb4_bin = 'sc_us_2' AND old.sid <=> target.sid AND old.r_date <=> target.r_date AND old.asin <=> target.asin
WHERE old.account_id COLLATE utf8mb4_bin = 'Sc_us' HAVING COUNT(*) > 0
ON DUPLICATE KEY UPDATE conflict_count = VALUES(conflict_count), detected_at = VALUES(detected_at);
UPDATE ls_sc_sales_report old LEFT JOIN ls_sc_sales_report target ON target.account_id COLLATE utf8mb4_bin = 'sc_us_2' AND old.sid <=> target.sid AND old.r_date <=> target.r_date AND old.asin <=> target.asin
SET old.account_id = 'sc_us_2' WHERE old.account_id COLLATE utf8mb4_bin = 'Sc_us' AND target.account_id IS NULL;
INSERT INTO migration_account_conflicts (table_name, old_account_id, new_account_id, conflict_count, detected_at)
SELECT 'ls_sc_sales_report', 'sc_us', 'sc_us_1', COUNT(*), NOW()
FROM ls_sc_sales_report old JOIN ls_sc_sales_report target ON target.account_id COLLATE utf8mb4_bin = 'sc_us_1' AND old.sid <=> target.sid AND old.r_date <=> target.r_date AND old.asin <=> target.asin
WHERE old.account_id COLLATE utf8mb4_bin = 'sc_us' HAVING COUNT(*) > 0
ON DUPLICATE KEY UPDATE conflict_count = VALUES(conflict_count), detected_at = VALUES(detected_at);
UPDATE ls_sc_sales_report old LEFT JOIN ls_sc_sales_report target ON target.account_id COLLATE utf8mb4_bin = 'sc_us_1' AND old.sid <=> target.sid AND old.r_date <=> target.r_date AND old.asin <=> target.asin
SET old.account_id = 'sc_us_1' WHERE old.account_id COLLATE utf8mb4_bin = 'sc_us' AND target.account_id IS NULL;

INSERT INTO migration_account_conflicts (table_name, old_account_id, new_account_id, conflict_count, detected_at)
SELECT 'ls_vc_realtime_sales', 'Sc_us', 'sc_us_2', COUNT(*), NOW()
FROM ls_vc_realtime_sales old JOIN ls_vc_realtime_sales target ON target.account_id COLLATE utf8mb4_bin = 'sc_us_2' AND old.asin <=> target.asin AND old.startTime <=> target.startTime
WHERE old.account_id COLLATE utf8mb4_bin = 'Sc_us' HAVING COUNT(*) > 0
ON DUPLICATE KEY UPDATE conflict_count = VALUES(conflict_count), detected_at = VALUES(detected_at);
UPDATE ls_vc_realtime_sales old LEFT JOIN ls_vc_realtime_sales target ON target.account_id COLLATE utf8mb4_bin = 'sc_us_2' AND old.asin <=> target.asin AND old.startTime <=> target.startTime
SET old.account_id = 'sc_us_2' WHERE old.account_id COLLATE utf8mb4_bin = 'Sc_us' AND target.account_id IS NULL;
INSERT INTO migration_account_conflicts (table_name, old_account_id, new_account_id, conflict_count, detected_at)
SELECT 'ls_vc_realtime_sales', 'sc_us', 'sc_us_1', COUNT(*), NOW()
FROM ls_vc_realtime_sales old JOIN ls_vc_realtime_sales target ON target.account_id COLLATE utf8mb4_bin = 'sc_us_1' AND old.asin <=> target.asin AND old.startTime <=> target.startTime
WHERE old.account_id COLLATE utf8mb4_bin = 'sc_us' HAVING COUNT(*) > 0
ON DUPLICATE KEY UPDATE conflict_count = VALUES(conflict_count), detected_at = VALUES(detected_at);
UPDATE ls_vc_realtime_sales old LEFT JOIN ls_vc_realtime_sales target ON target.account_id COLLATE utf8mb4_bin = 'sc_us_1' AND old.asin <=> target.asin AND old.startTime <=> target.startTime
SET old.account_id = 'sc_us_1' WHERE old.account_id COLLATE utf8mb4_bin = 'sc_us' AND target.account_id IS NULL;

INSERT INTO migration_account_conflicts (table_name, old_account_id, new_account_id, conflict_count, detected_at)
SELECT 'ls_vc_sales_report', 'Sc_us', 'sc_us_2', COUNT(*), NOW()
FROM ls_vc_sales_report old JOIN ls_vc_sales_report target ON target.account_id COLLATE utf8mb4_bin = 'sc_us_2' AND old.asin <=> target.asin AND old.`date` <=> target.`date`
WHERE old.account_id COLLATE utf8mb4_bin = 'Sc_us' HAVING COUNT(*) > 0
ON DUPLICATE KEY UPDATE conflict_count = VALUES(conflict_count), detected_at = VALUES(detected_at);
UPDATE ls_vc_sales_report old LEFT JOIN ls_vc_sales_report target ON target.account_id COLLATE utf8mb4_bin = 'sc_us_2' AND old.asin <=> target.asin AND old.`date` <=> target.`date`
SET old.account_id = 'sc_us_2' WHERE old.account_id COLLATE utf8mb4_bin = 'Sc_us' AND target.account_id IS NULL;
INSERT INTO migration_account_conflicts (table_name, old_account_id, new_account_id, conflict_count, detected_at)
SELECT 'ls_vc_sales_report', 'sc_us', 'sc_us_1', COUNT(*), NOW()
FROM ls_vc_sales_report old JOIN ls_vc_sales_report target ON target.account_id COLLATE utf8mb4_bin = 'sc_us_1' AND old.asin <=> target.asin AND old.`date` <=> target.`date`
WHERE old.account_id COLLATE utf8mb4_bin = 'sc_us' HAVING COUNT(*) > 0
ON DUPLICATE KEY UPDATE conflict_count = VALUES(conflict_count), detected_at = VALUES(detected_at);
UPDATE ls_vc_sales_report old LEFT JOIN ls_vc_sales_report target ON target.account_id COLLATE utf8mb4_bin = 'sc_us_1' AND old.asin <=> target.asin AND old.`date` <=> target.`date`
SET old.account_id = 'sc_us_1' WHERE old.account_id COLLATE utf8mb4_bin = 'sc_us' AND target.account_id IS NULL;

INSERT INTO migration_account_conflicts (table_name, old_account_id, new_account_id, conflict_count, detected_at)
SELECT 'store_sync_selection', 'Sc_us', 'sc_us_2', COUNT(*), NOW()
FROM store_sync_selection old JOIN store_sync_selection target ON target.account_id COLLATE utf8mb4_bin = 'sc_us_2' AND old.sid <=> target.sid
WHERE old.account_id COLLATE utf8mb4_bin = 'Sc_us' HAVING COUNT(*) > 0
ON DUPLICATE KEY UPDATE conflict_count = VALUES(conflict_count), detected_at = VALUES(detected_at);
UPDATE store_sync_selection old LEFT JOIN store_sync_selection target ON target.account_id COLLATE utf8mb4_bin = 'sc_us_2' AND old.sid <=> target.sid
SET old.account_id = 'sc_us_2' WHERE old.account_id COLLATE utf8mb4_bin = 'Sc_us' AND target.account_id IS NULL;
INSERT INTO migration_account_conflicts (table_name, old_account_id, new_account_id, conflict_count, detected_at)
SELECT 'store_sync_selection', 'sc_us', 'sc_us_1', COUNT(*), NOW()
FROM store_sync_selection old JOIN store_sync_selection target ON target.account_id COLLATE utf8mb4_bin = 'sc_us_1' AND old.sid <=> target.sid
WHERE old.account_id COLLATE utf8mb4_bin = 'sc_us' HAVING COUNT(*) > 0
ON DUPLICATE KEY UPDATE conflict_count = VALUES(conflict_count), detected_at = VALUES(detected_at);
UPDATE store_sync_selection old LEFT JOIN store_sync_selection target ON target.account_id COLLATE utf8mb4_bin = 'sc_us_1' AND old.sid <=> target.sid
SET old.account_id = 'sc_us_1' WHERE old.account_id COLLATE utf8mb4_bin = 'sc_us' AND target.account_id IS NULL;

INSERT INTO migration_account_conflicts (table_name, old_account_id, new_account_id, conflict_count, detected_at)
SELECT 'vc_store_profiles', 'Sc_us', 'sc_us_2', COUNT(*), NOW()
FROM vc_store_profiles old JOIN vc_store_profiles target ON target.account_id COLLATE utf8mb4_bin = 'sc_us_2' AND old.sid <=> target.sid
WHERE old.account_id COLLATE utf8mb4_bin = 'Sc_us' HAVING COUNT(*) > 0
ON DUPLICATE KEY UPDATE conflict_count = VALUES(conflict_count), detected_at = VALUES(detected_at);
UPDATE vc_store_profiles old LEFT JOIN vc_store_profiles target ON target.account_id COLLATE utf8mb4_bin = 'sc_us_2' AND old.sid <=> target.sid
SET old.account_id = 'sc_us_2' WHERE old.account_id COLLATE utf8mb4_bin = 'Sc_us' AND target.account_id IS NULL;
INSERT INTO migration_account_conflicts (table_name, old_account_id, new_account_id, conflict_count, detected_at)
SELECT 'vc_store_profiles', 'sc_us', 'sc_us_1', COUNT(*), NOW()
FROM vc_store_profiles old JOIN vc_store_profiles target ON target.account_id COLLATE utf8mb4_bin = 'sc_us_1' AND old.sid <=> target.sid
WHERE old.account_id COLLATE utf8mb4_bin = 'sc_us' HAVING COUNT(*) > 0
ON DUPLICATE KEY UPDATE conflict_count = VALUES(conflict_count), detected_at = VALUES(detected_at);
UPDATE vc_store_profiles old LEFT JOIN vc_store_profiles target ON target.account_id COLLATE utf8mb4_bin = 'sc_us_1' AND old.sid <=> target.sid
SET old.account_id = 'sc_us_1' WHERE old.account_id COLLATE utf8mb4_bin = 'sc_us' AND target.account_id IS NULL;
