package db

import (
	"context"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
)

type FBAInventorySnapshotTarget struct {
	Store     string
	Date      time.Time
	StartedAt time.Time
}

const fbaInventorySnapshotDeleteSQL = `DELETE FROM fba_inventory_daily_snapshots
WHERE account_id = ? AND sid = ? AND snapshot_date = ?`

const fbaInventorySnapshotInsertSQL = `INSERT INTO fba_inventory_daily_snapshots (
account_id, snapshot_date,
afn_erp_real_shipped_quantity, afn_fulfillable_quantity, afn_fulfillable_quantity_multi,
afn_inbound_receiving_quantity, afn_inbound_shipped_quantity, afn_inbound_working_quantity,
afn_researching_quantity, afn_reserved_quantity, afn_unsellable_quantity,
asin, brand_id, brand_name, category_id, category_name, cost,
estimated_excess_quantity, estimated_storage_cost_next_month,
fba_inventory_level_health_status, fba_minimum_inventory_level, fnsku,
fulfillment_channel_name, historical_days_of_supply,
inv_age_0_to_30_days, inv_age_0_to_90_days, inv_age_181_to_270_days,
inv_age_271_to_330_days, inv_age_271_to_365_days, inv_age_31_to_60_days,
inv_age_331_to_365_days, inv_age_365_plus_days, inv_age_61_to_90_days,
inv_age_91_to_180_days, long_term_historical_days_of_supply,
low_inventory_level_fee_applied, msku, name, product_image, product_name,
recommended_action, reserved_customerorders, reserved_fc_processing,
reserved_fc_transfers, sell_through, share_type,
short_term_historical_days_of_supply, sid, sku, stock_cost_total,
total_fulfillable_quantity, wname, source_synced_at, updated_at
)
SELECT i.account_id, ?,
i.afn_erp_real_shipped_quantity, i.afn_fulfillable_quantity, i.afn_fulfillable_quantity_multi,
i.afn_inbound_receiving_quantity, i.afn_inbound_shipped_quantity, i.afn_inbound_working_quantity,
i.afn_researching_quantity, i.afn_reserved_quantity, i.afn_unsellable_quantity,
i.asin, i.brand_id, i.brand_name, i.category_id, i.category_name, i.cost,
i.estimated_excess_quantity, i.estimated_storage_cost_next_month,
i.fba_inventory_level_health_status, i.fba_minimum_inventory_level, i.fnsku,
i.fulfillment_channel_name, i.historical_days_of_supply,
i.inv_age_0_to_30_days, i.inv_age_0_to_90_days, i.inv_age_181_to_270_days,
i.inv_age_271_to_330_days, i.inv_age_271_to_365_days, i.inv_age_31_to_60_days,
i.inv_age_331_to_365_days, i.inv_age_365_plus_days, i.inv_age_61_to_90_days,
i.inv_age_91_to_180_days, i.long_term_historical_days_of_supply,
i.low_inventory_level_fee_applied, i.msku, i.name, i.product_image, i.product_name,
i.recommended_action, i.reserved_customerorders, i.reserved_fc_processing,
i.reserved_fc_transfers, i.sell_through, i.share_type,
i.short_term_historical_days_of_supply, i.sid, i.sku, i.stock_cost_total,
i.total_fulfillable_quantity, i.wname, i.synced_at, CURRENT_TIMESTAMP(6)
FROM ls_fba_inventory i
WHERE i.account_id = ? AND i.sid = ? AND i.synced_at >= ?`

func CaptureFBAInventorySnapshots(ctx context.Context, dbx *sqlx.DB, accountID string, targets []FBAInventorySnapshotTarget) error {
	if dbx == nil || accountID == "" || len(targets) == 0 {
		return fmt.Errorf("FBA inventory snapshot: database, account, and targets are required")
	}
	tx, err := dbx.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("FBA inventory snapshot: begin transaction: %w", err)
	}
	defer tx.Rollback()
	seen := make(map[string]struct{}, len(targets))
	for _, target := range targets {
		date := target.Date.Format("2006-01-02")
		if target.Store == "" || target.Date.IsZero() || target.StartedAt.IsZero() {
			return fmt.Errorf("FBA inventory snapshot: store, date, and task start are required")
		}
		key := target.Store + "\x00" + date
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		if _, err := tx.ExecContext(ctx, fbaInventorySnapshotDeleteSQL, accountID, target.Store, date); err != nil {
			return fmt.Errorf("FBA inventory snapshot: replace %s/%s: %w", target.Store, date, err)
		}
		if _, err := tx.ExecContext(ctx, fbaInventorySnapshotInsertSQL, date, accountID, target.Store, target.StartedAt); err != nil {
			return fmt.Errorf("FBA inventory snapshot: capture %s/%s: %w", target.Store, date, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("FBA inventory snapshot: commit: %w", err)
	}
	return nil
}
