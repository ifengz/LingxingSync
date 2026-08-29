// Package vcad publishes the small normalized VC ad fact table used by
// downstream consumers. Raw tables remain the source of truth and are never
// changed by this projector.
package vcad

import (
	"context"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
)

type Target struct {
	ProfileID string
	Date      time.Time
}

type SQLStore struct{ DB *sqlx.DB }

func (s SQLStore) Rebuild(ctx context.Context, accountID string, targets []Target) error {
	if s.DB == nil {
		return fmt.Errorf("vc ad: nil database")
	}
	seen := make(map[string]struct{}, len(targets))
	for _, target := range targets {
		date := target.Date.Format("2006-01-02")
		key := target.ProfileID + "\x00" + date
		if target.ProfileID == "" || target.Date.IsZero() || has(seen, key) {
			continue
		}
		seen[key] = struct{}{}
		for _, typ := range []string{"SP", "SD", "HSA"} {
			if _, err := s.DB.ExecContext(ctx, `DELETE FROM vc_ad_daily WHERE account_id = ? AND profile_id = ? AND business_date = ? AND campaign_type = ?`, accountID, target.ProfileID, date, typ); err != nil {
				return fmt.Errorf("vc ad: clear %s %s/%s: %w", typ, target.ProfileID, date, err)
			}
		}
		if err := s.insertProduct(ctx, accountID, target, "SP", "ls_ad_vc_sp_product"); err != nil {
			return err
		}
		if err := s.insertProduct(ctx, accountID, target, "SD", "ls_ad_vc_sd_product"); err != nil {
			return err
		}
		if err := s.insertHSA(ctx, accountID, target); err != nil {
			return err
		}
	}
	return nil
}

func has(seen map[string]struct{}, key string) bool {
	_, ok := seen[key]
	return ok
}

func (s SQLStore) insertProduct(ctx context.Context, accountID string, target Target, typ, table string) error {
	query := fmt.Sprintf(`
INSERT INTO vc_ad_daily
(account_id, profile_id, attribution_scope, asin, business_date, campaign_type, spend, ad_sales, ad_orders, clicks, impressions, currency)
SELECT p.account_id, p.profile_id, 'asin', p.asin, p.report_date, ?,
       SUM(p.cost), SUM(p.sales), SUM(p.orders), SUM(p.clicks), SUM(p.impressions), MAX(COALESCE(a.currency_code, a.currency))
FROM %s p
LEFT JOIN ls_ad_vendor_accounts a ON a.account_id = p.account_id AND a.profile_id = p.profile_id
WHERE p.account_id = ? AND p.profile_id = ? AND p.report_date = ? AND p.asin IS NOT NULL AND p.asin <> ''
GROUP BY p.account_id, p.profile_id, p.asin, p.report_date`, table)
	if _, err := s.DB.ExecContext(ctx, query, typ, accountID, target.ProfileID, target.Date.Format("2006-01-02")); err != nil {
		return fmt.Errorf("vc ad: build %s %s/%s: %w", typ, target.ProfileID, target.Date.Format("2006-01-02"), err)
	}
	return nil
}

func (s SQLStore) insertHSA(ctx context.Context, accountID string, target Target) error {
	_, err := s.DB.ExecContext(ctx, `
INSERT INTO vc_ad_daily
(account_id, profile_id, attribution_scope, asin, business_date, campaign_type, spend, ad_sales, ad_orders, clicks, impressions, currency)
SELECT p.account_id, p.profile_id, 'profile_unattributed', '', p.report_date, 'HSA',
       SUM(p.cost), SUM(p.sales), SUM(p.orders), SUM(p.clicks), SUM(p.impressions), MAX(COALESCE(a.currency_code, a.currency))
FROM ls_ad_vc_hsa_product p
LEFT JOIN ls_ad_vendor_accounts a ON a.account_id = p.account_id AND a.profile_id = p.profile_id
WHERE p.account_id = ? AND p.profile_id = ? AND p.report_date = ?
GROUP BY p.account_id, p.profile_id, p.report_date`, accountID, target.ProfileID, target.Date.Format("2006-01-02"))
	if err != nil {
		return fmt.Errorf("vc ad: build HSA %s/%s: %w", target.ProfileID, target.Date.Format("2006-01-02"), err)
	}
	return nil
}
