-- Add SC Performance category rank facts to existing listing daily rows.
-- The raw source already stores cate_rank and small_cate_rank; this migration
-- only extends the canonical listing fact table and is safe to rerun.
SET @has_cate_rank := (
    SELECT COUNT(*) FROM INFORMATION_SCHEMA.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'listing_daily_metrics'
      AND COLUMN_NAME = 'cate_rank'
);
SET @sql := IF(@has_cate_rank = 0,
    'ALTER TABLE listing_daily_metrics
       ADD COLUMN cate_rank BIGINT NULL,
       ADD COLUMN cate_rank_source VARCHAR(16) NOT NULL DEFAULT ''''',
    'DO 0');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @has_small_cate_rank := (
    SELECT COUNT(*) FROM INFORMATION_SCHEMA.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'listing_daily_metrics'
      AND COLUMN_NAME = 'small_cate_rank'
);
SET @sql := IF(@has_small_cate_rank = 0,
    'ALTER TABLE listing_daily_metrics
       ADD COLUMN small_cate_rank BIGINT NULL,
       ADD COLUMN small_cate_rank_source VARCHAR(16) NOT NULL DEFAULT ''''',
    'DO 0');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;
