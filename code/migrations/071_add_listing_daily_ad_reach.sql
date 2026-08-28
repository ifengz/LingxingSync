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

-- Reproject only the already-synced, identity-safe advertising raw rows. A
-- missing source row deliberately leaves the daily value NULL.
UPDATE listing_daily_metrics m
JOIN listing_dimensions d ON d.id = m.listing_dimension_id
JOIN ls_stores s ON s.sid = d.store_id
JOIN (
    SELECT account_id, sid, report_date, asin, sku,
           SUM(impressions) AS impressions, SUM(clicks) AS clicks
      FROM ls_ad_sp_product
     GROUP BY account_id, sid, report_date, asin, sku
) raw ON raw.account_id = s.account_id AND raw.sid = d.store_id
     AND raw.report_date = m.business_date AND raw.asin = d.asin AND raw.sku = d.sku
   SET m.sp_impressions = CASE WHEN raw.impressions IS NULL THEN m.sp_impressions ELSE raw.impressions END,
       m.sp_impressions_source = CASE WHEN raw.impressions IS NULL THEN m.sp_impressions_source ELSE 'api' END,
       m.sp_clicks = CASE WHEN raw.clicks IS NULL THEN m.sp_clicks ELSE raw.clicks END,
       m.sp_clicks_source = CASE WHEN raw.clicks IS NULL THEN m.sp_clicks_source ELSE 'api' END
 WHERE d.channel = 'sc_fba' AND d.identity_scope = 'listing';

UPDATE listing_daily_metrics m
JOIN listing_dimensions d ON d.id = m.listing_dimension_id
JOIN ls_stores s ON s.sid = d.store_id
JOIN (
    SELECT account_id, sid, report_date, asin, sku,
           SUM(impressions) AS impressions, SUM(clicks) AS clicks
      FROM ls_ad_sd_product
     GROUP BY account_id, sid, report_date, asin, sku
) raw ON raw.account_id = s.account_id AND raw.sid = d.store_id
     AND raw.report_date = m.business_date AND raw.asin = d.asin AND raw.sku = d.sku
   SET m.sd_impressions = CASE WHEN raw.impressions IS NULL THEN m.sd_impressions ELSE raw.impressions END,
       m.sd_impressions_source = CASE WHEN raw.impressions IS NULL THEN m.sd_impressions_source ELSE 'api' END,
       m.sd_clicks = CASE WHEN raw.clicks IS NULL THEN m.sd_clicks ELSE raw.clicks END,
       m.sd_clicks_source = CASE WHEN raw.clicks IS NULL THEN m.sd_clicks_source ELSE 'api' END
 WHERE d.channel = 'sc_fba' AND d.identity_scope = 'listing';

UPDATE listing_daily_metrics m
JOIN listing_dimensions d ON d.id = m.listing_dimension_id
JOIN ls_stores s ON s.sid = d.store_id
JOIN (
    SELECT account_id, sid, report_date,
           SUM(impressions) AS impressions, SUM(clicks) AS clicks
      FROM ls_ad_hsa_campaign
     GROUP BY account_id, sid, report_date
) raw ON raw.account_id = s.account_id AND raw.sid = d.store_id
     AND raw.report_date = m.business_date
   SET m.hsa_impressions = CASE WHEN raw.impressions IS NULL THEN m.hsa_impressions ELSE raw.impressions END,
       m.hsa_impressions_source = CASE WHEN raw.impressions IS NULL THEN m.hsa_impressions_source ELSE 'api' END,
       m.hsa_clicks = CASE WHEN raw.clicks IS NULL THEN m.hsa_clicks ELSE raw.clicks END,
       m.hsa_clicks_source = CASE WHEN raw.clicks IS NULL THEN m.hsa_clicks_source ELSE 'api' END
 WHERE d.channel = 'hsa' AND d.identity_scope = 'store';
