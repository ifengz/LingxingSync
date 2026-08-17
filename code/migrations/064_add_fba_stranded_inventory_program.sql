-- Preserve the production Stranded Inventory field observed in audit 76.
-- This is additive and keeps all existing rows unchanged.
SET @stranded_program_exists := (
    SELECT COUNT(*) FROM INFORMATION_SCHEMA.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'ls_fba_stranded_inventory' AND COLUMN_NAME = 'program'
);
SET @stranded_program_sql := IF(@stranded_program_exists = 0,
    'ALTER TABLE ls_fba_stranded_inventory ADD COLUMN `program` VARCHAR(128) NULL AFTER `inbound-shipped-qty`',
    'DO 0');
PREPARE stranded_program_stmt FROM @stranded_program_sql;
EXECUTE stranded_program_stmt;
DEALLOCATE PREPARE stranded_program_stmt;
