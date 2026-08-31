package main

import (
	"context"
	"time"

	"github.com/jmoiron/sqlx"

	"lingxing-sync/internal/config"
	"lingxing-sync/internal/rebuild"
)

// rebuildChannelsForStore is kept as a thin wrapper so the CLI tests in
// rebuild_test.go (package main) continue to pass. The implementation lives
// in internal/rebuild so the HTTP endpoint shares the same code path.
func rebuildChannelsForStore(storeType string) ([]string, error) {
	return rebuild.ChannelsForStore(storeType)
}

// runListingDailyRebuild delegates to the shared package used by the HTTP
// rebuild endpoint, ensuring both entry points stay in sync.
func runListingDailyRebuild(ctx context.Context, dbx *sqlx.DB, cfg *config.Config, accountID, storeID string, from, to time.Time) (int, error) {
	return rebuild.RunListingDaily(ctx, dbx, cfg, accountID, storeID, from, to, nil)
}

// parseRebuildDate delegates to the shared package.
func parseRebuildDate(value, flagName string) (time.Time, error) {
	return rebuild.ParseDate(value, flagName)
}