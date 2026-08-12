-- Upgrade an existing 033 fact table to microsecond update timestamps.
-- CREATE TABLE IF NOT EXISTS cannot change a column that was already created.
SET @listing_daily_updated_at_precision := (
    SELECT DATETIME_PRECISION
    FROM INFORMATION_SCHEMA.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE()
      AND TABLE_NAME = 'listing_daily_metrics'
      AND COLUMN_NAME = 'updated_at'
);

SET @listing_daily_updated_at_precision_sql := CASE
    WHEN @listing_daily_updated_at_precision IS NULL
         OR @listing_daily_updated_at_precision = 6
        THEN 'DO 0'
    ELSE 'ALTER TABLE listing_daily_metrics MODIFY updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6)'
END;

PREPARE listing_daily_updated_at_precision_stmt FROM @listing_daily_updated_at_precision_sql;
EXECUTE listing_daily_updated_at_precision_stmt;
DEALLOCATE PREPARE listing_daily_updated_at_precision_stmt;
