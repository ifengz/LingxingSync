-- 074: 给所有数据源表的 synced_at 补索引。
--
-- 背景：/api/endpoints（数据源页）对每个 endpoint 执行一次
-- SELECT MAX(synced_at) FROM `<table>`（db.TableLastSync），此前 synced_at
-- 无索引，大表（订单/广告报告几十万行起）每次都是全表扫描，N 个 endpoint
-- 串行累加，数据源列表迟迟渲染不出来。
--
-- 幂等性：MySQL 没有 ADD INDEX IF NOT EXISTS，沿用 029 的样板——
-- 查 INFORMATION_SCHEMA 确认索引不存在才 ALTER，可重复执行。
-- 表可能尚未创建（模板未启用），先判表存在，缺表直接跳过。

-- ls_ad_accounts
SET @t := (SELECT COUNT(*) FROM INFORMATION_SCHEMA.TABLES WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'ls_ad_accounts');
SET @i := (SELECT COUNT(*) FROM INFORMATION_SCHEMA.STATISTICS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'ls_ad_accounts' AND INDEX_NAME = 'idx_ls_ad_accounts_synced_at');
SET @sql := IF(@t = 1 AND @i = 0, 'ALTER TABLE `ls_ad_accounts` ADD INDEX `idx_ls_ad_accounts_synced_at` (synced_at)', 'DO 0');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

-- ls_ad_hsa_campaign
SET @t := (SELECT COUNT(*) FROM INFORMATION_SCHEMA.TABLES WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'ls_ad_hsa_campaign');
SET @i := (SELECT COUNT(*) FROM INFORMATION_SCHEMA.STATISTICS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'ls_ad_hsa_campaign' AND INDEX_NAME = 'idx_ls_ad_hsa_campaign_synced_at');
SET @sql := IF(@t = 1 AND @i = 0, 'ALTER TABLE `ls_ad_hsa_campaign` ADD INDEX `idx_ls_ad_hsa_campaign_synced_at` (synced_at)', 'DO 0');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

-- ls_ad_sd_campaign
SET @t := (SELECT COUNT(*) FROM INFORMATION_SCHEMA.TABLES WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'ls_ad_sd_campaign');
SET @i := (SELECT COUNT(*) FROM INFORMATION_SCHEMA.STATISTICS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'ls_ad_sd_campaign' AND INDEX_NAME = 'idx_ls_ad_sd_campaign_synced_at');
SET @sql := IF(@t = 1 AND @i = 0, 'ALTER TABLE `ls_ad_sd_campaign` ADD INDEX `idx_ls_ad_sd_campaign_synced_at` (synced_at)', 'DO 0');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

-- ls_ad_sd_product
SET @t := (SELECT COUNT(*) FROM INFORMATION_SCHEMA.TABLES WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'ls_ad_sd_product');
SET @i := (SELECT COUNT(*) FROM INFORMATION_SCHEMA.STATISTICS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'ls_ad_sd_product' AND INDEX_NAME = 'idx_ls_ad_sd_product_synced_at');
SET @sql := IF(@t = 1 AND @i = 0, 'ALTER TABLE `ls_ad_sd_product` ADD INDEX `idx_ls_ad_sd_product_synced_at` (synced_at)', 'DO 0');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

-- ls_ad_sp_campaign
SET @t := (SELECT COUNT(*) FROM INFORMATION_SCHEMA.TABLES WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'ls_ad_sp_campaign');
SET @i := (SELECT COUNT(*) FROM INFORMATION_SCHEMA.STATISTICS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'ls_ad_sp_campaign' AND INDEX_NAME = 'idx_ls_ad_sp_campaign_synced_at');
SET @sql := IF(@t = 1 AND @i = 0, 'ALTER TABLE `ls_ad_sp_campaign` ADD INDEX `idx_ls_ad_sp_campaign_synced_at` (synced_at)', 'DO 0');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

-- ls_ad_sp_product
SET @t := (SELECT COUNT(*) FROM INFORMATION_SCHEMA.TABLES WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'ls_ad_sp_product');
SET @i := (SELECT COUNT(*) FROM INFORMATION_SCHEMA.STATISTICS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'ls_ad_sp_product' AND INDEX_NAME = 'idx_ls_ad_sp_product_synced_at');
SET @sql := IF(@t = 1 AND @i = 0, 'ALTER TABLE `ls_ad_sp_product` ADD INDEX `idx_ls_ad_sp_product_synced_at` (synced_at)', 'DO 0');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

-- ls_ad_vc_hsa_product
SET @t := (SELECT COUNT(*) FROM INFORMATION_SCHEMA.TABLES WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'ls_ad_vc_hsa_product');
SET @i := (SELECT COUNT(*) FROM INFORMATION_SCHEMA.STATISTICS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'ls_ad_vc_hsa_product' AND INDEX_NAME = 'idx_ls_ad_vc_hsa_product_synced_at');
SET @sql := IF(@t = 1 AND @i = 0, 'ALTER TABLE `ls_ad_vc_hsa_product` ADD INDEX `idx_ls_ad_vc_hsa_product_synced_at` (synced_at)', 'DO 0');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

-- ls_ad_vc_sd_product
SET @t := (SELECT COUNT(*) FROM INFORMATION_SCHEMA.TABLES WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'ls_ad_vc_sd_product');
SET @i := (SELECT COUNT(*) FROM INFORMATION_SCHEMA.STATISTICS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'ls_ad_vc_sd_product' AND INDEX_NAME = 'idx_ls_ad_vc_sd_product_synced_at');
SET @sql := IF(@t = 1 AND @i = 0, 'ALTER TABLE `ls_ad_vc_sd_product` ADD INDEX `idx_ls_ad_vc_sd_product_synced_at` (synced_at)', 'DO 0');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

-- ls_ad_vc_sp_product
SET @t := (SELECT COUNT(*) FROM INFORMATION_SCHEMA.TABLES WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'ls_ad_vc_sp_product');
SET @i := (SELECT COUNT(*) FROM INFORMATION_SCHEMA.STATISTICS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'ls_ad_vc_sp_product' AND INDEX_NAME = 'idx_ls_ad_vc_sp_product_synced_at');
SET @sql := IF(@t = 1 AND @i = 0, 'ALTER TABLE `ls_ad_vc_sp_product` ADD INDEX `idx_ls_ad_vc_sp_product_synced_at` (synced_at)', 'DO 0');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

-- ls_ad_vendor_accounts
SET @t := (SELECT COUNT(*) FROM INFORMATION_SCHEMA.TABLES WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'ls_ad_vendor_accounts');
SET @i := (SELECT COUNT(*) FROM INFORMATION_SCHEMA.STATISTICS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'ls_ad_vendor_accounts' AND INDEX_NAME = 'idx_ls_ad_vendor_accounts_synced_at');
SET @sql := IF(@t = 1 AND @i = 0, 'ALTER TABLE `ls_ad_vendor_accounts` ADD INDEX `idx_ls_ad_vendor_accounts_synced_at` (synced_at)', 'DO 0');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

-- ls_fba_inventory
SET @t := (SELECT COUNT(*) FROM INFORMATION_SCHEMA.TABLES WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'ls_fba_inventory');
SET @i := (SELECT COUNT(*) FROM INFORMATION_SCHEMA.STATISTICS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'ls_fba_inventory' AND INDEX_NAME = 'idx_ls_fba_inventory_synced_at');
SET @sql := IF(@t = 1 AND @i = 0, 'ALTER TABLE `ls_fba_inventory` ADD INDEX `idx_ls_fba_inventory_synced_at` (synced_at)', 'DO 0');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

-- ls_mp_fbm_orders
SET @t := (SELECT COUNT(*) FROM INFORMATION_SCHEMA.TABLES WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'ls_mp_fbm_orders');
SET @i := (SELECT COUNT(*) FROM INFORMATION_SCHEMA.STATISTICS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'ls_mp_fbm_orders' AND INDEX_NAME = 'idx_ls_mp_fbm_orders_synced_at');
SET @sql := IF(@t = 1 AND @i = 0, 'ALTER TABLE `ls_mp_fbm_orders` ADD INDEX `idx_ls_mp_fbm_orders_synced_at` (synced_at)', 'DO 0');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

-- ls_mp_store_mappings
SET @t := (SELECT COUNT(*) FROM INFORMATION_SCHEMA.TABLES WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'ls_mp_store_mappings');
SET @i := (SELECT COUNT(*) FROM INFORMATION_SCHEMA.STATISTICS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'ls_mp_store_mappings' AND INDEX_NAME = 'idx_ls_mp_store_mappings_synced_at');
SET @sql := IF(@t = 1 AND @i = 0, 'ALTER TABLE `ls_mp_store_mappings` ADD INDEX `idx_ls_mp_store_mappings_synced_at` (synced_at)', 'DO 0');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

-- ls_sales_orders
SET @t := (SELECT COUNT(*) FROM INFORMATION_SCHEMA.TABLES WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'ls_sales_orders');
SET @i := (SELECT COUNT(*) FROM INFORMATION_SCHEMA.STATISTICS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'ls_sales_orders' AND INDEX_NAME = 'idx_ls_sales_orders_synced_at');
SET @sql := IF(@t = 1 AND @i = 0, 'ALTER TABLE `ls_sales_orders` ADD INDEX `idx_ls_sales_orders_synced_at` (synced_at)', 'DO 0');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

-- ls_sc_fba_order_addresses
SET @t := (SELECT COUNT(*) FROM INFORMATION_SCHEMA.TABLES WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'ls_sc_fba_order_addresses');
SET @i := (SELECT COUNT(*) FROM INFORMATION_SCHEMA.STATISTICS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'ls_sc_fba_order_addresses' AND INDEX_NAME = 'idx_ls_sc_fba_order_addresses_synced_at');
SET @sql := IF(@t = 1 AND @i = 0, 'ALTER TABLE `ls_sc_fba_order_addresses` ADD INDEX `idx_ls_sc_fba_order_addresses_synced_at` (synced_at)', 'DO 0');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

-- ls_sc_listing
SET @t := (SELECT COUNT(*) FROM INFORMATION_SCHEMA.TABLES WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'ls_sc_listing');
SET @i := (SELECT COUNT(*) FROM INFORMATION_SCHEMA.STATISTICS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'ls_sc_listing' AND INDEX_NAME = 'idx_ls_sc_listing_synced_at');
SET @sql := IF(@t = 1 AND @i = 0, 'ALTER TABLE `ls_sc_listing` ADD INDEX `idx_ls_sc_listing_synced_at` (synced_at)', 'DO 0');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

-- ls_sc_order_details
SET @t := (SELECT COUNT(*) FROM INFORMATION_SCHEMA.TABLES WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'ls_sc_order_details');
SET @i := (SELECT COUNT(*) FROM INFORMATION_SCHEMA.STATISTICS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'ls_sc_order_details' AND INDEX_NAME = 'idx_ls_sc_order_details_synced_at');
SET @sql := IF(@t = 1 AND @i = 0, 'ALTER TABLE `ls_sc_order_details` ADD INDEX `idx_ls_sc_order_details_synced_at` (synced_at)', 'DO 0');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

-- ls_sc_performance_daily
SET @t := (SELECT COUNT(*) FROM INFORMATION_SCHEMA.TABLES WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'ls_sc_performance_daily');
SET @i := (SELECT COUNT(*) FROM INFORMATION_SCHEMA.STATISTICS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'ls_sc_performance_daily' AND INDEX_NAME = 'idx_ls_sc_performance_daily_synced_at');
SET @sql := IF(@t = 1 AND @i = 0, 'ALTER TABLE `ls_sc_performance_daily` ADD INDEX `idx_ls_sc_performance_daily_synced_at` (synced_at)', 'DO 0');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

-- ls_sc_products
SET @t := (SELECT COUNT(*) FROM INFORMATION_SCHEMA.TABLES WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'ls_sc_products');
SET @i := (SELECT COUNT(*) FROM INFORMATION_SCHEMA.STATISTICS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'ls_sc_products' AND INDEX_NAME = 'idx_ls_sc_products_synced_at');
SET @sql := IF(@t = 1 AND @i = 0, 'ALTER TABLE `ls_sc_products` ADD INDEX `idx_ls_sc_products_synced_at` (synced_at)', 'DO 0');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

-- ls_sc_refunds
SET @t := (SELECT COUNT(*) FROM INFORMATION_SCHEMA.TABLES WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'ls_sc_refunds');
SET @i := (SELECT COUNT(*) FROM INFORMATION_SCHEMA.STATISTICS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'ls_sc_refunds' AND INDEX_NAME = 'idx_ls_sc_refunds_synced_at');
SET @sql := IF(@t = 1 AND @i = 0, 'ALTER TABLE `ls_sc_refunds` ADD INDEX `idx_ls_sc_refunds_synced_at` (synced_at)', 'DO 0');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

-- ls_sc_removal_orders
SET @t := (SELECT COUNT(*) FROM INFORMATION_SCHEMA.TABLES WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'ls_sc_removal_orders');
SET @i := (SELECT COUNT(*) FROM INFORMATION_SCHEMA.STATISTICS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'ls_sc_removal_orders' AND INDEX_NAME = 'idx_ls_sc_removal_orders_synced_at');
SET @sql := IF(@t = 1 AND @i = 0, 'ALTER TABLE `ls_sc_removal_orders` ADD INDEX `idx_ls_sc_removal_orders_synced_at` (synced_at)', 'DO 0');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

-- ls_sc_sales_report
SET @t := (SELECT COUNT(*) FROM INFORMATION_SCHEMA.TABLES WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'ls_sc_sales_report');
SET @i := (SELECT COUNT(*) FROM INFORMATION_SCHEMA.STATISTICS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'ls_sc_sales_report' AND INDEX_NAME = 'idx_ls_sc_sales_report_synced_at');
SET @sql := IF(@t = 1 AND @i = 0, 'ALTER TABLE `ls_sc_sales_report` ADD INDEX `idx_ls_sc_sales_report_synced_at` (synced_at)', 'DO 0');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

-- ls_sc_sales_revenue
SET @t := (SELECT COUNT(*) FROM INFORMATION_SCHEMA.TABLES WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'ls_sc_sales_revenue');
SET @i := (SELECT COUNT(*) FROM INFORMATION_SCHEMA.STATISTICS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'ls_sc_sales_revenue' AND INDEX_NAME = 'idx_ls_sc_sales_revenue_synced_at');
SET @sql := IF(@t = 1 AND @i = 0, 'ALTER TABLE `ls_sc_sales_revenue` ADD INDEX `idx_ls_sc_sales_revenue_synced_at` (synced_at)', 'DO 0');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

-- ls_stores
SET @t := (SELECT COUNT(*) FROM INFORMATION_SCHEMA.TABLES WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'ls_stores');
SET @i := (SELECT COUNT(*) FROM INFORMATION_SCHEMA.STATISTICS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'ls_stores' AND INDEX_NAME = 'idx_ls_stores_synced_at');
SET @sql := IF(@t = 1 AND @i = 0, 'ALTER TABLE `ls_stores` ADD INDEX `idx_ls_stores_synced_at` (synced_at)', 'DO 0');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

-- ls_vc_inventory
SET @t := (SELECT COUNT(*) FROM INFORMATION_SCHEMA.TABLES WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'ls_vc_inventory');
SET @i := (SELECT COUNT(*) FROM INFORMATION_SCHEMA.STATISTICS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'ls_vc_inventory' AND INDEX_NAME = 'idx_ls_vc_inventory_synced_at');
SET @sql := IF(@t = 1 AND @i = 0, 'ALTER TABLE `ls_vc_inventory` ADD INDEX `idx_ls_vc_inventory_synced_at` (synced_at)', 'DO 0');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

-- ls_vc_margin
SET @t := (SELECT COUNT(*) FROM INFORMATION_SCHEMA.TABLES WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'ls_vc_margin');
SET @i := (SELECT COUNT(*) FROM INFORMATION_SCHEMA.STATISTICS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'ls_vc_margin' AND INDEX_NAME = 'idx_ls_vc_margin_synced_at');
SET @sql := IF(@t = 1 AND @i = 0, 'ALTER TABLE `ls_vc_margin` ADD INDEX `idx_ls_vc_margin_synced_at` (synced_at)', 'DO 0');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

-- ls_vc_orders
SET @t := (SELECT COUNT(*) FROM INFORMATION_SCHEMA.TABLES WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'ls_vc_orders');
SET @i := (SELECT COUNT(*) FROM INFORMATION_SCHEMA.STATISTICS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'ls_vc_orders' AND INDEX_NAME = 'idx_ls_vc_orders_synced_at');
SET @sql := IF(@t = 1 AND @i = 0, 'ALTER TABLE `ls_vc_orders` ADD INDEX `idx_ls_vc_orders_synced_at` (synced_at)', 'DO 0');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

-- ls_vc_po_details
SET @t := (SELECT COUNT(*) FROM INFORMATION_SCHEMA.TABLES WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'ls_vc_po_details');
SET @i := (SELECT COUNT(*) FROM INFORMATION_SCHEMA.STATISTICS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'ls_vc_po_details' AND INDEX_NAME = 'idx_ls_vc_po_details_synced_at');
SET @sql := IF(@t = 1 AND @i = 0, 'ALTER TABLE `ls_vc_po_details` ADD INDEX `idx_ls_vc_po_details_synced_at` (synced_at)', 'DO 0');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

-- ls_vc_realtime_sales
SET @t := (SELECT COUNT(*) FROM INFORMATION_SCHEMA.TABLES WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'ls_vc_realtime_sales');
SET @i := (SELECT COUNT(*) FROM INFORMATION_SCHEMA.STATISTICS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'ls_vc_realtime_sales' AND INDEX_NAME = 'idx_ls_vc_realtime_sales_synced_at');
SET @sql := IF(@t = 1 AND @i = 0, 'ALTER TABLE `ls_vc_realtime_sales` ADD INDEX `idx_ls_vc_realtime_sales_synced_at` (synced_at)', 'DO 0');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

-- ls_vc_sales_report
SET @t := (SELECT COUNT(*) FROM INFORMATION_SCHEMA.TABLES WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'ls_vc_sales_report');
SET @i := (SELECT COUNT(*) FROM INFORMATION_SCHEMA.STATISTICS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'ls_vc_sales_report' AND INDEX_NAME = 'idx_ls_vc_sales_report_synced_at');
SET @sql := IF(@t = 1 AND @i = 0, 'ALTER TABLE `ls_vc_sales_report` ADD INDEX `idx_ls_vc_sales_report_synced_at` (synced_at)', 'DO 0');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

-- ls_vc_traffic
SET @t := (SELECT COUNT(*) FROM INFORMATION_SCHEMA.TABLES WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'ls_vc_traffic');
SET @i := (SELECT COUNT(*) FROM INFORMATION_SCHEMA.STATISTICS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'ls_vc_traffic' AND INDEX_NAME = 'idx_ls_vc_traffic_synced_at');
SET @sql := IF(@t = 1 AND @i = 0, 'ALTER TABLE `ls_vc_traffic` ADD INDEX `idx_ls_vc_traffic_synced_at` (synced_at)', 'DO 0');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;
