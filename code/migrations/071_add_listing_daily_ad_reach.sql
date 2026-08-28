-- Add the per-ad-type exposure metrics without rebuilding existing daily facts.
SET @has_sp_impressions := (SELECT COUNT(*) FROM INFORMATION_SCHEMA.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'listing_daily_metrics' AND COLUMN_NAME = 'sp_impressions');
SET @sql := IF(@has_sp_impressions = 0, 'ALTER TABLE listing_daily_metrics ADD COLUMN sp_impressions BIGINT NULL, ADD COLUMN sp_impressions_source VARCHAR(16) NOT NULL DEFAULT ''''', 'DO 0');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @has_sp_clicks := (SELECT COUNT(*) FROM INFORMATION_SCHEMA.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'listing_daily_metrics' AND COLUMN_NAME = 'sp_clicks');
SET @sql := IF(@has_sp_clicks = 0, 'ALTER TABLE listing_daily_metrics ADD COLUMN sp_clicks BIGINT NULL, ADD COLUMN sp_clicks_source VARCHAR(16) NOT NULL DEFAULT ''''', 'DO 0');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @has_sd_impressions := (SELECT COUNT(*) FROM INFORMATION_SCHEMA.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'listing_daily_metrics' AND COLUMN_NAME = 'sd_impressions');
SET @sql := IF(@has_sd_impressions = 0, 'ALTER TABLE listing_daily_metrics ADD COLUMN sd_impressions BIGINT NULL, ADD COLUMN sd_impressions_source VARCHAR(16) NOT NULL DEFAULT ''''', 'DO 0');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @has_sd_clicks := (SELECT COUNT(*) FROM INFORMATION_SCHEMA.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'listing_daily_metrics' AND COLUMN_NAME = 'sd_clicks');
SET @sql := IF(@has_sd_clicks = 0, 'ALTER TABLE listing_daily_metrics ADD COLUMN sd_clicks BIGINT NULL, ADD COLUMN sd_clicks_source VARCHAR(16) NOT NULL DEFAULT ''''', 'DO 0');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @has_hsa_impressions := (SELECT COUNT(*) FROM INFORMATION_SCHEMA.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'listing_daily_metrics' AND COLUMN_NAME = 'hsa_impressions');
SET @sql := IF(@has_hsa_impressions = 0, 'ALTER TABLE listing_daily_metrics ADD COLUMN hsa_impressions BIGINT NULL, ADD COLUMN hsa_impressions_source VARCHAR(16) NOT NULL DEFAULT ''''', 'DO 0');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @has_hsa_clicks := (SELECT COUNT(*) FROM INFORMATION_SCHEMA.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'listing_daily_metrics' AND COLUMN_NAME = 'hsa_clicks');
SET @sql := IF(@has_hsa_clicks = 0, 'ALTER TABLE listing_daily_metrics ADD COLUMN hsa_clicks BIGINT NULL, ADD COLUMN hsa_clicks_source VARCHAR(16) NOT NULL DEFAULT ''''', 'DO 0');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @has_sb_impressions := (SELECT COUNT(*) FROM INFORMATION_SCHEMA.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'listing_daily_metrics' AND COLUMN_NAME = 'sb_impressions');
SET @sql := IF(@has_sb_impressions = 0, 'ALTER TABLE listing_daily_metrics ADD COLUMN sb_impressions BIGINT NULL, ADD COLUMN sb_impressions_source VARCHAR(16) NOT NULL DEFAULT ''''', 'DO 0');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @has_sb_clicks := (SELECT COUNT(*) FROM INFORMATION_SCHEMA.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'listing_daily_metrics' AND COLUMN_NAME = 'sb_clicks');
SET @sql := IF(@has_sb_clicks = 0, 'ALTER TABLE listing_daily_metrics ADD COLUMN sb_clicks BIGINT NULL, ADD COLUMN sb_clicks_source VARCHAR(16) NOT NULL DEFAULT ''''', 'DO 0');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @has_source_check := (SELECT COUNT(*) FROM INFORMATION_SCHEMA.TABLE_CONSTRAINTS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'listing_daily_metrics' AND CONSTRAINT_NAME = 'chk_listing_daily_sources');
SET @has_ad_reach_source_check := (
    SELECT COUNT(*)
    FROM INFORMATION_SCHEMA.TABLE_CONSTRAINTS tc
    JOIN INFORMATION_SCHEMA.CHECK_CONSTRAINTS cc ON cc.CONSTRAINT_SCHEMA = tc.CONSTRAINT_SCHEMA AND cc.CONSTRAINT_NAME = tc.CONSTRAINT_NAME
    WHERE tc.TABLE_SCHEMA = DATABASE() AND tc.TABLE_NAME = 'listing_daily_metrics' AND tc.CONSTRAINT_NAME = 'chk_listing_daily_sources'
      AND cc.CHECK_CLAUSE LIKE '%sp_impressions_source%'
);
SET @sql := IF(@has_source_check = 1 AND @has_ad_reach_source_check = 0, 'ALTER TABLE listing_daily_metrics DROP CHECK chk_listing_daily_sources', 'DO 0');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @has_source_check := (SELECT COUNT(*) FROM INFORMATION_SCHEMA.TABLE_CONSTRAINTS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'listing_daily_metrics' AND CONSTRAINT_NAME = 'chk_listing_daily_sources');
SET @sql := IF(@has_source_check = 0, 'ALTER TABLE listing_daily_metrics ADD CONSTRAINT chk_listing_daily_sources CHECK (
sales_units_source IN ('''' , ''api'', ''report'') AND sales_amount_source IN ('''' , ''api'', ''report'') AND returns_qty_source IN ('''' , ''api'', ''report'') AND
inventory_sellable_source IN ('''' , ''api'', ''report'') AND inventory_inbound_source IN ('''' , ''api'', ''report'') AND inventory_reserved_source IN ('''' , ''api'', ''report'') AND inventory_unfulfillable_source IN ('''' , ''api'', ''report'') AND inventory_local_warehouse_source IN ('''' , ''api'', ''report'') AND inventory_unhealthy_units_source IN ('''' , ''api'', ''report'') AND inventory_aged90_sellable_units_source IN ('''' , ''api'', ''report'') AND inventory_sell_through_rate_source IN ('''' , ''api'', ''report'') AND inventory_receive_fill_rate_source IN ('''' , ''api'', ''report'') AND inventory_vendor_confirmation_rate_source IN ('''' , ''api'', ''report'') AND inventory_avg_lead_time_days_source IN ('''' , ''api'', ''report'') AND inventory_sellable_cost_source IN ('''' , ''api'', ''report'') AND inventory_unfulfillable_cost_source IN ('''' , ''api'', ''report'') AND inventory_aged90_cost_source IN ('''' , ''api'', ''report'') AND inventory_unhealthy_cost_source IN ('''' , ''api'', ''report'') AND inventory_inbound_cost_source IN ('''' , ''api'', ''report'') AND inventory_currency_source IN ('''' , ''api'', ''report'') AND inventory_inbound_receiving_source IN ('''' , ''api'', ''report'') AND inventory_inbound_shipped_source IN ('''' , ''api'', ''report'') AND inventory_inbound_working_source IN ('''' , ''api'', ''report'') AND inventory_reserved_customer_orders_source IN ('''' , ''api'', ''report'') AND inventory_reserved_fc_processing_source IN ('''' , ''api'', ''report'') AND inventory_reserved_fc_transfers_source IN ('''' , ''api'', ''report'') AND
sessions_desktop_source IN ('''' , ''api'', ''report'') AND sessions_mobile_source IN ('''' , ''api'', ''report'') AND sessions_total_source IN ('''' , ''api'', ''report'') AND review_count_source IN ('''' , ''api'', ''report'') AND rating_source IN ('''' , ''api'', ''report'') AND
sp_spend_source IN ('''' , ''api'', ''report'') AND sp_sales_source IN ('''' , ''api'', ''report'') AND sp_orders_source IN ('''' , ''api'', ''report'') AND sp_impressions_source IN ('''' , ''api'', ''report'') AND sp_clicks_source IN ('''' , ''api'', ''report'') AND
sd_spend_source IN ('''' , ''api'', ''report'') AND sd_sales_source IN ('''' , ''api'', ''report'') AND sd_orders_source IN ('''' , ''api'', ''report'') AND sd_impressions_source IN ('''' , ''api'', ''report'') AND sd_clicks_source IN ('''' , ''api'', ''report'') AND
hsa_spend_source IN ('''' , ''api'', ''report'') AND hsa_sales_source IN ('''' , ''api'', ''report'') AND hsa_orders_source IN ('''' , ''api'', ''report'') AND hsa_impressions_source IN ('''' , ''api'', ''report'') AND hsa_clicks_source IN ('''' , ''api'', ''report'') AND
sb_spend_source IN ('''' , ''api'', ''report'') AND sb_sales_source IN ('''' , ''api'', ''report'') AND sb_orders_source IN ('''' , ''api'', ''report'') AND sb_impressions_source IN ('''' , ''api'', ''report'') AND sb_clicks_source IN ('''' , ''api'', ''report''))', 'DO 0');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;
