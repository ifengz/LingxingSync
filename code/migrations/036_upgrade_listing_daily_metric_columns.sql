-- Upgrade the early listing_daily_metrics shape without deleting existing facts.
SET @has_legacy_inventory := (
    SELECT COUNT(*) FROM INFORMATION_SCHEMA.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'listing_daily_metrics'
      AND COLUMN_NAME = 'inventory_snapshot'
);
SET @has_current_inventory := (
    SELECT COUNT(*) FROM INFORMATION_SCHEMA.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'listing_daily_metrics'
      AND COLUMN_NAME = 'inventory_sellable'
);
SET @has_source_check := (
    SELECT COUNT(*) FROM INFORMATION_SCHEMA.TABLE_CONSTRAINTS
    WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'listing_daily_metrics'
      AND CONSTRAINT_NAME = 'chk_listing_daily_sources'
);

SET @sql := IF(@has_legacy_inventory = 1 AND @has_current_inventory = 0 AND @has_source_check = 1,
    'ALTER TABLE listing_daily_metrics DROP CHECK chk_listing_daily_sources', 'DO 0');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @sql := IF(@has_legacy_inventory = 1 AND @has_current_inventory = 0,
    'ALTER TABLE listing_daily_metrics CHANGE COLUMN inventory_snapshot inventory_sellable BIGINT NULL, CHANGE COLUMN inventory_snapshot_source inventory_sellable_source VARCHAR(16) NOT NULL DEFAULT ''''', 'DO 0');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @has_verified_fields := (
    SELECT COUNT(*) FROM INFORMATION_SCHEMA.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'listing_daily_metrics'
      AND COLUMN_NAME = 'verified_fields'
);
SET @sql := IF(@has_current_inventory + @has_legacy_inventory = 1 AND @has_verified_fields = 0,
    'ALTER TABLE listing_daily_metrics
       ADD COLUMN inventory_inbound BIGINT NULL,
       ADD COLUMN inventory_inbound_source VARCHAR(16) NOT NULL DEFAULT '''',
       ADD COLUMN inventory_reserved BIGINT NULL,
       ADD COLUMN inventory_reserved_source VARCHAR(16) NOT NULL DEFAULT '''',
       ADD COLUMN inventory_unfulfillable BIGINT NULL,
       ADD COLUMN inventory_unfulfillable_source VARCHAR(16) NOT NULL DEFAULT '''',
       ADD COLUMN inventory_local_warehouse BIGINT NULL,
       ADD COLUMN inventory_local_warehouse_source VARCHAR(16) NOT NULL DEFAULT '''',
       ADD COLUMN inventory_unhealthy_units BIGINT NULL,
       ADD COLUMN inventory_unhealthy_units_source VARCHAR(16) NOT NULL DEFAULT '''',
       ADD COLUMN inventory_aged90_sellable_units BIGINT NULL,
       ADD COLUMN inventory_aged90_sellable_units_source VARCHAR(16) NOT NULL DEFAULT '''',
       ADD COLUMN inventory_sell_through_rate DECIMAL(20,6) NULL,
       ADD COLUMN inventory_sell_through_rate_source VARCHAR(16) NOT NULL DEFAULT '''',
       ADD COLUMN inventory_receive_fill_rate DECIMAL(20,6) NULL,
       ADD COLUMN inventory_receive_fill_rate_source VARCHAR(16) NOT NULL DEFAULT '''',
       ADD COLUMN inventory_vendor_confirmation_rate DECIMAL(20,6) NULL,
       ADD COLUMN inventory_vendor_confirmation_rate_source VARCHAR(16) NOT NULL DEFAULT '''',
       ADD COLUMN inventory_avg_lead_time_days DECIMAL(20,6) NULL,
       ADD COLUMN inventory_avg_lead_time_days_source VARCHAR(16) NOT NULL DEFAULT '''',
       ADD COLUMN inventory_sellable_cost DECIMAL(20,6) NULL,
       ADD COLUMN inventory_sellable_cost_source VARCHAR(16) NOT NULL DEFAULT '''',
       ADD COLUMN inventory_unfulfillable_cost DECIMAL(20,6) NULL,
       ADD COLUMN inventory_unfulfillable_cost_source VARCHAR(16) NOT NULL DEFAULT '''',
       ADD COLUMN inventory_aged90_cost DECIMAL(20,6) NULL,
       ADD COLUMN inventory_aged90_cost_source VARCHAR(16) NOT NULL DEFAULT '''',
       ADD COLUMN inventory_unhealthy_cost DECIMAL(20,6) NULL,
       ADD COLUMN inventory_unhealthy_cost_source VARCHAR(16) NOT NULL DEFAULT '''',
       ADD COLUMN inventory_inbound_cost DECIMAL(20,6) NULL,
       ADD COLUMN inventory_inbound_cost_source VARCHAR(16) NOT NULL DEFAULT '''',
       ADD COLUMN inventory_currency VARCHAR(8) NULL,
       ADD COLUMN inventory_currency_source VARCHAR(16) NOT NULL DEFAULT '''',
       ADD COLUMN inventory_inbound_receiving BIGINT NULL,
       ADD COLUMN inventory_inbound_receiving_source VARCHAR(16) NOT NULL DEFAULT '''',
       ADD COLUMN inventory_inbound_shipped BIGINT NULL,
       ADD COLUMN inventory_inbound_shipped_source VARCHAR(16) NOT NULL DEFAULT '''',
       ADD COLUMN inventory_inbound_working BIGINT NULL,
       ADD COLUMN inventory_inbound_working_source VARCHAR(16) NOT NULL DEFAULT '''',
       ADD COLUMN inventory_reserved_customer_orders BIGINT NULL,
       ADD COLUMN inventory_reserved_customer_orders_source VARCHAR(16) NOT NULL DEFAULT '''',
       ADD COLUMN inventory_reserved_fc_processing BIGINT NULL,
       ADD COLUMN inventory_reserved_fc_processing_source VARCHAR(16) NOT NULL DEFAULT '''',
       ADD COLUMN inventory_reserved_fc_transfers BIGINT NULL,
       ADD COLUMN inventory_reserved_fc_transfers_source VARCHAR(16) NOT NULL DEFAULT '''',
       ADD COLUMN hsa_spend DECIMAL(20,6) NULL,
       ADD COLUMN hsa_spend_source VARCHAR(16) NOT NULL DEFAULT '''',
       ADD COLUMN hsa_sales DECIMAL(20,6) NULL,
       ADD COLUMN hsa_sales_source VARCHAR(16) NOT NULL DEFAULT '''',
       ADD COLUMN hsa_orders BIGINT NULL,
       ADD COLUMN hsa_orders_source VARCHAR(16) NOT NULL DEFAULT '''',
       ADD COLUMN sb_spend DECIMAL(20,6) NULL,
       ADD COLUMN sb_spend_source VARCHAR(16) NOT NULL DEFAULT '''',
       ADD COLUMN sb_sales DECIMAL(20,6) NULL,
       ADD COLUMN sb_sales_source VARCHAR(16) NOT NULL DEFAULT '''',
       ADD COLUMN sb_orders BIGINT NULL,
       ADD COLUMN sb_orders_source VARCHAR(16) NOT NULL DEFAULT '''',
       ADD COLUMN verified_fields JSON NOT NULL DEFAULT (JSON_OBJECT())', 'DO 0');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @has_source_check := (
    SELECT COUNT(*) FROM INFORMATION_SCHEMA.TABLE_CONSTRAINTS
    WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'listing_daily_metrics'
      AND CONSTRAINT_NAME = 'chk_listing_daily_sources'
);
SET @sql := IF(@has_source_check = 0 AND @has_verified_fields = 0,
    'ALTER TABLE listing_daily_metrics ADD CONSTRAINT chk_listing_daily_sources CHECK (
       sales_units_source IN ('''', ''api'', ''report'') AND sales_amount_source IN ('''', ''api'', ''report'') AND returns_qty_source IN ('''', ''api'', ''report'') AND
       inventory_sellable_source IN ('''', ''api'', ''report'') AND inventory_inbound_source IN ('''', ''api'', ''report'') AND inventory_reserved_source IN ('''', ''api'', ''report'') AND inventory_unfulfillable_source IN ('''', ''api'', ''report'') AND inventory_local_warehouse_source IN ('''', ''api'', ''report'') AND inventory_unhealthy_units_source IN ('''', ''api'', ''report'') AND inventory_aged90_sellable_units_source IN ('''', ''api'', ''report'') AND inventory_sell_through_rate_source IN ('''', ''api'', ''report'') AND inventory_receive_fill_rate_source IN ('''', ''api'', ''report'') AND inventory_vendor_confirmation_rate_source IN ('''', ''api'', ''report'') AND inventory_avg_lead_time_days_source IN ('''', ''api'', ''report'') AND inventory_sellable_cost_source IN ('''', ''api'', ''report'') AND inventory_unfulfillable_cost_source IN ('''', ''api'', ''report'') AND inventory_aged90_cost_source IN ('''', ''api'', ''report'') AND inventory_unhealthy_cost_source IN ('''', ''api'', ''report'') AND inventory_inbound_cost_source IN ('''', ''api'', ''report'') AND inventory_currency_source IN ('''', ''api'', ''report'') AND inventory_inbound_receiving_source IN ('''', ''api'', ''report'') AND inventory_inbound_shipped_source IN ('''', ''api'', ''report'') AND inventory_inbound_working_source IN ('''', ''api'', ''report'') AND inventory_reserved_customer_orders_source IN ('''', ''api'', ''report'') AND inventory_reserved_fc_processing_source IN ('''', ''api'', ''report'') AND inventory_reserved_fc_transfers_source IN ('''', ''api'', ''report'') AND
       sessions_desktop_source IN ('''', ''api'', ''report'') AND sessions_mobile_source IN ('''', ''api'', ''report'') AND sessions_total_source IN ('''', ''api'', ''report'') AND review_count_source IN ('''', ''api'', ''report'') AND rating_source IN ('''', ''api'', ''report'') AND sp_spend_source IN ('''', ''api'', ''report'') AND sp_sales_source IN ('''', ''api'', ''report'') AND sp_orders_source IN ('''', ''api'', ''report'') AND sd_spend_source IN ('''', ''api'', ''report'') AND sd_sales_source IN ('''', ''api'', ''report'') AND sd_orders_source IN ('''', ''api'', ''report'') AND hsa_spend_source IN ('''', ''api'', ''report'') AND hsa_sales_source IN ('''', ''api'', ''report'') AND hsa_orders_source IN ('''', ''api'', ''report'') AND sb_spend_source IN ('''', ''api'', ''report'') AND sb_sales_source IN ('''', ''api'', ''report'') AND sb_orders_source IN ('''', ''api'', ''report''))', 'DO 0');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;
