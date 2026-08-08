-- 009_rename_account.sql — 账号 ID 一次性改名（宪法级唯一性收紧的配套数据迁移）
--
-- 背景：历史上建出了仅大小写不同的两个账号 `sc_us`（自营）与 `Sc_us`（联营）。
-- 新规则按「大小写不敏感唯一」判重（参考 GitHub），二者视为撞名，必须真正区分。
-- 统一改名：
--   sc_us  → sc_us_1   （自营）
--   Sc_us  → sc_us_2   （联营）
--
-- 关键约束：MySQL 字符串列默认 *_ci 排序规则（大小写不敏感）。若直接
--   WHERE account_id = 'Sc_us'
-- 会同时命中 'Sc_us' 和 'sc_us' 两个账号，把两者都改错。因此每条 WHERE 都用
--   account_id COLLATE utf8mb4_bin = '...'
-- 强制按字节精确匹配，只命中目标账号。
--
-- 幂等：改名后原值不复存在，重跑影响 0 行。可随启动重复执行（宪法 §迁移）。
--
-- 范围：仅 migrations/ 建过、且含 account_id 列的 10 张表：
--   sync_tasks（account_id 为普通列）
--   ls_stores / ls_sales_orders / ls_inventory / ls_ads_daily /
--   ls_sc_sales_report / ls_vc_realtime_sales / ls_vc_sales_report /
--   store_sync_selection / vc_store_profiles（account_id 为复合主键首列）
-- probe 端点对应的 ls_vc_orders / ls_sc_performance / ls_vc_traffic / ls_vc_inventory
-- 未被任何迁移创建（probe=true 启动跳过建表断言、不写数据），故不在此处理，
-- 否则 UPDATE 不存在的表会 fail-loud 阻断启动。

-- ---- Sc_us → sc_us_2（联营） ----
UPDATE sync_tasks           SET account_id = 'sc_us_2' WHERE account_id COLLATE utf8mb4_bin = 'Sc_us';
UPDATE ls_stores            SET account_id = 'sc_us_2' WHERE account_id COLLATE utf8mb4_bin = 'Sc_us';
UPDATE ls_sales_orders      SET account_id = 'sc_us_2' WHERE account_id COLLATE utf8mb4_bin = 'Sc_us';
UPDATE ls_inventory         SET account_id = 'sc_us_2' WHERE account_id COLLATE utf8mb4_bin = 'Sc_us';
UPDATE ls_ads_daily         SET account_id = 'sc_us_2' WHERE account_id COLLATE utf8mb4_bin = 'Sc_us';
UPDATE ls_sc_sales_report   SET account_id = 'sc_us_2' WHERE account_id COLLATE utf8mb4_bin = 'Sc_us';
UPDATE ls_vc_realtime_sales SET account_id = 'sc_us_2' WHERE account_id COLLATE utf8mb4_bin = 'Sc_us';
UPDATE ls_vc_sales_report   SET account_id = 'sc_us_2' WHERE account_id COLLATE utf8mb4_bin = 'Sc_us';
UPDATE store_sync_selection SET account_id = 'sc_us_2' WHERE account_id COLLATE utf8mb4_bin = 'Sc_us';
UPDATE vc_store_profiles    SET account_id = 'sc_us_2' WHERE account_id COLLATE utf8mb4_bin = 'Sc_us';

-- ---- sc_us → sc_us_1（自营） ----
UPDATE sync_tasks           SET account_id = 'sc_us_1' WHERE account_id COLLATE utf8mb4_bin = 'sc_us';
UPDATE ls_stores            SET account_id = 'sc_us_1' WHERE account_id COLLATE utf8mb4_bin = 'sc_us';
UPDATE ls_sales_orders      SET account_id = 'sc_us_1' WHERE account_id COLLATE utf8mb4_bin = 'sc_us';
UPDATE ls_inventory         SET account_id = 'sc_us_1' WHERE account_id COLLATE utf8mb4_bin = 'sc_us';
UPDATE ls_ads_daily         SET account_id = 'sc_us_1' WHERE account_id COLLATE utf8mb4_bin = 'sc_us';
UPDATE ls_sc_sales_report   SET account_id = 'sc_us_1' WHERE account_id COLLATE utf8mb4_bin = 'sc_us';
UPDATE ls_vc_realtime_sales SET account_id = 'sc_us_1' WHERE account_id COLLATE utf8mb4_bin = 'sc_us';
UPDATE ls_vc_sales_report   SET account_id = 'sc_us_1' WHERE account_id COLLATE utf8mb4_bin = 'sc_us';
UPDATE store_sync_selection SET account_id = 'sc_us_1' WHERE account_id COLLATE utf8mb4_bin = 'sc_us';
UPDATE vc_store_profiles    SET account_id = 'sc_us_1' WHERE account_id COLLATE utf8mb4_bin = 'sc_us';
