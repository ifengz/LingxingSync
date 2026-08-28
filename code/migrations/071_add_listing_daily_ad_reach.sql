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
