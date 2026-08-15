SET @has_reserved_staging := (
    SELECT COUNT(*) FROM INFORMATION_SCHEMA.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE()
      AND TABLE_NAME = 'ls_fba_reserved_inventory'
      AND COLUMN_NAME = 'reserved_staging'
);
SET @sql := IF(@has_reserved_staging = 0,
    'ALTER TABLE ls_fba_reserved_inventory ADD COLUMN reserved_staging VARCHAR(32) NULL AFTER `reserved_fc-processing`',
    'DO 0');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @has_reserved_program := (
    SELECT COUNT(*) FROM INFORMATION_SCHEMA.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE()
      AND TABLE_NAME = 'ls_fba_reserved_inventory'
      AND COLUMN_NAME = 'program'
);
SET @sql := IF(@has_reserved_program = 0,
    'ALTER TABLE ls_fba_reserved_inventory ADD COLUMN program VARCHAR(128) NULL AFTER reserved_staging',
    'DO 0');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;
