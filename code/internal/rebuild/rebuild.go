// Package rebuild provides the historical listing-daily backfill used by both
// the CLI lane (-rebuild-listing-daily) and the HTTP endpoint
// (POST /api/rebuild-listing-daily). Keeping the loop here means the two entry
// points cannot drift apart.
package rebuild

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"

	"lingxing-sync/internal/config"
	"lingxing-sync/internal/db"
	"lingxing-sync/internal/listingdaily"
)

// ChannelsForStore returns the advertising channels to rebuild for a store
// type (SC → sc_fba/hsa, VC → vc).
func ChannelsForStore(storeType string) ([]string, error) {
	switch strings.ToUpper(strings.TrimSpace(storeType)) {
	case "", "SC":
		return []string{"sc_fba", "hsa"}, nil
	case "VC":
		return []string{"vc"}, nil
	default:
		return nil, fmt.Errorf("历史日维回刷：不支持店铺类型 %q", storeType)
	}
}

// ProgressFunc is called after each store/channel/day batch is persisted.
// totalRows is the cumulative row count written so far.
type ProgressFunc func(accountID, storeID, channel string, date time.Time, rows int, totalRows int)

// RunListingDaily rebuilds listing_daily_metrics from the ls_* raw tables for
// every account/store/channel/day in [from, to]. An empty accountID or storeID
// matches all. It returns the total number of rows written.
func RunListingDaily(ctx context.Context, dbx *sqlx.DB, cfg *config.Config, accountID, storeID string, from, to time.Time, progress ProgressFunc) (int, error) {
	if dbx == nil || cfg == nil {
		return 0, fmt.Errorf("历史日维回刷：数据库或配置未提供")
	}
	if from.IsZero() || to.IsZero() || from.After(to) {
		return 0, fmt.Errorf("历史日维回刷：日期范围无效")
	}
	accounts := cfg.Accounts
	if strings.TrimSpace(accountID) != "" {
		account := cfg.FindAccount(accountID)
		if account == nil {
			return 0, fmt.Errorf("历史日维回刷：账号不存在 %q", accountID)
		}
		accounts = []config.Account{*account}
	}
	reader := listingdaily.SQLSourceReader{DB: dbx}
	store := listingdaily.SQLStore{DB: dbx}
	today := time.Now()
	rowsWritten := 0
	for _, account := range accounts {
		stores, _, err := db.ListStoresForAccount(dbx, account.ID)
		if err != nil {
			return rowsWritten, err
		}
		for _, currentStore := range stores {
			if strings.TrimSpace(storeID) != "" && currentStore.SID != storeID {
				continue
			}
			channels, err := ChannelsForStore(currentStore.StoreType)
			if err != nil {
				return rowsWritten, err
			}
			for date := from; !date.After(to); date = date.AddDate(0, 0, 1) {
				for _, channel := range channels {
					_, rows, err := listingdaily.BuildFromSQL(ctx, reader, account.ID, currentStore.SID, channel, date, today, listingdaily.ReportAbsent)
					if err != nil {
						return rowsWritten, fmt.Errorf("历史日维回刷 %s/%s/%s/%s: %w", account.ID, currentStore.SID, channel, date.Format("2006-01-02"), err)
					}
					if err := store.Persist(ctx, rows); err != nil {
						return rowsWritten, fmt.Errorf("历史日维回刷 %s/%s/%s/%s 写入失败: %w", account.ID, currentStore.SID, channel, date.Format("2006-01-02"), err)
					}
					rowsWritten += len(rows)
					if progress != nil {
						progress(account.ID, currentStore.SID, channel, date, len(rows), rowsWritten)
					}
				}
			}
		}
	}
	return rowsWritten, nil
}

// ParseDate parses a YYYY-MM-DD string into a time.Time.
func ParseDate(value, flagName string) (time.Time, error) {
	date, err := time.Parse("2006-01-02", strings.TrimSpace(value))
	if err != nil {
		return time.Time{}, fmt.Errorf("%s 必须是 YYYY-MM-DD: %w", flagName, err)
	}
	return date, nil
}